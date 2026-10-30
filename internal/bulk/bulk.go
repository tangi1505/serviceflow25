// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package bulk is the bulk interface of the records (Hauptplan E24, E38; architektur.md §6.4):
// a source delivers many records at once under its own keys, and only what changed is
// written. PL31 is the synchronous call of up to 1,000 rows with a result per row; the
// asynchronous stream (PL32), the full state of a source (PL33) and field ownership (PL34)
// build on it.
package bulk

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/records"
)

// MaxRows is the most rows of one synchronous call; more go through the asynchronous bulk.
const MaxRows = 1000

// KeyField is the field of a row with the source's key of the record.
const KeyField = "external_key"

// InvalidError is a call that cannot run at all (no source, no rows, too many rows); Field
// names the part of the request.
type InvalidError struct{ Field, Message string }

func (e *InvalidError) Error() string { return "bulk: " + e.Field + ": " + e.Message }

// Writer writes one row; records.Service is the one.
type Writer interface {
	Sync(ctx context.Context, typeName string, key records.SyncKey, fields map[string]any, actor records.Actor) (uuid.UUID, int64, string, error)
}

// Result is what happened to one row.
type Result struct {
	Index       int               `json:"index"`
	ExternalKey string            `json:"external_key,omitempty"`
	Status      string            `json:"status"` // created, updated, unchanged or error
	ID          *uuid.UUID        `json:"id,omitempty"`
	Version     int64             `json:"version,omitempty"`
	Code        string            `json:"code,omitempty"` // with error: validation, forbidden, conflict
	Message     string            `json:"message,omitempty"`
	Problems    []records.Problem `json:"problems,omitempty"`
}

// Outcome is the answer of a call: a result per row in the order of the rows, and the counts.
type Outcome struct {
	Items     []Result `json:"items"`
	Created   int      `json:"created"`
	Updated   int      `json:"updated"`
	Unchanged int      `json:"unchanged"`
	Errors    int      `json:"errors"`
}

// StatusError is the status of a row that was not written.
const StatusError = "error"

// Sync writes the rows of one source one after the other. A row that fails gets its error and
// the others go on; an error of the database itself stops the call.
func Sync(ctx context.Context, w Writer, typeName, source string, rows []map[string]any, actor records.Actor) (Outcome, error) {
	source = strings.TrimSpace(source)
	switch {
	case source == "" || len(source) > 100:
		return Outcome{}, &InvalidError{"source", "must be 1 to 100 characters"}
	case len(rows) == 0:
		return Outcome{}, &InvalidError{"rows", "no rows"}
	case len(rows) > MaxRows:
		return Outcome{}, &InvalidError{"rows", fmt.Sprintf("%d rows, at most %d in one call; send more as a stream (asynchronous bulk)", len(rows), MaxRows)}
	}
	out := Outcome{Items: make([]Result, 0, len(rows))}
	for i, row := range rows {
		res := Result{Index: i}
		key, fields, err := split(row)
		res.ExternalKey = key
		if err == nil {
			var (
				id     uuid.UUID
				status string
			)
			id, res.Version, status, err = w.Sync(ctx, typeName, records.SyncKey{Source: source, Key: key, Hash: Hash(fields)}, fields, actor)
			if err == nil {
				res.Status, res.ID = status, &id
			}
		}
		if err != nil {
			if !rowError(&res, err) {
				return Outcome{}, fmt.Errorf("bulk: row %d: %w", i, err)
			}
			res.Version = 0
		}
		switch res.Status {
		case records.SyncCreated:
			out.Created++
		case records.SyncUpdated:
			out.Updated++
		case records.SyncUnchanged:
			out.Unchanged++
		default:
			out.Errors++
		}
		out.Items = append(out.Items, res)
	}
	return out, nil
}

// split takes the external key out of a row; the rest are the fields.
func split(row map[string]any) (string, map[string]any, error) {
	raw, ok := row[KeyField]
	key, isText := raw.(string)
	key = strings.TrimSpace(key)
	if !ok || !isText || key == "" {
		return "", nil, &records.ValidationError{Problems: []records.Problem{{Field: KeyField, Code: "required", Message: "every row needs the key of the record at its source"}}}
	}
	fields := make(map[string]any, len(row)-1)
	for k, v := range row {
		if k != KeyField {
			fields[k] = v
		}
	}
	return key, fields, nil
}

// Hash is the checksum of the fields a row delivers: SHA-256 over their JSON with the keys in
// order, so that the same values give the same hash however they were sent.
func Hash(fields map[string]any) []byte {
	raw, _ := json.Marshal(fields) // values decoded from JSON always marshal again
	sum := sha256.Sum256(raw)
	return sum[:]
}

// rowError fills the result of a row that was refused; false means the error is not the
// row's fault.
func rowError(res *Result, err error) bool {
	res.Status, res.ID = StatusError, nil
	switch {
	case errors.Is(err, records.ErrForbidden):
		res.Code = "forbidden"
	case errors.Is(err, records.ErrNotFound):
		// A record the caller may not read is not there for the caller.
		res.Code = "not_found"
	case errors.Is(err, records.ErrConflict), errors.Is(err, records.ErrNotAllowed):
		res.Code = "conflict"
	default:
		ve, ok := records.AsValidationError(err)
		if !ok {
			return false
		}
		res.Code, res.Problems = "validation", ve.Problems
		return true
	}
	res.Message = err.Error()
	return true
}
