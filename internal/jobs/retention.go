// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/db/queries"
)

// EventRetentionArgs is the nightly job that deletes events older than the retention
// (Hauptplan E46: 7 days, adjustable with ITSM_EVENT_RETENTION). PL24 builds the stream and
// its 410 on it. It is the first of the automatic actions of G183.
type EventRetentionArgs struct{}

// Kind is the job kind and the name of the schedule.
func (EventRetentionArgs) Kind() string { return "event.retention" }

type eventRetentionWorker struct {
	river.WorkerDefaults[EventRetentionArgs]
	pool   *pgxpool.Pool
	keep   time.Duration
	logger *slog.Logger
	// db runs the delete; nil means the pool (a test passes its transaction).
	db queries.DBTX
}

func (w *eventRetentionWorker) Work(ctx context.Context, _ *river.Job[EventRetentionArgs]) error {
	before := pgtype.Timestamptz{Time: time.Now().Add(-w.keep), Valid: true}
	var db queries.DBTX = w.pool
	if w.db != nil {
		db = w.db
	}
	n, err := queries.New(db).DeleteEventsBefore(ctx, before)
	if err != nil {
		return fmt.Errorf("event retention: %w", err)
	}
	w.logger.Info("events expired", "deleted", n, "older_than", w.keep.String())
	return nil
}

// RegisterEventRetention adds the retention job: its worker and the nightly schedule.
func RegisterEventRetention(s *Service, keep time.Duration) error {
	AddWorker(s, &eventRetentionWorker{pool: s.pool, keep: keep, logger: s.o.Logger})
	return s.AddPeriodic(Periodic{
		Name:     EventRetentionArgs{}.Kind(),
		Label:    map[string]string{"de": "Alte Ereignisse entfernen", "en": "Expire old events"},
		Schedule: "30 2 * * *",
		Queue:    QueueDefault,
		Args:     func() river.JobArgs { return EventRetentionArgs{} },
	})
}
