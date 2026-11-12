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
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/jobs"
)

// --- reminders of open approvals (WF7, WF-Plan E18; Inventar G67) ---
//
// When a stage is asked, it gets the moment of its first reminder: the working days of the
// approval later (remindAt). A job every minute takes the stages due, reminds whoever in them
// has not decided yet (event approval_reminder with the approvers, a row of the history) and
// sets the next moment, until the stage is decided. A record in the trash, or a stage without
// an open question, is not reminded again.

// EventApprovalReminder is the event of a reminder; its payload names the approvers.
const EventApprovalReminder = "approval_reminder"

// remindBatch bounds the stages one run reminds.
const remindBatch = 200

// RemindArgs is the job every minute.
type RemindArgs struct{}

// Kind names the job.
func (RemindArgs) Kind() string { return "approval.remind" }

// RegisterRemindJob adds the job every minute and its worker.
func RegisterRemindJob(js *jobs.Service, s *Service, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	jobs.AddWorker(js, &remindWorker{s: s, logger: logger})
	return js.AddPeriodic(jobs.Periodic{
		Name:     RemindArgs{}.Kind(),
		Label:    map[string]string{"de": "Erinnerung an Genehmigungen", "en": "Approval reminders"},
		Schedule: "* * * * *",
		Queue:    jobs.QueueDefault,
		Args:     func() river.JobArgs { return RemindArgs{} },
	})
}

type remindWorker struct {
	river.WorkerDefaults[RemindArgs]
	s      *Service
	logger *slog.Logger
}

// Work reminds the stages due now.
func (w *remindWorker) Work(ctx context.Context, _ *river.Job[RemindArgs]) error {
	n, err := w.s.RemindApprovals(ctx, time.Now())
	if err != nil {
		return err
	}
	return river.RecordOutput(ctx, map[string]any{"reminded": n})
}

// RemindApprovals reminds the open stages whose moment has come at now and returns how many.
func (s *Service) RemindApprovals(ctx context.Context, now time.Time) (int, error) {
	ctx = authz.WithAccess(ctx, authz.System())
	rows, err := s.db.Tenant(ctx).Query(ctx, `SELECT sg.approval_id, sg.position FROM approval_stage sg JOIN approval ap ON ap.id = sg.approval_id
WHERE sg.state = 'open' AND ap.state = 'open' AND sg.remind_at <= $1 ORDER BY sg.remind_at LIMIT $2`, now, remindBatch)
	if err != nil {
		return 0, fmt.Errorf("records: reminders: %w", err)
	}
	type due struct {
		approval uuid.UUID
		stage    int
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (due, error) {
		var d due
		return d, row.Scan(&d.approval, &d.stage)
	})
	if err != nil {
		return 0, fmt.Errorf("records: reminders: %w", err)
	}
	n := 0
	for _, d := range list {
		done, err := s.remindStage(ctx, d.approval, d.stage, now)
		if err != nil {
			return n, err
		}
		if done {
			n++
		}
	}
	return n, nil
}

// remindStage reminds one stage when it is still due; another instance may have done it.
func (s *Service) remindStage(ctx context.Context, approvalID uuid.UUID, stage int, now time.Time) (bool, error) {
	done := false
	typeName := ""
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var (
			rid      uuid.UUID
			days     *int
			at       time.Time
			reminded int
		)
		err := tx.QueryRow(ctx, `SELECT ap.type_name, ap.record_id, ap.remind_days, sg.remind_at, sg.reminders
FROM approval_stage sg JOIN approval ap ON ap.id = sg.approval_id
WHERE sg.approval_id = $1 AND sg.position = $2 AND sg.state = 'open' AND ap.state = 'open' AND sg.remind_at <= $3
FOR UPDATE OF sg SKIP LOCKED`, approvalID, stage, now).Scan(&typeName, &rid, &days, &at, &reminded)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("records: reminder: %w", err)
		}
		stop := func() error {
			_, err := tx.Exec(ctx, `UPDATE approval_stage SET remind_at = NULL WHERE approval_id = $1 AND position = $2`, approvalID, stage)
			return err
		}
		t, err := s.approvalType(ctx, typeName)
		if err != nil {
			return stop()
		}
		rec, err := s.read(ctx, tx, t, rid, false, false)
		if errors.Is(err, ErrNotFound) {
			return stop()
		}
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT approver_type, approver_id FROM approval_step
WHERE approval_id = $1 AND stage = $2 AND state = 'open' ORDER BY position`, approvalID, stage)
		if err != nil {
			return fmt.Errorf("records: reminder: %w", err)
		}
		asked, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Ref, error) {
			var r Ref
			return r, row.Scan(&r.Type, &r.ID)
		})
		if err != nil {
			return fmt.Errorf("records: reminder: %w", err)
		}
		if len(asked) == 0 {
			return stop()
		}
		// The next moment counts from this one, so that a late run does not shift the rhythm;
		// after a long pause it counts from now.
		next := s.remindAt(ctx, rec, days, at)
		if next != nil && !next.After(now) {
			next = s.remindAt(ctx, rec, days, now)
		}
		reminded++
		if _, err := tx.Exec(ctx, `UPDATE approval_stage SET reminders = $3, reminded_at = $4, remind_at = $5 WHERE approval_id = $1 AND position = $2`,
			approvalID, stage, reminded, now, next); err != nil {
			return fmt.Errorf("records: reminder: %w", err)
		}
		done = true
		entry := map[string]any{"approval": approvalID.String(), "stage": stage, "reminder": reminded, "approvers": asked}
		payload := map[string]any{"approval": approvalID.String(), "stage": stage, "reminder": reminded, "approvers": asked}
		return s.approvalWritten(ctx, tx, t, rid, rec, "", EventApprovalReminder, entry, payload, nil)
	})
	if typeName != "" {
		err = s.written(ctx, typeName, err)
	}
	return done, err
}
