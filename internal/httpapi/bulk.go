// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"group24.de/itsm/internal/bulk"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/records"
)

// maxBulkBody bounds a synchronous bulk call: 1,000 rows of up to 8 KiB each.
const maxBulkBody = 8 << 20

// idempotencyTTL is how long the answer to an Idempotency-Key is kept (PL31).
const idempotencyTTL = 24 * time.Hour

// idempotent is what is kept per Idempotency-Key: the checksum of the request body and the
// answer.
type idempotent struct {
	Body     string          `json:"body"`
	Status   int             `json:"status,omitempty"` // 200 when not set (PL31)
	Location string          `json:"location,omitempty"`
	Response json.RawMessage `json:"response"`
}

// BulkRecords is the bulk of one source (Hauptplan E38): synchronous with a JSON body (PL31),
// asynchronous with an NDJSON stream (PL32).
func (s *server) BulkRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, p gen.BulkRecordsParams) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/x-ndjson" {
		s.bulkStream(w, r, typeName, p)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBulkBody+1))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body: "+err.Error(), nil)
		return
	}
	if len(raw) > maxBulkBody {
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "too_large",
			"a synchronous bulk call takes at most 8 MiB; send more as a stream (asynchronous bulk)", nil)
		return
	}
	var in struct {
		Source string           `json:"source"`
		Rows   []map[string]any `json:"rows"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body: "+err.Error(), nil)
		return
	}
	if _, err := s.o.Records.Type(r.Context(), typeName); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}

	// The same key with the same body gets the same answer, without writing again.
	idemKey, ok := s.idempotencyKey(w, r, typeName, p)
	if !ok {
		return
	}
	bodySum := checksum(raw)
	if idemKey != "" && s.replay(w, r, idemKey, bodySum) {
		return
	}

	out, err := bulk.Sync(r.Context(), s.o.Records, typeName, in.Source, in.Rows, actor(r))
	if s.bulkError(w, r, err) {
		return
	}
	body, err := json.Marshal(out)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.keep(r, idemKey, idempotent{Body: bodySum, Response: body})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// bulkStream keeps an NDJSON stream and queues its job (PL32): 202 with the job.
func (s *server) bulkStream(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, p gen.BulkRecordsParams) {
	if s.o.Bulk == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the asynchronous bulk needs the file store (ITSM_FILES_DIR)", nil)
		return
	}
	if _, err := s.o.Records.Type(r.Context(), typeName); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	idemKey, ok := s.idempotencyKey(w, r, typeName, p)
	if !ok {
		return
	}
	sum := sha256.New()
	if idemKey != "" {
		if _, err := s.o.KV.Get(r.Context(), idemKey); err == nil {
			// Known key: only the checksum of the stream is needed to answer.
			if _, err := io.Copy(sum, r.Body); err != nil {
				writeProblem(w, r, http.StatusBadRequest, "bad_request", "body: "+err.Error(), nil)
				return
			}
			if s.replay(w, r, idemKey, hex.EncodeToString(sum.Sum(nil))) {
				return
			}
			sum.Reset()
		}
	}
	source := ""
	if p.Source != nil {
		source = *p.Source
	}
	job, err := s.o.Bulk.Start(r.Context(), typeName, source, io.TeeReader(r.Body, sum), actor(r))
	if s.bulkError(w, r, err) {
		return
	}
	body, err := json.Marshal(job)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	location := fmt.Sprintf("/api/v1/jobs/%d", job.ID)
	s.keep(r, idemKey, idempotent{Body: hex.EncodeToString(sum.Sum(nil)), Status: http.StatusAccepted, Location: location, Response: body})
	w.Header().Set("Location", location)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write(body)
}

// bulkError answers the errors of a bulk call; false means there was none.
func (s *server) bulkError(w http.ResponseWriter, r *http.Request, err error) bool {
	var invalid *bulk.InvalidError
	switch {
	case err == nil:
		return false
	case errors.As(err, &invalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{
			Field: invalid.Field, Code: "invalid", Message: invalid.Message}})
	case errors.Is(err, bulk.ErrTooLarge):
		writeProblem(w, r, http.StatusRequestEntityTooLarge, "too_large", err.Error(), nil)
	default:
		writeError(w, r, s.o.Logger, err)
	}
	return true
}

// idempotencyKey is the Valkey key of the request's Idempotency-Key, "" without one; false
// means the answer is written (no Valkey).
func (s *server) idempotencyKey(w http.ResponseWriter, r *http.Request, typeName string, p gen.BulkRecordsParams) (string, bool) {
	if p.IdempotencyKey == nil {
		return "", true
	}
	if s.o.KV == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "Idempotency-Key needs Valkey, which this instance does not have", nil)
		return "", false
	}
	// Scoped to the caller and the type, so that two callers cannot see each other's answers.
	who := "anonymous"
	if a := actor(r); a != nil {
		who = (*a).String()
	}
	return "bulk:idem:" + who + ":" + typeName + ":" + checksum([]byte(*p.IdempotencyKey)), true
}

// replay answers a known key: the kept answer for the same body, 422 for another. false means
// the key is not known (or no longer readable) and the request runs.
func (s *server) replay(w http.ResponseWriter, r *http.Request, idemKey, bodySum string) bool {
	kept, err := s.o.KV.Get(r.Context(), idemKey)
	if errors.Is(err, cache.ErrMiss) {
		return false
	}
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return true
	}
	var prev idempotent
	if json.Unmarshal(kept, &prev) != nil {
		return false
	}
	if prev.Body != bodySum {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{
			Field: "Idempotency-Key", Code: "reused", Message: "this key was used for another request within 24 hours"}})
		return true
	}
	status := prev.Status
	if status == 0 {
		status = http.StatusOK
	}
	if prev.Location != "" {
		w.Header().Set("Location", prev.Location)
	}
	w.Header().Set("Idempotent-Replayed", "true")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(prev.Response)
	return true
}

// keep stores the answer to a key for 24 hours.
func (s *server) keep(r *http.Request, idemKey string, v idempotent) {
	if idemKey == "" {
		return
	}
	kept, _ := json.Marshal(v)
	if err := s.o.KV.Set(r.Context(), idemKey, kept, idempotencyTTL); err != nil {
		s.o.Logger.Warn("bulk: keep the idempotency key", "error", err.Error())
	}
}

func checksum(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
