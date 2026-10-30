// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"errors"
	"fmt"
	"strings"
)

// Errors the HTTP layer maps to status codes (PL4, Hauptplan E71).
var (
	// ErrNotFound: no such type, or no such record outside the trash.
	ErrNotFound = errors.New("not found")
	// ErrConflict: the record changed since the version the caller saw (Hauptplan E29), or a
	// restore collides with a unique value.
	ErrConflict = errors.New("conflict")
)

// Problem is one validation finding on one field.
type Problem struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationError lists every problem of one input at once.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Field + ": " + p.Message
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(parts, "; "))
}

// AsValidationError returns the validation error inside err, if any.
func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}
