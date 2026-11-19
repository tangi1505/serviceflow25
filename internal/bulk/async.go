// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package bulk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/records"
)

// --- the asynchronous bulk (PL32; architektur.md §6.4) ---
//
// More than a synchronous call carries comes as NDJSON: one row per line, each with
// external_key. Start keeps the stream in the file store and queues a job; the job reads it
// in batches of BatchSize rows, writes every batch set-based (records.SyncBatch) with the
// rights of the account that started it, and reports its progress and result as the job's
// output: the counts and the first MaxFailures failed rows. A key that comes twice in one
// stream is an error from its second line on. Only an error of the database fails the job;
// River tries it again, and the rows written before are then unchanged.

// SettingMaxSize bounds a stream (setting of the root node, bytes).
const SettingMaxSize = "bulk.max_size"

// DefaultMaxSize is the bound without the setting: 1 GiB.
var DefaultMaxSize int64 = 1 << 30

// Sizes of the job.
const (
	BatchSize   = 5000
	MaxFailures = 1000
	maxLine     = 1 << 20 // a row of more than 1 MiB is an error of its line
	maxAttempts = 5
)

// KindApply names the job.
const KindApply = "bulk.apply"

// ErrTooLarge is a stream beyond bulk.max_size (413).
var ErrTooLarge = errors.New("bulk: stream too large")

// ApplyArgs is the job of one stream.
type ApplyArgs struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Stream string `json:"stream"` // key in the file store
	// Account started the run: its rights apply, and it may read the job (PL32).
	Account uuid.UUID `json:"account"`
	// Tenant is the tenant the account worked in; the run writes there (PL45).
	Tenant string `json:"tenant,omitempty"`
}

// Kind names the job.
func (ApplyArgs) Kind() string { return KindApply }

// BatchWriter writes a batch; records.Service is the one.
type BatchWriter interface {
	SyncBatch(ctx context.Context, typeName, source string, rows []records.BulkRow, actor records.Actor) ([]records.BulkResult, error)
}

// Report is the output of the job: the progress while it runs, the result when it is done.
type Report struct {
	Done      bool      `json:"done"`
	Lines     int       `json:"lines"`
	Created   int       `json:"created"`
	Updated   int       `json:"updated"`
	Unchanged int       `json:"unchanged"`
	Errors    int       `json:"errors"`
	Failures  []Failure `json:"failures,omitempty"`
}

// Failure is a row that was not written.
type Failure struct {
	Line        int               `json:"line"`
	ExternalKey string            `json:"external_key,omitempty"`
	Code        string            `json:"code"`
	Message     string            `json:"message,omitempty"`
	Problems    []records.Problem `json:"problems,omitempty"`
}

// Async starts and works the asynchronous runs.
type Async struct {
	store    files.Store
	jobs     *jobs.Service
	w        BatchWriter
	load     func(ctx context.Context, account uuid.UUID) (*authz.Access, error)
	settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)
	logger   *slog.Logger
	// Queue is the River queue of the jobs; tests use their own, so that a running instance
	// does not take them.
	Queue string
}

// NewAsync returns the service; settings may be nil (then the default bound applies).
func NewAsync(store files.Store, js *jobs.Service, w BatchWriter, load func(context.Context, uuid.UUID) (*authz.Access, error),
	settings func(context.Context, string, string) (json.RawMessage, string, error), logger *slog.Logger) *Async {
	if logger == nil {
		logger = slog.Default()
	}
	return &Async{store: store, jobs: js, w: w, load: load, settings: settings, logger: logger, Queue: jobs.QueueBulk}
}

// Register adds the worker of the job.
func (a *Async) Register() { jobs.AddWorker(a.jobs, &applyWorker{a: a}) }

// Start keeps the stream and queues its job.
func (a *Async) Start(ctx context.Context, typeName, source string, body io.Reader, actor records.Actor) (jobs.Job, error) {
	if len(source) == 0 || len(source) > 100 {
		return jobs.Job{}, &InvalidError{"source", "must be 1 to 100 characters"}
	}
	if actor == nil {
		return jobs.Job{}, &InvalidError{"source", "an asynchronous run needs a signed-in caller"}
	}
	limit := a.maxSize(ctx)
	id, err := uuid.NewV7()
	if err != nil {
		return jobs.Job{}, err
	}
	key := "bulk/" + id.String() + ".ndjson"
	n, err := a.store.Put(ctx, key, io.LimitReader(body, limit+1))
	if err != nil {
		_ = a.store.Delete(context.WithoutCancel(ctx), key)
		return jobs.Job{}, fmt.Errorf("bulk: keep the stream: %w", err)
	}
	switch {
	case n > limit:
		_ = a.store.Delete(ctx, key)
		return jobs.Job{}, fmt.Errorf("%w: more than %d bytes (setting %s)", ErrTooLarge, limit, SettingMaxSize)
	case n == 0:
		_ = a.store.Delete(ctx, key)
		return jobs.Job{}, &InvalidError{"rows", "the stream is empty"}
	}
	job, err := a.jobs.Enqueue(ctx, ApplyArgs{Type: typeName, Source: source, Stream: key, Account: *actor, Tenant: jobs.TenantOf(ctx)},
		&river.InsertOpts{Queue: a.Queue, MaxAttempts: maxAttempts})
	if err != nil {
		_ = a.store.Delete(context.WithoutCancel(ctx), key)
		return jobs.Job{}, err
	}
	return job, nil
}

func (a *Async) maxSize(ctx context.Context) int64 {
	if a.settings == nil {
		return DefaultMaxSize
	}
	raw, _, err := a.settings(ctx, "root", SettingMaxSize)
	if err != nil || raw == nil {
		return DefaultMaxSize
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil || n <= 0 {
		return DefaultMaxSize
	}
	return n
}

type applyWorker struct {
	river.WorkerDefaults[ApplyArgs]
	a *Async
}

// Timeout lets a run take up to an hour: a stream of up to bulk.max_size holds millions of
// rows, River's default of one minute cut a run of 100,000 on a shared ki-lab at 95,000 (PL74
// CI, 2026-11-19).
func (w *applyWorker) Timeout(*river.Job[ApplyArgs]) time.Duration { return time.Hour }

func (w *applyWorker) Work(ctx context.Context, job *river.Job[ApplyArgs]) error {
	report, err := w.a.apply(ctx, job.Args, func(r Report) {
		// The progress while the job runs; failing to show it changes nothing.
		if c := river.ClientFromContext[pgx.Tx](ctx); c != nil {
			_, _ = c.JobUpdate(ctx, job.ID, &river.JobUpdateParams{Output: r})
		}
	})
	_ = river.RecordOutput(ctx, report)
	switch {
	case errors.Is(err, files.ErrNoObject):
		return river.JobCancel(err)
	case err != nil && job.Attempt < job.MaxAttempts:
		return err
	}
	// Done, or failed for the last time: the stream is no longer needed.
	if derr := w.a.store.Delete(context.WithoutCancel(ctx), job.Args.Stream); derr != nil {
		w.a.logger.Warn("bulk: remove the stream", "stream", job.Args.Stream, "error", derr.Error())
	}
	if err == nil {
		w.a.logger.Info("bulk run done", "type", job.Args.Type, "source", job.Args.Source, "lines", report.Lines,
			"created", report.Created, "updated", report.Updated, "unchanged", report.Unchanged, "errors", report.Errors)
	}
	return err
}

// apply reads the stream and writes it batch by batch.
func (a *Async) apply(ctx context.Context, args ApplyArgs, progress func(Report)) (Report, error) {
	var rep Report
	access, err := a.load(ctx, args.Account)
	if err != nil {
		return rep, fmt.Errorf("bulk: rights of %s: %w", args.Account, err)
	}
	ctx = authz.WithAccess(ctx, access.Enter(args.Tenant))
	actor := records.Actor(&args.Account)
	f, err := a.store.Open(ctx, args.Stream)
	if err != nil {
		return rep, err
	}
	defer f.Close()

	fail := func(line int, key string, err error) {
		rep.Errors++
		if len(rep.Failures) >= MaxFailures {
			return
		}
		res := Result{}
		if !rowError(&res, err) {
			res.Code, res.Message = "invalid", err.Error()
		}
		rep.Failures = append(rep.Failures, Failure{Line: line, ExternalKey: key, Code: res.Code, Message: res.Message, Problems: res.Problems})
	}
	seen := map[string]int{}
	var (
		batch []records.BulkRow
		lines []int
	)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		results, err := a.w.SyncBatch(ctx, args.Type, args.Source, batch, actor)
		if err != nil {
			return err
		}
		for i, r := range results {
			switch {
			case r.Err != nil:
				fail(lines[i], batch[i].Key, r.Err)
			case r.Status == records.SyncCreated:
				rep.Created++
			case r.Status == records.SyncUpdated:
				rep.Updated++
			default:
				rep.Unchanged++
			}
		}
		batch, lines = batch[:0], lines[:0]
		progress(rep)
		return nil
	}

	handle := func(line int, raw []byte) error {
		row, ok := decodeRow(raw)
		if !ok {
			fail(line, "", &records.ValidationError{Problems: []records.Problem{{Field: "line " + strconv.Itoa(line), Code: "json", Message: "must be one JSON object"}}})
			return nil
		}
		key, fields, err := split(row)
		if err != nil {
			fail(line, key, err)
			return nil
		}
		if first, dup := seen[key]; dup {
			fail(line, key, fmt.Errorf("%w: key %q came already in line %d of this stream", records.ErrConflict, key, first))
			return nil
		}
		seen[key] = line
		batch = append(batch, records.BulkRow{Key: key, Fields: fields, Hash: Hash(fields)})
		lines = append(lines, line)
		if len(batch) == BatchSize {
			return flush()
		}
		return nil
	}

	rd := bufio.NewReaderSize(f, 64<<10)
	for line := 1; ; line++ {
		raw, err := readLine(rd)
		if errors.Is(err, errLongLine) {
			rep.Lines++
			fail(line, "", &InvalidError{"rows", fmt.Sprintf("line %d is longer than 1 MiB", line)})
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return rep, err
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			rep.Lines++
			if herr := handle(line, raw); herr != nil {
				return rep, herr
			}
		}
		if err != nil {
			break
		}
	}
	if err := flush(); err != nil {
		return rep, err
	}
	rep.Done = true
	return rep, nil
}

// decodeRow reads one line as a JSON object, numbers as json.Number like the API.
func decodeRow(raw []byte) (map[string]any, bool) {
	var row map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&row) != nil || row == nil {
		return nil, false
	}
	return row, true
}

var errLongLine = errors.New("bulk: line too long")

// readLine reads one line without its end; at the end of the stream the error is io.EOF. A
// line beyond maxLine is read to its end and reported as errLongLine.
func readLine(rd *bufio.Reader) ([]byte, error) {
	var buf []byte
	long := false
	for {
		part, err := rd.ReadSlice('\n')
		if !long && len(buf)+len(part) > maxLine {
			long, buf = true, nil
		}
		if !long {
			buf = append(buf, part...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if long {
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			return nil, errLongLine
		}
		return bytes.TrimRight(buf, "\r\n"), err
	}
}
