// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package search is the interface of the full-text search (Hauptplan E21, E101; PL26). The
// API talks to an Engine; the one the core ships is PostgreSQL (tsvector, German dictionary,
// trigram) through the record service, which owns the type tables (E34). Meilisearch or
// OpenSearch can come as another Engine without touching the API.
package search

import (
	"context"

	"group24.de/itsm/internal/records"
)

// Query and Hit are the ones of the record service; another engine fills them the same way.
type (
	Query = records.SearchQuery
	Hit   = records.SearchHit
)

// Engine finds records for the session in the context, with its rights.
type Engine interface {
	Search(ctx context.Context, q Query) ([]Hit, error)
}

// Postgres is the engine of the core.
type Postgres struct{ rec *records.Service }

// NewPostgres returns the PostgreSQL engine.
func NewPostgres(rec *records.Service) *Postgres { return &Postgres{rec: rec} }

// Search runs the query in PostgreSQL.
func (p *Postgres) Search(ctx context.Context, q Query) ([]Hit, error) { return p.rec.Search(ctx, q) }
