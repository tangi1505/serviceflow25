// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package export writes the list of a type as CSV, XLSX or ODS (UI5; Inventar G230, G231;
// Hauptplan E125). It reads page by page through the JSON query (E40) with the rights of the
// caller, so the file holds exactly the rows the list shows and the columns it names, with the
// values as the list shows them: labels in the language of the caller, references by the
// title of what they point to, numbers and dates as such. Up to MaxDirect rows stream to the
// caller; more run as a job whose file waits as a temporary file of the caller (PL18).
package export

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/richtext"
)

// MaxDirect is the most rows an export streams in the request by default; more go to a job.
const MaxDirect = 100000

// pageSize is how many rows one query reads.
const pageSize = 500

// KindFile names the job of a large export.
const KindFile = "export.file"

// Request is what to export: the format and the view of the list.
type Request struct {
	Format   string          `json:"format"`
	Where    json.RawMessage `json:"where,omitempty"`
	Columns  []string        `json:"columns,omitempty"`
	Sort     string          `json:"sort,omitempty"`
	TimeZone string          `json:"time_zone,omitempty"`
	// Lang is the language of labels and of yes/no; the handler takes it from the request.
	Lang string `json:"lang,omitempty"`
}

// FileArgs is the job of a large export.
type FileArgs struct {
	Type    string  `json:"type"`
	Request Request `json:"request"`
	// Account asked for the export: its rights apply, it may read the job and the file.
	Account uuid.UUID `json:"account"`
	// Tenant is the tenant the account worked in; the job works there too (PL45).
	Tenant string `json:"tenant,omitempty"`
}

// Kind names the job.
func (FileArgs) Kind() string { return KindFile }

// Result is the output of the job.
type Result struct {
	Rows int       `json:"rows"`
	File uuid.UUID `json:"file"`
	Name string    `json:"name"`
}

// Service exports lists.
type Service struct {
	rec    *records.Service
	files  *files.Service
	jobs   *jobs.Service
	load   func(ctx context.Context, account uuid.UUID) (*authz.Access, error)
	logger *slog.Logger
	// Queue is the River queue of the jobs; tests use their own.
	Queue string
	// MaxDirect is the most rows streamed in the request; tests lower it.
	MaxDirect int64
}

// New returns the service.
func New(rec *records.Service, fs *files.Service, js *jobs.Service, load func(context.Context, uuid.UUID) (*authz.Access, error), logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{rec: rec, files: fs, jobs: js, load: load, logger: logger, Queue: jobs.QueueBulk, MaxDirect: MaxDirect}
}

// Register adds the worker of the job.
func (s *Service) Register() { jobs.AddWorker(s.jobs, &fileWorker{s: s}) }

// column is one column of the export: a field or a system column, and for a reference the
// path to the title of what it points to.
type column struct {
	name  string
	label string
	field *meta.Field
	title string // "parent.title" for a reference, read along
}

var systemLabels = map[string]map[string]string{
	"id":         {"de": "ID", "en": "ID"},
	"version":    {"de": "Version", "en": "Version"},
	"state":      {"de": "Status", "en": "State"},
	"org_path":   {"de": "Organisationseinheit", "en": "Organisational unit"},
	"created_at": {"de": "Angelegt", "en": "Created"},
	"updated_at": {"de": "Geändert", "en": "Updated"},
	"sla_due_at": {"de": "Frist", "en": "Deadline"},
}

func label(l meta.Label, lang, fallback string) string {
	if v := l[lang]; v != "" {
		return v
	}
	if v := l["de"]; v != "" {
		return v
	}
	return fallback
}

// plan checks the request against the type and works out the columns and the query.
func (s *Service) plan(ctx context.Context, t meta.Type, req Request) ([]column, *time.Location, error) {
	var ps []records.Problem
	if !slices.Contains(Formats, req.Format) {
		ps = append(ps, records.Problem{Field: "format", Code: "choice", Message: "must be one of " + strings.Join(Formats, ", ")})
	}
	loc := time.UTC
	if req.TimeZone != "" {
		l, err := time.LoadLocation(req.TimeZone)
		if err != nil {
			ps = append(ps, records.Problem{Field: "time_zone", Code: "unknown", Message: fmt.Sprintf("%q is no time zone", req.TimeZone)})
		} else {
			loc = l
		}
	}
	names := req.Columns
	if len(names) == 0 {
		for _, f := range t.Fields {
			names = append(names, f.Name)
		}
	}
	var cols []column
	for _, n := range names {
		if l, ok := systemLabels[n]; ok {
			c := column{name: n, label: label(l, req.Lang, n)}
			if n == "state" && len(t.States) == 0 {
				continue
			}
			cols = append(cols, c)
			continue
		}
		f, ok := t.Field(n)
		if !ok {
			ps = append(ps, records.Problem{Field: "columns", Code: "unknown", Message: fmt.Sprintf("%s has no column %q", t.QualifiedName(), n)})
			continue
		}
		c := column{name: n, label: label(f.Label, req.Lang, n), field: &f}
		if f.Type == meta.FieldReference {
			if target, err := s.rec.Type(ctx, f.To); err == nil && target.TitleField != "" {
				c.title = n + "." + target.TitleField
			}
		}
		cols = append(cols, c)
	}
	if len(ps) > 0 {
		return nil, nil, &records.ValidationError{Problems: ps}
	}
	return cols, loc, nil
}

func (s *Service) query(cols []column, req Request) records.Query {
	q := records.Query{Where: req.Where, Limit: pageSize}
	for _, c := range cols {
		q.Fields = append(q.Fields, c.name)
		if c.title != "" {
			q.Fields = append(q.Fields, c.title)
		}
	}
	if req.Sort != "" {
		q.Sort = []string{req.Sort}
	}
	return q
}

// Count is how many rows the export would have.
func (s *Service) Count(ctx context.Context, typeName string, req Request) (int64, error) {
	t, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return 0, err
	}
	cols, _, err := s.plan(ctx, t, req)
	if err != nil {
		return 0, err
	}
	q := s.query(cols, req)
	q.Limit, q.Count = 1, true
	page, _, err := s.rec.Query(ctx, typeName, q)
	if err != nil {
		return 0, err
	}
	if page.Total == nil {
		return 0, errors.New("export: the query did not count")
	}
	return *page.Total, nil
}

// Write writes the export to w and returns the number of rows.
func (s *Service) Write(ctx context.Context, w io.Writer, typeName string, req Request) (int, error) {
	t, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return 0, err
	}
	cols, loc, err := s.plan(ctx, t, req)
	if err != nil {
		return 0, err
	}
	words := [2]string{"ja", "nein"}
	if req.Lang == "en" {
		words = [2]string{"yes", "no"}
	}
	sh, err := newSheet(req.Format, w, words)
	if err != nil {
		return 0, err
	}
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.label
	}
	if err := sh.Header(header); err != nil {
		return 0, err
	}
	ctx = records.WithLang(ctx, records.Lang{Read: req.Lang})
	q := s.query(cols, req)
	rows := 0
	for {
		page, _, err := s.rec.Query(ctx, typeName, q)
		if err != nil {
			return rows, err
		}
		for _, rec := range page.Items {
			cells := make([]Cell, len(cols))
			for i, c := range cols {
				cells[i] = cellOf(t, c, rec, req.Lang, loc)
			}
			if err := sh.Row(cells); err != nil {
				return rows, err
			}
			rows++
		}
		if page.Next == "" {
			break
		}
		q.After = page.Next
	}
	return rows, sh.Close()
}

// cellOf turns one value into a cell as the list shows it.
func cellOf(t meta.Type, c column, rec records.Record, lang string, loc *time.Location) Cell {
	v := rec[c.name]
	if v == nil {
		return Cell{}
	}
	if c.field == nil {
		switch c.name {
		case "state":
			for _, st := range t.States {
				if st.Name == fmt.Sprint(v) {
					return Cell{Kind: Text, S: label(st.Label, lang, st.Name)}
				}
			}
		case "version":
			if n, ok := number(v); ok {
				return Cell{Kind: Number, F: n}
			}
		case "created_at", "updated_at", "sla_due_at":
			if tm, ok := moment(v); ok {
				return Cell{Kind: DateTime, T: tm.In(loc)}
			}
		}
		return Cell{Kind: Text, S: fmt.Sprint(v)}
	}
	f := c.field
	switch f.Type {
	case meta.FieldNumber:
		if n, ok := number(v); ok {
			return Cell{Kind: Number, F: n}
		}
	case meta.FieldBoolean:
		if b, ok := v.(bool); ok {
			return Cell{Kind: Bool, B: b}
		}
	case meta.FieldDate:
		if tm, ok := day(v); ok {
			return Cell{Kind: Date, T: tm}
		}
	case meta.FieldDateTime:
		if tm, ok := moment(v); ok {
			return Cell{Kind: DateTime, T: tm.In(loc)}
		}
	case meta.FieldChoice:
		for _, o := range f.Options {
			if o.Value == fmt.Sprint(v) {
				return Cell{Kind: Text, S: label(o.Label, lang, o.Value)}
			}
		}
	case meta.FieldReference:
		if title, ok := rec[c.title]; ok && title != nil && fmt.Sprint(title) != "" {
			return Cell{Kind: Text, S: fmt.Sprint(title)}
		}
	case meta.FieldSequence:
		// The running number as it is shown, with its prefix (PL49).
		return Cell{Kind: Text, S: f.Prefix + fmt.Sprint(v)}
	}
	if f.Format == meta.FormatRich {
		// Formatted text (UI7) as its plain text.
		if s, ok := v.(string); ok {
			return Cell{Kind: Text, S: richtext.Text(s)}
		}
	}
	if m, ok := v.(map[string]any); ok {
		// A translatable field read with every language (PL21): the one asked for.
		if s, ok := m[lang].(string); ok {
			return Cell{Kind: Text, S: s}
		}
		raw, _ := json.Marshal(m)
		return Cell{Kind: Text, S: string(raw)}
	}
	return Cell{Kind: Text, S: fmt.Sprint(v)}
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func moment(v any) (time.Time, bool) {
	switch m := v.(type) {
	case time.Time:
		return m, true
	case string:
		tm, err := time.Parse(time.RFC3339Nano, m)
		return tm, err == nil
	}
	return time.Time{}, false
}

func day(v any) (time.Time, bool) {
	switch d := v.(type) {
	case time.Time:
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), true
	case string:
		tm, err := time.Parse("2006-01-02", d)
		return tm, err == nil
	}
	return time.Time{}, false
}

// FileName is the name of the file of an export: type, date and extension.
func FileName(typeName, format string, now time.Time) string {
	return fmt.Sprintf("%s-%s.%s", strings.ReplaceAll(typeName, ".", "-"), now.Format("2006-01-02-1504"), format)
}

// CanQueue says whether large exports can run: they need the file store.
func (s *Service) CanQueue() bool { return s.files != nil }

// Start queues the job of a large export for the account.
func (s *Service) Start(ctx context.Context, typeName string, req Request, account uuid.UUID) (jobs.Job, error) {
	return s.jobs.Enqueue(ctx, FileArgs{Type: typeName, Request: req, Account: account, Tenant: jobs.TenantOf(ctx)},
		&river.InsertOpts{Queue: s.Queue, MaxAttempts: 3})
}

type fileWorker struct {
	river.WorkerDefaults[FileArgs]
	s *Service
}

// Work writes the export into a temporary file of the account; the output names it.
func (w *fileWorker) Work(ctx context.Context, job *river.Job[FileArgs]) error {
	access, err := w.s.load(ctx, job.Args.Account)
	if err != nil {
		return fmt.Errorf("export: rights of %s: %w", job.Args.Account, err)
	}
	ctx = authz.WithAccess(ctx, access.Enter(job.Args.Tenant))
	name := FileName(job.Args.Type, job.Args.Request.Format, time.Now())
	pr, pw := io.Pipe()
	done := make(chan int, 1)
	go func() {
		n, err := w.s.Write(ctx, pw, job.Args.Type, job.Args.Request)
		_ = pw.CloseWithError(err)
		done <- n
	}()
	f, err := w.s.files.Upload(ctx, name, pr, records.Actor(&job.Args.Account))
	// An upload that stopped early stops the writer too.
	_ = pr.CloseWithError(err)
	rows := <-done
	if err != nil {
		return fmt.Errorf("export: keep the file: %w", err)
	}
	w.s.logger.Info("export done", "type", job.Args.Type, "format", job.Args.Request.Format, "rows", rows, "file", f.ID)
	return river.RecordOutput(ctx, Result{Rows: rows, File: f.ID, Name: name})
}
