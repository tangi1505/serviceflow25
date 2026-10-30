// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// --- batch of changes (UI6, Hauptplan E126) ---
//
// A batch changes chosen records by their id, each at the version the caller saw: fields
// (roles under actors), a transition (with fields) or the trash. Every row goes through the
// same path as a single call, with rights, field states, rules, history and event, and stands
// alone: one that fails leaves the others written. The bulk (E24) stays the path of sources.

// MaxBatch is the most rows of one batch.
const MaxBatch = 1000

// BatchRow is one change of a batch.
type BatchRow struct {
	ID         uuid.UUID      `json:"id"`
	Version    int64          `json:"version"`
	Set        map[string]any `json:"set,omitempty"`
	Transition string         `json:"transition,omitempty"`
	Trash      bool           `json:"trash,omitempty"`
}

// Outcomes of a row.
const (
	BatchOK        = "ok"
	BatchConflict  = "conflict"  // the record has another version now
	BatchRefused   = "refused"   // the transition does not apply
	BatchForbidden = "forbidden" // no right for this change
	BatchNotFound  = "not_found" // gone or not visible
	BatchInvalid   = "invalid"   // the values break the rules of the type
	BatchError     = "error"     // anything else; the batch goes on
)

// BatchResult is the outcome of one row.
type BatchResult struct {
	ID       uuid.UUID `json:"id"`
	Status   string    `json:"status"`
	Version  int64     `json:"version,omitempty"`
	Message  string    `json:"message,omitempty"`
	Problems []Problem `json:"problems,omitempty"`
}

// Batch applies the rows one after the other.
func (s *Service) Batch(ctx context.Context, typeName string, rows []BatchRow, actor Actor) ([]BatchResult, error) {
	if _, err := s.Type(ctx, typeName); err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) > MaxBatch {
		return nil, &ValidationError{Problems: []Problem{{Field: "rows", Code: "count", Message: "a batch has 1 to 1000 rows"}}}
	}
	out := make([]BatchResult, len(rows))
	for i, row := range rows {
		out[i] = s.batchRow(ctx, typeName, row, actor)
	}
	return out, nil
}

func (s *Service) batchRow(ctx context.Context, typeName string, row BatchRow, actor Actor) BatchResult {
	res := BatchResult{ID: row.ID}
	kinds := 0
	if row.Transition != "" {
		kinds++
	}
	if row.Trash {
		kinds++
	}
	if len(row.Set) > 0 && row.Transition == "" {
		kinds++
	}
	if kinds != 1 || row.Version < 1 {
		res.Status, res.Message = BatchInvalid, "a row has a version and one change: set, transition (with set as its fields) or trash"
		return res
	}
	var (
		rec Record
		err error
	)
	switch {
	case row.Trash:
		err = s.TrashVersion(ctx, typeName, row.ID, row.Version, actor)
		if err == nil {
			res.Version = row.Version + 1
		}
	case row.Transition != "":
		rec, err = s.Transition(ctx, typeName, row.ID, row.Transition, row.Set, row.Version, actor)
	default:
		rec, err = s.Update(ctx, typeName, row.ID, row.Version, row.Set, actor)
	}
	if err == nil {
		res.Status = BatchOK
		if v, ok := rec["version"].(int64); ok {
			res.Version = v
		}
		return res
	}
	res.Message = err.Error()
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		res.Status, res.Problems = BatchInvalid, verr.Problems
	case errors.Is(err, ErrNotAllowed):
		res.Status = BatchRefused
	case errors.Is(err, ErrConflict):
		res.Status = BatchConflict
	case errors.Is(err, ErrForbidden):
		res.Status = BatchForbidden
	case errors.Is(err, ErrNotFound):
		res.Status = BatchNotFound
	default:
		res.Status = BatchError
	}
	return res
}
