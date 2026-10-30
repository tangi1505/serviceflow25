// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package files

import (
	"context"

	"github.com/riverqueue/river"

	"group24.de/itsm/internal/jobs"
)

// CleanupArgs is the hourly job that removes broken uploads and unattached temporary files
// (Inventar G245; PL18).
type CleanupArgs struct{}

// Kind is the job kind and the name of the schedule.
func (CleanupArgs) Kind() string { return "files.cleanup" }

type cleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	svc *Service
}

func (w *cleanupWorker) Work(ctx context.Context, _ *river.Job[CleanupArgs]) error {
	n, err := w.svc.Cleanup(ctx)
	if err != nil {
		return err
	}
	w.svc.logger.Info("files cleaned up", "removed", n)
	return river.RecordOutput(ctx, map[string]int{"removed": n})
}

// RegisterCleanupJob adds the cleanup job: its worker and the hourly schedule.
func RegisterCleanupJob(js *jobs.Service, svc *Service) error {
	jobs.AddWorker(js, &cleanupWorker{svc: svc})
	return js.AddPeriodic(jobs.Periodic{
		Name:     CleanupArgs{}.Kind(),
		Label:    map[string]string{"de": "Temporäre Dateien aufräumen", "en": "Clean up temporary files"},
		Schedule: "15 * * * *",
		Queue:    jobs.QueueBulk,
		Args:     func() river.JobArgs { return CleanupArgs{} },
	})
}
