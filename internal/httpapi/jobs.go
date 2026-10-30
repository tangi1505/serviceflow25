// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/bulk"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/jobs"
)

// --- jobs (PL23) ---
//
// The log and the schedules are read with the action "read" on the type "core.job", changes
// need "manage_jobs" on it (admin has both through * *). Job arguments may carry personal
// data, so there is no reading for everyone. One exception: whoever started an asynchronous
// bulk run may read its job, to follow it (PL32).

const jobsType = "core.job"

func (s *server) jobsReady(w http.ResponseWriter, r *http.Request, action string) bool {
	if s.o.Jobs == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "jobs are not configured on this instance", nil)
		return false
	}
	if a := authz.From(r.Context()); !a.Allowed(jobsType, action) {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "jobs need the right "+action+" on core.job", nil)
		return false
	}
	return true
}

func (s *server) ListJobs(w http.ResponseWriter, r *http.Request, params gen.ListJobsParams) {
	if !s.jobsReady(w, r, authz.ActionRead) {
		return
	}
	var f jobs.Filter
	if params.Queue != nil {
		f.Queue = *params.Queue
	}
	if params.Kind != nil {
		f.Kind = *params.Kind
	}
	if params.State != nil {
		f.State = string(*params.State)
	}
	if params.Limit != nil {
		f.Limit = *params.Limit
	}
	f.Installation = authz.From(r.Context()).Installation(jobsType, authz.ActionRead)
	list, err := s.o.Jobs.List(r.Context(), f)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetJob(w http.ResponseWriter, r *http.Request, jobID gen.JobId) {
	if s.o.Jobs != nil && !authz.From(r.Context()).Allowed(jobsType, authz.ActionRead) {
		// Not the log: the own bulk run only, and the same answer as for no job at all.
		job, err := s.o.Jobs.Get(r.Context(), jobID)
		if err == nil && ownBulkRun(r, job) {
			writeJSON(w, http.StatusOK, job)
			return
		}
	}
	if !s.jobsReady(w, r, authz.ActionRead) || !s.jobInTenant(w, r, jobID) {
		return
	}
	job, err := s.o.Jobs.Get(r.Context(), jobID)
	s.jobOrError(w, r, job, err)
}

// jobInTenant answers 404 for a job of another tenant, or of the system without installation
// (PL45).
func (s *server) jobInTenant(w http.ResponseWriter, r *http.Request, jobID int64) bool {
	job, err := s.o.Jobs.Get(r.Context(), jobID)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	if !jobs.Visible(r.Context(), job.Args, authz.From(r.Context()).Installation(jobsType, authz.ActionRead)) {
		writeError(w, r, s.o.Logger, fmt.Errorf("%w: job %d", jobs.ErrNotFound, jobID))
		return false
	}
	return true
}

// ownBulkRun reports whether job is an asynchronous bulk run the caller started.
func ownBulkRun(r *http.Request, job jobs.Job) bool {
	if job.Kind != bulk.KindApply {
		return false
	}
	var args bulk.ApplyArgs
	a := actor(r)
	return a != nil && json.Unmarshal(job.Args, &args) == nil && args.Account == *a
}

func (s *server) CancelJob(w http.ResponseWriter, r *http.Request, jobID gen.JobId) {
	if !s.jobsReady(w, r, "manage_jobs") || !s.jobInTenant(w, r, jobID) {
		return
	}
	job, err := s.o.Jobs.Cancel(r.Context(), jobID)
	s.jobOrError(w, r, job, err)
}

func (s *server) RetryJob(w http.ResponseWriter, r *http.Request, jobID gen.JobId) {
	if !s.jobsReady(w, r, "manage_jobs") || !s.jobInTenant(w, r, jobID) {
		return
	}
	job, err := s.o.Jobs.Retry(r.Context(), jobID)
	s.jobOrError(w, r, job, err)
}

func (s *server) jobOrError(w http.ResponseWriter, r *http.Request, job jobs.Job, err error) {
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *server) ListJobSchedules(w http.ResponseWriter, r *http.Request) {
	if !s.jobsReady(w, r, authz.ActionRead) {
		return
	}
	list, err := s.o.Jobs.Schedules(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetJobSchedule(w http.ResponseWriter, r *http.Request, name gen.ScheduleName) {
	if !s.jobsReady(w, r, authz.ActionRead) {
		return
	}
	sched, err := s.o.Jobs.ScheduleByName(r.Context(), name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

func (s *server) UpdateJobSchedule(w http.ResponseWriter, r *http.Request, name gen.ScheduleName) {
	if !s.jobsReady(w, r, "manage_jobs") {
		return
	}
	var body gen.JobSchedulePatch
	if !decode(w, r, &body) {
		return
	}
	sched, err := s.o.Jobs.SetSchedule(r.Context(), name, jobs.SchedulePatch{Enabled: body.Enabled, Schedule: body.Schedule}, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, sched)
}

func (s *server) RunJobSchedule(w http.ResponseWriter, r *http.Request, name gen.ScheduleName) {
	if !s.jobsReady(w, r, "manage_jobs") {
		return
	}
	job, err := s.o.Jobs.RunNow(r.Context(), name)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
