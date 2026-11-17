// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package jobs runs background work with River in PostgreSQL (Hauptplan E19, E85; PL23):
// queues with precedence, periodic jobs on cron schedules that a table may override, and the
// log of runs for the API. Jobs are inserted in the transaction of the change that causes
// them (EnqueueTx), so a job never exists without its change and vice versa. With databases of
// their own for the tenants every database has its River client in this process (PL47c): a job
// lives and runs in the database of the change that queued it, bound to it (db.Database.Bind).
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
)

// The queues (Hauptplan E85, LZ10): work behind the interface never waits behind bulk runs
// because every queue has workers of its own. The day values of the dashboards have theirs
// (PL70, Pascal 2026-11-17): behind a flood of incidents in "default" their run of the minute
// did not come for more than an hour, and the dashboards showed old numbers.
const (
	QueueInteractive = "interactive"
	QueueDefault     = "default"
	QueueBulk        = "bulk"
	QueueDays        = "days"
)

// Queues lists the standard queues in order of precedence. Other names are allowed (an
// installation may add queues with workers of their own; tests use queues no server works).
var Queues = []string{QueueInteractive, QueueDefault, QueueBulk, QueueDays}

// reloadChannel carries the name of a changed schedule to every instance (Valkey Pub/Sub).
const reloadChannel = "jobs.schedule"

var (
	// ErrNotFound says there is no such job or schedule.
	ErrNotFound = errors.New("not found")
	// ErrInvalid says the request cannot be fulfilled as given (bad schedule, unknown queue
	// or state); it answers as 422.
	ErrInvalid = errors.New("invalid")
)

// Periodic is a job the worker enqueues on a schedule (G183). Name is also the job kind.
type Periodic struct {
	Name  string
	Label map[string]string
	// Schedule is the default cron expression; the table job_schedule may override it.
	Schedule string
	Queue    string
	Args     func() river.JobArgs
	// Central runs the job in the control database only (PL47c): work that picks its databases
	// itself, like the LDAP sync. Every other periodic job runs in each database.
	Central bool
}

// Schedule is a periodic job as the API shows it: defaults and effective values.
type Schedule struct {
	Name            string            `json:"name"`
	Label           map[string]string `json:"label,omitempty"`
	Queue           string            `json:"queue"`
	DefaultSchedule string            `json:"default_schedule"`
	Schedule        string            `json:"schedule"`
	Enabled         bool              `json:"enabled"`
	NextRun         *time.Time        `json:"next_run,omitempty"`
	LastRun         *Job              `json:"last_run,omitempty"`
	UpdatedAt       *time.Time        `json:"updated_at,omitempty"`
	UpdatedBy       *uuid.UUID        `json:"updated_by,omitempty"`
}

// SchedulePatch changes a schedule; nil fields keep their value.
type SchedulePatch struct {
	Enabled  *bool
	Schedule *string
}

// Job is one row of the log as the API shows it (River's river_job without traces).
type Job struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	Queue       string          `json:"queue"`
	State       string          `json:"state"`
	Priority    int             `json:"priority"`
	Attempt     int             `json:"attempt"`
	MaxAttempts int             `json:"max_attempts"`
	CreatedAt   time.Time       `json:"created_at"`
	ScheduledAt time.Time       `json:"scheduled_at"`
	AttemptedAt *time.Time      `json:"attempted_at,omitempty"`
	FinalizedAt *time.Time      `json:"finalized_at,omitempty"`
	Args        json.RawMessage `json:"args"`
	Errors      []JobError      `json:"errors,omitempty"`
	// Output is what the job reported, its progress while it runs (the asynchronous bulk,
	// PL32) and its result when it is done.
	Output json.RawMessage `json:"output,omitempty"`
}

// JobError is one failed attempt.
type JobError struct {
	At      time.Time `json:"at"`
	Attempt int       `json:"attempt"`
	Error   string    `json:"error"`
}

// Filter narrows the list of jobs.
type Filter struct {
	Queue string
	Kind  string
	State string
	Limit int
	// Installation shows a session in a tenant also the jobs of no tenant (PL45).
	Installation bool
}

// Options configure the service.
type Options struct {
	Logger *slog.Logger
	// Workers per queue; nil or empty means insert only (the api role).
	Workers map[string]int
	// Cache carries schedule changes to the other instances; nil keeps them local.
	Cache *cache.Client
	// Retention of finished jobs in the log (E86): zero means 7 days for completed and
	// cancelled jobs and 30 days for discarded ones.
	CompletedRetention time.Duration
	DiscardedRetention time.Duration
}

// Service is the job runner of one process.
type Service struct {
	db      *db.Router
	o       Options
	workers *river.Workers

	mu       sync.Mutex
	periodic map[string]Periodic
	// clients holds the River client of every database by its name (db.Database.Name).
	clients map[string]*river.Client[pgx.Tx]
	started bool
}

// New prepares a service; workers and periodic jobs are added before Start.
func New(rt *db.Router, o Options) *Service {
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	if o.CompletedRetention == 0 {
		o.CompletedRetention = 7 * 24 * time.Hour
	}
	if o.DiscardedRetention == 0 {
		o.DiscardedRetention = 30 * 24 * time.Hour
	}
	return &Service{db: rt, o: o, workers: river.NewWorkers(), periodic: map[string]Periodic{}}
}

// AddWorker registers the worker of a job kind; call it before Start.
func AddWorker[T river.JobArgs](s *Service, w river.Worker[T]) {
	river.AddWorker(s.workers, w)
}

// AddPeriodic registers a periodic job; call it before Start.
func (s *Service) AddPeriodic(p Periodic) error {
	if p.Name == "" || p.Args == nil || p.Queue == "" {
		return fmt.Errorf("%w: a periodic job needs a name, a queue and args", ErrInvalid)
	}
	if _, err := parseCron(p.Schedule); err != nil {
		return fmt.Errorf("periodic job %s: %w", p.Name, err)
	}
	if kind := p.Args().Kind(); kind != p.Name {
		return fmt.Errorf("%w: periodic job %s: args are of kind %s", ErrInvalid, p.Name, kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.periodic[p.Name]; dup {
		return fmt.Errorf("%w: periodic job %s registered twice", ErrInvalid, p.Name)
	}
	s.periodic[p.Name] = p
	return nil
}

// bind is the middleware that works every job bound to the database of its client (PL47c).
type bind struct {
	river.WorkerMiddlewareDefaults
	to func(context.Context) context.Context
}

func (b *bind) Work(ctx context.Context, _ *rivertype.JobRow, doInner func(context.Context) error) error {
	return doInner(b.to(ctx))
}

// Start builds a River client for every database. With workers they start working their
// queues and enqueuing the periodic jobs (River elects one leader per database for that);
// without workers the clients only insert and read. Start returns at once; Stop ends the work.
func (s *Service) Start(ctx context.Context) error {
	clients := map[string]*river.Client[pgx.Tx]{}
	for _, d := range s.db.Databases() {
		client, err := s.newClient(d)
		if err != nil {
			return err
		}
		clients[d.Name] = client
	}
	s.mu.Lock()
	s.clients = clients
	s.mu.Unlock()
	if len(s.o.Workers) == 0 {
		return nil
	}
	if err := s.reload(ctx); err != nil {
		return err
	}
	for _, d := range s.db.Databases() {
		if err := clients[d.Name].Start(ctx); err != nil {
			return fmt.Errorf("jobs: start in database %q: %w", d.Name, err)
		}
	}
	s.mu.Lock()
	s.started = true
	s.mu.Unlock()
	s.o.Logger.Info("worker started", "queues", s.o.Workers, "periodic", len(s.periodic), "databases", len(clients))
	if s.o.Cache != nil {
		go s.listen(ctx)
	}
	return nil
}

// newClient builds the River client of one database.
func (s *Service) newClient(d db.Database) (*river.Client[pgx.Tx], error) {
	cfg := &river.Config{
		Logger: s.o.Logger,
		// Fetch only kinds this instance works, so that instances with different workers
		// (also the test packages against one database) share the queues without failing
		// each other's jobs.
		FetchOnlyKnownKinds:         true,
		CompletedJobRetentionPeriod: s.o.CompletedRetention,
		CancelledJobRetentionPeriod: s.o.CompletedRetention,
		DiscardedJobRetentionPeriod: s.o.DiscardedRetention,
	}
	if len(s.o.Workers) > 0 {
		cfg.Workers = s.workers
		cfg.Queues = make(map[string]river.QueueConfig, len(s.o.Workers))
		for q, n := range s.o.Workers {
			if q == "" {
				return nil, fmt.Errorf("jobs: %w: empty queue name", ErrInvalid)
			}
			cfg.Queues[q] = river.QueueConfig{MaxWorkers: n}
		}
	}
	cfg.Middleware = []rivertype.Middleware{&bind{to: d.Bind}}
	client, err := river.NewClient(riverpgxv5.New(d.Pool()), cfg)
	if err != nil {
		return nil, fmt.Errorf("jobs: database %q: %w", d.Name, err)
	}
	return client, nil
}

// listen reloads the schedules whenever an instance changed one (PL9 did the same for rights).
func (s *Service) listen(ctx context.Context) {
	for ctx.Err() == nil {
		err := s.o.Cache.Subscribe(ctx, reloadChannel, func(name string) {
			if err := s.reload(ctx); err != nil {
				s.o.Logger.Error("jobs: reload schedules", "changed", name, "error", err.Error())
			}
		})
		if err != nil {
			s.o.Logger.Warn("jobs: schedule subscription lost, retrying", "error", err.Error())
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
	}
}

// Stop ends the work gracefully: running jobs finish within the context.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	clients, started := s.clients, s.started
	s.started = false
	s.mu.Unlock()
	if !started {
		return nil
	}
	var errs []error
	for name, client := range clients {
		if err := client.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("jobs: stop in database %q: %w", name, err))
		}
	}
	s.o.Logger.Info("worker stopped")
	return errors.Join(errs...)
}

// ready is the client of the database the context works in (PL47c): the tenant's for a
// request in a tenant, the bound one for a job.
func (s *Service) ready(ctx context.Context) (*river.Client[pgx.Tx], error) {
	return s.clientOf(s.db.Current(ctx).Name)
}

func (s *Service) clientOf(name string) (*river.Client[pgx.Tx], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		return nil, errors.New("jobs: service not started")
	}
	client, ok := s.clients[name]
	if !ok {
		return nil, fmt.Errorf("jobs: no client for database %q", name)
	}
	return client, nil
}

// reload replaces River's periodic jobs with the effective schedules, in every database; a
// central job only in the control database.
func (s *Service) reload(ctx context.Context) error {
	effective, err := s.effective(ctx)
	if err != nil {
		return err
	}
	for _, d := range s.db.Databases() {
		client, err := s.clientOf(d.Name)
		if err != nil {
			return err
		}
		list := make([]*river.PeriodicJob, 0, len(effective))
		for _, e := range effective {
			p := s.periodicOf(e.Name)
			if !e.Enabled || (p.Central && d.Name != "") {
				continue
			}
			schedule, err := parseCron(e.Schedule)
			if err != nil {
				return err
			}
			list = append(list, river.NewPeriodicJob(schedule, func() (river.JobArgs, *river.InsertOpts) {
				return p.Args(), &river.InsertOpts{Queue: p.Queue}
			}, &river.PeriodicJobOpts{ID: e.Name}))
		}
		bundle := client.PeriodicJobs()
		bundle.Clear()
		bundle.AddMany(list)
	}
	return nil
}

func (s *Service) periodicOf(name string) Periodic {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.periodic[name]
}

// effective merges the registered defaults with the overrides of the table, sorted by name. The
// overrides are central (PL47c): they lie in the control database and hold for every database.
func (s *Service) effective(ctx context.Context) ([]Schedule, error) {
	rows, err := queries.New(s.db.Control()).ListJobSchedules(ctx)
	if err != nil {
		return nil, fmt.Errorf("jobs: schedules: %w", err)
	}
	overrides := make(map[string]queries.JobSchedule, len(rows))
	for _, r := range rows {
		overrides[r.Name] = r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Schedule, 0, len(s.periodic))
	for _, p := range s.periodic {
		e := Schedule{Name: p.Name, Label: p.Label, Queue: p.Queue, DefaultSchedule: p.Schedule, Schedule: p.Schedule, Enabled: true}
		if o, ok := overrides[p.Name]; ok {
			e.Schedule, e.Enabled = o.Schedule, o.Enabled
			at := o.UpdatedAt.Time
			e.UpdatedAt = &at
			if o.UpdatedBy.Valid {
				by := o.UpdatedBy.UUID
				e.UpdatedBy = &by
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Schedules lists the periodic jobs with their next and last run; the last run is the one in
// the database the request works in.
func (s *Service) Schedules(ctx context.Context) ([]Schedule, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return nil, err
	}
	out, err := s.effective(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for i := range out {
		if out[i].Enabled {
			if schedule, err := parseCron(out[i].Schedule); err == nil {
				next := schedule.Next(now)
				out[i].NextRun = &next
			}
		}
		res, err := client.JobList(ctx, river.NewJobListParams().Kinds(out[i].Name).OrderBy(river.JobListOrderByID, river.SortOrderDesc).First(1))
		if err != nil {
			return nil, fmt.Errorf("jobs: last run of %s: %w", out[i].Name, err)
		}
		if len(res.Jobs) == 1 {
			last := toJob(res.Jobs[0])
			out[i].LastRun = &last
		}
	}
	return out, nil
}

// ScheduleByName returns one schedule.
func (s *Service) ScheduleByName(ctx context.Context, name string) (Schedule, error) {
	all, err := s.Schedules(ctx)
	if err != nil {
		return Schedule{}, err
	}
	for _, e := range all {
		if e.Name == name {
			return e, nil
		}
	}
	return Schedule{}, fmt.Errorf("%w: schedule %s", ErrNotFound, name)
}

// SetSchedule stores an override and tells every instance; it takes effect without restart.
func (s *Service) SetSchedule(ctx context.Context, name string, patch SchedulePatch, actor *uuid.UUID) (Schedule, error) {
	current, err := s.ScheduleByName(ctx, name)
	if err != nil {
		return Schedule{}, err
	}
	schedule, enabled := current.Schedule, current.Enabled
	if patch.Schedule != nil {
		if _, err := parseCron(*patch.Schedule); err != nil {
			return Schedule{}, err
		}
		schedule = *patch.Schedule
	}
	if patch.Enabled != nil {
		enabled = *patch.Enabled
	}
	var by uuid.NullUUID
	if actor != nil {
		by = uuid.NullUUID{UUID: *actor, Valid: true}
	}
	if _, err := queries.New(s.db.Control()).UpsertJobSchedule(ctx, queries.UpsertJobScheduleParams{
		Name: name, Schedule: schedule, Enabled: enabled, UpdatedBy: by,
	}); err != nil {
		return Schedule{}, fmt.Errorf("jobs: store schedule: %w", err)
	}
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if started {
		if err := s.reload(ctx); err != nil {
			return Schedule{}, err
		}
	}
	if s.o.Cache != nil {
		if err := s.o.Cache.Publish(ctx, reloadChannel, name); err != nil {
			s.o.Logger.Warn("jobs: schedule change not published", "name", name, "error", err.Error())
		}
	}
	return s.ScheduleByName(ctx, name)
}

// RunNow enqueues a periodic job right away, regardless of its schedule, in the database the
// request works in; a central job in the control database.
func (s *Service) RunNow(ctx context.Context, name string) (Job, error) {
	p, ok := s.periodicLookup(name)
	if !ok {
		return Job{}, fmt.Errorf("%w: schedule %s", ErrNotFound, name)
	}
	if p.Central {
		ctx = s.db.Databases()[0].Bind(ctx)
	}
	return s.Enqueue(ctx, p.Args(), &river.InsertOpts{Queue: p.Queue})
}

func (s *Service) periodicLookup(name string) (Periodic, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.periodic[name]
	return p, ok
}

// Enqueue inserts a job outside a transaction (opts may be nil: queue default).
func (s *Service) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return Job{}, err
	}
	res, err := client.Insert(ctx, args, withQueue(opts))
	if err != nil {
		return Job{}, fmt.Errorf("jobs: enqueue %s: %w", args.Kind(), err)
	}
	return toJob(res.Job), nil
}

// EnqueueTx inserts a job in the transaction of the change that causes it (E19).
func (s *Service) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return Job{}, err
	}
	res, err := client.InsertTx(ctx, tx, args, withQueue(opts))
	if err != nil {
		return Job{}, fmt.Errorf("jobs: enqueue %s: %w", args.Kind(), err)
	}
	return toJob(res.Job), nil
}

// EnqueueManyTx inserts many jobs in one statement of the transaction: the dispatcher queues
// the jobs of a whole batch of events this way (PL41).
func (s *Service) EnqueueManyTx(ctx context.Context, tx pgx.Tx, params []river.InsertManyParams) error {
	if len(params) == 0 {
		return nil
	}
	client, err := s.ready(ctx)
	if err != nil {
		return err
	}
	for i := range params {
		params[i].InsertOpts = withQueue(params[i].InsertOpts)
	}
	if _, err := client.InsertManyFastTx(ctx, tx, params); err != nil {
		return fmt.Errorf("jobs: enqueue %d jobs: %w", len(params), err)
	}
	return nil
}

func withQueue(opts *river.InsertOpts) *river.InsertOpts {
	if opts == nil {
		opts = &river.InsertOpts{}
	}
	if opts.Queue == "" {
		opts.Queue = QueueDefault
	}
	return opts
}

var states = map[string]rivertype.JobState{
	"available": rivertype.JobStateAvailable, "cancelled": rivertype.JobStateCancelled,
	"completed": rivertype.JobStateCompleted, "discarded": rivertype.JobStateDiscarded,
	"pending": rivertype.JobStatePending, "retryable": rivertype.JobStateRetryable,
	"running": rivertype.JobStateRunning, "scheduled": rivertype.JobStateScheduled,
}

// TenantOf is the tenant a job started by the request carries in its args as "tenant"
// (PL45): the tenant of the session, empty without tenants.
func TenantOf(ctx context.Context) string {
	if a := authz.From(ctx); a.Bounded() {
		return a.CurrentTenant()
	}
	return ""
}

// Visible reports whether the request sees a job (PL45): without tenants every one; in a
// tenant the jobs that carry it, and those of no tenant (the system's) with installation.
func Visible(ctx context.Context, args []byte, installation bool) bool {
	a := authz.From(ctx)
	if !a.Bounded() {
		return true
	}
	var probe struct {
		Tenant string `json:"tenant"`
	}
	_ = json.Unmarshal(args, &probe)
	if probe.Tenant == "" {
		return installation
	}
	return probe.Tenant == a.CurrentTenant()
}

// List returns the newest jobs first, at most Filter.Limit (default 50, at most 500).
func (s *Service) List(ctx context.Context, f Filter) ([]Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return nil, err
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	params := river.NewJobListParams().OrderBy(river.JobListOrderByID, river.SortOrderDesc).First(limit)
	if f.Queue != "" {
		params = params.Queues(f.Queue)
	}
	if f.Kind != "" {
		params = params.Kinds(f.Kind)
	}
	if f.State != "" {
		st, ok := states[f.State]
		if !ok {
			return nil, fmt.Errorf("%w: unknown state %q", ErrInvalid, f.State)
		}
		params = params.States(st)
	}
	if a := authz.From(ctx); a.Bounded() {
		// In a tenant its jobs, and those of the system with installation (PL45; Visible).
		params = params.Where("(args->>'tenant' = @tenant OR (@installation::boolean AND args->>'tenant' IS NULL))",
			river.NamedArgs{"tenant": a.CurrentTenant(), "installation": f.Installation})
	}
	res, err := client.JobList(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("jobs: list: %w", err)
	}
	out := make([]Job, 0, len(res.Jobs))
	for _, j := range res.Jobs {
		out = append(out, toJob(j))
	}
	return out, nil
}

// Get returns one job.
func (s *Service) Get(ctx context.Context, id int64) (Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return Job{}, err
	}
	row, err := client.JobGet(ctx, id)
	return s.rowOrNotFound(id, row, err)
}

// Cancel stops a job: a waiting one is cancelled at once, a running one when it checks
// its context, a finished one stays as it is.
func (s *Service) Cancel(ctx context.Context, id int64) (Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return Job{}, err
	}
	row, err := client.JobCancel(ctx, id)
	return s.rowOrNotFound(id, row, err)
}

// Retry makes a job available again right away, also one that used up its attempts.
func (s *Service) Retry(ctx context.Context, id int64) (Job, error) {
	client, err := s.ready(ctx)
	if err != nil {
		return Job{}, err
	}
	row, err := client.JobRetry(ctx, id)
	return s.rowOrNotFound(id, row, err)
}

func (s *Service) rowOrNotFound(id int64, row *rivertype.JobRow, err error) (Job, error) {
	if errors.Is(err, rivertype.ErrNotFound) {
		return Job{}, fmt.Errorf("%w: job %d", ErrNotFound, id)
	}
	if err != nil {
		return Job{}, fmt.Errorf("jobs: job %d: %w", id, err)
	}
	return toJob(row), nil
}

func toJob(r *rivertype.JobRow) Job {
	j := Job{
		ID: r.ID, Kind: r.Kind, Queue: r.Queue, State: string(r.State), Priority: r.Priority,
		Attempt: r.Attempt, MaxAttempts: r.MaxAttempts, CreatedAt: r.CreatedAt, ScheduledAt: r.ScheduledAt,
		AttemptedAt: r.AttemptedAt, FinalizedAt: r.FinalizedAt, Args: json.RawMessage(r.EncodedArgs),
	}
	if len(j.Args) == 0 {
		j.Args = json.RawMessage("{}")
	}
	for _, e := range r.Errors {
		j.Errors = append(j.Errors, JobError{At: e.At, Attempt: e.Attempt, Error: e.Error})
	}
	if out := r.Output(); len(out) > 0 {
		j.Output = json.RawMessage(out)
	}
	return j
}
