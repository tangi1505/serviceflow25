// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
)

// --- the trash empties itself (PL76, Kern-Plan E78, Hauptplan V43) ---
//
// A record stays in the trash for trash.keep_days of the node it belongs to (default 30; 0
// keeps it until someone purges it). A job once a night purges what is older, page by page
// through the partial index of the trash (PL74), each record in a transaction of its own, the
// same way as a purge by hand: with the history, the event, the clocks, the timeline, the
// roles and the attachments. The actor is the system.

// SettingTrashKeepDays is the setting of the days a record stays in the trash.
const SettingTrashKeepDays = "trash.keep_days"

// defaultTrashKeepDays holds when no node and no declaration says otherwise.
const defaultTrashKeepDays = 30

// trashBatch is the size of one page of candidates.
const trashBatch = 200

// EmptyTrashArgs is the nightly job.
type EmptyTrashArgs struct{}

// Kind names the job.
func (EmptyTrashArgs) Kind() string { return "trash.empty" }

// RegisterEmptyTrashJob adds the nightly job and its worker.
func RegisterEmptyTrashJob(js *jobs.Service, s *Service, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	jobs.AddWorker(js, &emptyTrashWorker{s: s, logger: logger})
	return js.AddPeriodic(jobs.Periodic{
		Name:     EmptyTrashArgs{}.Kind(),
		Label:    map[string]string{"de": "Papierkorb nach der Frist leeren", "en": "Empty the trash after its period"},
		Schedule: "45 2 * * *",
		Queue:    jobs.QueueDefault,
		Args:     func() river.JobArgs { return EmptyTrashArgs{} },
	})
}

type emptyTrashWorker struct {
	river.WorkerDefaults[EmptyTrashArgs]
	s      *Service
	logger *slog.Logger
}

// Work purges what has been in the trash longer than its period.
func (w *emptyTrashWorker) Work(ctx context.Context, _ *river.Job[EmptyTrashArgs]) error {
	n, err := w.s.EmptyTrash(ctx, time.Now())
	if n > 0 {
		w.logger.Info("trash emptied", "purged", n)
	}
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"purged": n})
}

// EmptyTrash purges, as the system, every record that has been in the trash at now longer than
// trash.keep_days of its node, and returns how many. Tests pass their own now.
func (s *Service) EmptyTrash(ctx context.Context, now time.Time) (int, error) {
	ctx = authz.WithAccess(ctx, authz.System())
	types, err := s.Types(ctx)
	if err != nil {
		return 0, err
	}
	keep := map[string]int{}
	keepOf := func(path string) (int, error) {
		if d, ok := keep[path]; ok {
			return d, nil
		}
		d, err := s.trashKeepDays(ctx, path)
		if err != nil {
			return 0, err
		}
		keep[path] = d
		return d, nil
	}
	purged := 0
	for _, t := range types {
		// Nothing leaves the trash before a day: 1 is the shortest period but 0 (never).
		before := now.Add(-24 * time.Hour)
		var after *trashCursor
		for {
			page, err := s.trashCandidates(ctx, t, before, after)
			if err != nil {
				return purged, err
			}
			for _, c := range page {
				days, err := keepOf(c.path)
				if err != nil {
					return purged, err
				}
				if days <= 0 || c.at.After(now.AddDate(0, 0, -days)) {
					continue
				}
				// Another instance, or a person, may have purged or restored it meanwhile.
				err = s.Purge(ctx, t.QualifiedName(), c.id, nil)
				if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
					continue
				}
				if err != nil {
					return purged, fmt.Errorf("records: empty trash %s %s: %w", t.QualifiedName(), c.id, err)
				}
				purged++
			}
			if len(page) < trashBatch {
				break
			}
			last := page[len(page)-1]
			after = &trashCursor{at: last.at, id: last.id}
		}
	}
	return purged, nil
}

type trashCandidate struct {
	id   uuid.UUID
	path string
	at   time.Time
}

// trashCandidates reads one page of records of a type that went to the trash before before,
// oldest first, after the cursor.
func (s *Service) trashCandidates(ctx context.Context, t meta.Type, before time.Time, after *trashCursor) ([]trashCandidate, error) {
	where := "deleted_at IS NOT NULL AND deleted_at <= $1"
	args := []any{before}
	if c := listScope(t, ""); c != "" {
		where += " AND " + c
	}
	if after != nil {
		where += " AND (deleted_at, id) > ($2, $3)"
		args = append(args, after.at, after.id)
	}
	rows, err := s.db.Tenant(ctx).Query(ctx, fmt.Sprintf(
		"SELECT id, org_path::text, deleted_at FROM %s WHERE %s ORDER BY deleted_at, id LIMIT %d", table(t), where, trashBatch), args...)
	if err != nil {
		return nil, fmt.Errorf("records: trash of %s: %w", t.QualifiedName(), err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (trashCandidate, error) {
		var c trashCandidate
		return c, row.Scan(&c.id, &c.path, &c.at)
	})
}

// trashKeepDays is trash.keep_days at a node: set there or above, else the declaration, else 30.
func (s *Service) trashKeepDays(ctx context.Context, path string) (int, error) {
	if s.SettingValue == nil {
		return defaultTrashKeepDays, nil
	}
	raw, _, err := s.SettingValue(ctx, path, SettingTrashKeepDays)
	if err != nil {
		return 0, fmt.Errorf("records: %s at %s: %w", SettingTrashKeepDays, path, err)
	}
	if len(raw) == 0 || string(raw) == "null" {
		return defaultTrashKeepDays, nil
	}
	var days float64
	if err := json.Unmarshal(raw, &days); err != nil {
		return 0, fmt.Errorf("records: %s at %s: %w", SettingTrashKeepDays, path, err)
	}
	return int(days), nil
}
