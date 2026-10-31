// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/riverqueue/river"

	"group24.de/itsm/internal/jobs"
)

// LDAPSyncArgs is the job that syncs one directory, or every enabled one when Directory is
// empty (the hourly schedule, E89). It runs in the queue bulk: a large directory takes a
// while and must not hold interactive work.
type LDAPSyncArgs struct {
	Directory string `json:"directory,omitempty"`
}

// Kind is the job kind and the name of the schedule.
func (LDAPSyncArgs) Kind() string { return "ldap.sync" }

type ldapSyncWorker struct {
	river.WorkerDefaults[LDAPSyncArgs]
	svc *Service
}

func (w *ldapSyncWorker) Work(ctx context.Context, job *river.Job[LDAPSyncArgs]) error {
	if job.Args.Directory != "" {
		res, err := w.svc.Sync(ctx, job.Args.Directory)
		if err != nil {
			return err
		}
		return river.RecordOutput(ctx, res)
	}
	results, err := w.svc.SyncAll(ctx)
	if len(results) > 0 {
		if rerr := river.RecordOutput(ctx, results); rerr != nil && err == nil {
			err = rerr
		}
	}
	return err
}

// RegisterSyncJob adds the worker and the hourly schedule of the directory sync.
func RegisterSyncJob(js *jobs.Service, svc *Service) error {
	jobs.AddWorker(js, &ldapSyncWorker{svc: svc})
	return js.AddPeriodic(jobs.Periodic{
		Name:     LDAPSyncArgs{}.Kind(),
		Label:    map[string]string{"de": "Verzeichnisse abgleichen (LDAP)", "en": "Sync directories (LDAP)"},
		Schedule: "0 * * * *",
		// Directories and login rules are central; the sync puts each account in its tenant.
		Central: true,
		Queue:   jobs.QueueBulk,
		Args:    func() river.JobArgs { return LDAPSyncArgs{} },
	})
}

// EnqueueSync queues the sync of one directory right away and returns the job.
func (s *Service) EnqueueSync(ctx context.Context, js *jobs.Service, name string) (jobs.Job, error) {
	if _, err := s.Directory(ctx, name); err != nil {
		return jobs.Job{}, err
	}
	if js == nil {
		return jobs.Job{}, errors.New("authn: jobs are not configured")
	}
	return js.Enqueue(ctx, LDAPSyncArgs{Directory: name}, &river.InsertOpts{Queue: jobs.QueueBulk})
}

// String renders a result for the command line.
func (r SyncResult) String() string {
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Sprintf("%s: persons %+v, groups %+v, members %+v", r.Directory, r.Persons, r.Groups, r.Members)
	}
	return string(raw)
}
