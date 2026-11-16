// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Command itsm is the single binary of the itsm platform (Hauptplan E35).
// It runs as api, worker or both, and carries the operational subcommands.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/auth"
	"group24.de/itsm/internal/authn"
	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/bulk"
	"group24.de/itsm/internal/cache"
	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/customer"
	"group24.de/itsm/internal/dashboard"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/export"
	"group24.de/itsm/internal/files"
	"group24.de/itsm/internal/forms"
	"group24.de/itsm/internal/httpapi"
	"group24.de/itsm/internal/jobs"
	"group24.de/itsm/internal/notify"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/portal"
	"group24.de/itsm/internal/prefs"
	"group24.de/itsm/internal/presence"
	"group24.de/itsm/internal/query"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/recurrence"
	"group24.de/itsm/internal/relations"
	"group24.de/itsm/internal/rules/engine"
	"group24.de/itsm/internal/savedsearch"
	"group24.de/itsm/internal/search"
	"group24.de/itsm/internal/seclog"
	"group24.de/itsm/internal/settings"
	"group24.de/itsm/internal/sla"
	"group24.de/itsm/internal/snippets"
	"group24.de/itsm/internal/webhooks"
)

const usage = `usage: itsm <command> [flags]

commands:
  serve        run the server; --role=api|worker|all (default from ITSM_ROLE, else all)
  migrate      apply database migrations of the core tables
  package      package install [--force] <dir> | package list | package export-customer <dir>
  user         user create|passwd|disable|enable|link|grant|tenant|list <username>; the password comes on stdin
  tenant       tenant create <name> [--label …] [--adopt [--admin <user>]…] | tenant list
  ldap         ldap list | ldap sync [name]; the bind password comes from ITSM_LDAP_PASSWORD_<NAME>
  client       client create <id> [--name …] [--ip-range …] | client list | client rotate <id>
  calendar     calendar holidays <calendar-id> <state> <year>: German public holidays into a calendar
  healthcheck  exit 0 if the local server answers /healthz (for container health checks)
  version      print the version
`

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, platform.Version)
		return 0
	case "serve":
		return exitCode(stderr, serve(args[1:], getenv, stdout))
	case "migrate":
		return exitCode(stderr, migrate(getenv, stdout))
	case "package":
		return exitCode(stderr, packageCmd(args[1:], getenv, stdout, stderr))
	case "user":
		return exitCode(stderr, userCmd(args[1:], getenv, os.Stdin, stdout))
	case "tenant":
		return exitCode(stderr, tenantCmd(args[1:], getenv, stdout))
	case "ldap":
		return exitCode(stderr, ldapCmd(args[1:], getenv, stdout, stderr))
	case "client":
		return exitCode(stderr, clientCmd(args[1:], getenv, stdout, stderr))
	case "calendar":
		return exitCode(stderr, calendarCmd(args[1:], getenv, stdout, stderr))
	case "healthcheck":
		return exitCode(stderr, healthcheck(getenv))
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func exitCode(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(stderr, "error:", err)
	return 1
}

func serve(args []string, getenv func(string) string, stdout io.Writer) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	role := fs.String("role", "", "api, worker or all")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	if addr := getenv("ITSM_PPROF_ADDR"); addr != "" {
		if err := startPprof(addr, slog.Default()); err != nil {
			return err
		}
	}
	if *role != "" {
		r, err := platform.ParseRole(*role)
		if err != nil {
			return err
		}
		cfg.Role = r
	}
	logger := platform.NewLogger(stdout, cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// One database, or with ITSM_TENANT_MODE=database one per tenant (PL47d, E141, E144).
	rt, closeDBs, err := openDatabases(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDBs()
	logger.Info("databases opened", "mode", cfg.TenantMode, "databases", len(rt.Databases()))

	kv, err := cache.Open(ctx, cfg.ValkeyAddr)
	if err != nil {
		return err
	}
	defer kv.Close()

	az := authz.New(rt, kv, logger)
	rec := records.New(rt)
	rec.AfterWrite = func(ctx context.Context, typeName string) {
		if typeName == "core.group_member" {
			az.Invalidate(ctx)
		}
	}
	// Jobs (PL23): the worker role works the queues, the api role only inserts and reads.
	var workers map[string]int
	if cfg.Role.RunsWorker() {
		workers = cfg.Workers
	}
	js := jobs.New(rt, jobs.Options{Logger: logger, Workers: workers, Cache: kv})
	if err := jobs.RegisterEventRetention(js, cfg.EventRetention); err != nil {
		return err
	}
	ev := events.New(rt, rec, kv, logger)
	authSvc := auth.New(rt, kv, logger)
	orgSvc := org.New(rt)
	// Security settings of local accounts (PL28): rules, sessions, lockout, local sign-in.
	authSvc.Settings = orgSvc.Setting
	authSvc.AllowLocal = func(ctx context.Context, account uuid.UUID) (bool, error) {
		a, err := az.Load(ctx, account)
		if err != nil {
			return false, err
		}
		return a.Allowed("core.role", "manage_rights"), nil
	}
	// The costs read their currency and hourly rate from the settings of the tree (PL20).
	rec.Settings = orgSvc.Setting
	// The settings the packages declare (SD20): checked when set, read with their default by
	// formulas (setting("…")) and SLAs (target_setting).
	settingsSvc := settings.New(rt, orgSvc)
	rec.SettingValue = settingsSvc.Value
	an := authn.New(rt, kv, authSvc, az, rec, orgSvc, logger, authn.Options{
		Issuer: cfg.OIDC.Issuer, ClientID: cfg.OIDC.ClientID, ClientSecret: cfg.OIDC.ClientSecret, Name: cfg.OIDC.Name,
		PublicURL: cfg.PublicURL, InternalURL: cfg.OIDC.InternalURL, Scopes: cfg.OIDC.Scopes, GroupsClaim: cfg.OIDC.GroupsClaim,
	})
	if err := authn.RegisterSyncJob(js, an); err != nil {
		return err
	}
	// Calculated fields of all records of a type after a new or changed formula (PL22).
	records.RegisterRecomputeJob(js, rec, logger)
	// Security event log (PL27) with its nightly retention.
	secLog := seclog.New(rt, orgSvc.Setting, logger)
	if err := seclog.RegisterRetentionJob(js, secLog); err != nil {
		return err
	}
	// Webhooks (PL25): the dispatcher queues the deliveries, the worker sends them.
	hooks := webhooks.New(rt, rec, js, orgSvc.Setting, az.Load, logger)
	webhooks.RegisterDeliveryJob(js, hooks)
	ev.Consume(events.ConsumerWebhooks, hooks.HandleBatch)
	// Rules "if, then" (WF4): before a write through the hook of records, after it as jobs
	// the dispatcher queues, on a schedule through a job every minute.
	ruleEngine := engine.New(rt, rec, js, kv, logger)
	ruleEngine.Rights = az.Load
	rec.Before = ruleEngine.Before
	if err := ruleEngine.Register(js); err != nil {
		return err
	}
	ev.Consume(events.ConsumerRules, ruleEngine.HandleBatch)
	// SLA clocks (WF9): they move in the transaction of the write, a job waits for each
	// deadline; working time comes from the calendars (WF8).
	calendars := calendar.New(rec, orgSvc.Setting)
	// Reminders of open approvals (WF7) count working days in the calendar of the record's unit,
	// without one calendar days; a job every minute reminds the stages due.
	rec.WorkingDays = func(ctx context.Context, path string, from time.Time, days int) (time.Time, error) {
		c, _, err := calendars.ForNode(ctx, path)
		if err != nil || c == nil {
			return from.AddDate(0, 0, days), err
		}
		return c.AddDays(from, days)
	}
	// The service time of a resolution (UI14a) counts in the calendar of the node, read with
	// system rights: whoever resolves need not read calendars.
	rec.WorkingTime = func(ctx context.Context, path string, from, to time.Time) (time.Duration, error) {
		c, _, err := calendars.ForNode(authz.WithAccess(ctx, authz.System()), path)
		if err != nil || c == nil {
			return to.Sub(from), err
		}
		return c.Between(from, to)
	}
	if err := records.RegisterRemindJob(js, rec, logger); err != nil {
		return err
	}
	slaSvc := sla.New(rt, rec, calendars, js, kv, logger)
	slaSvc.Register(js)
	slaSvc.Settings, slaSvc.Declared = settingsSvc.Value, settingsSvc.Definition
	// Escalation levels are rules with the trigger clock (WF10).
	slaSvc.Levels = func(ctx context.Context, typeName string, id uuid.UUID) ([]sla.Level, error) {
		list, err := ruleEngine.Levels(ctx, typeName, id)
		out := make([]sla.Level, len(list))
		for i, l := range list {
			out[i] = sla.Level{Rule: l.Rule, Percent: l.Percent, Minutes: l.Minutes}
		}
		return out, err
	}
	// Recurring records (WF14): a job every minute creates those due.
	recurSvc := recurrence.New(rec, logger)
	if err := recurSvc.Register(js); err != nil {
		return err
	}
	// Notifications (WF15, WF19): a consumer of the dispatcher and the jobs that find the persons,
	// keep the entries of the inbox and send the mail; without a mail server the inbox still
	// fills. A job every night removes old entries of the inbox.
	notifySvc := notify.New(rt, rec, js, az.Load, notify.NewMailer(notify.SMTP(cfg.SMTP)), logger)
	notifySvc.PublicURL, notifySvc.Settings = cfg.PublicURL, orgSvc.Setting
	notifySvc.SetKV(kv)
	notifySvc.Register(js)
	ev.Consume(events.ConsumerNotify, notifySvc.HandleBatch)
	if err := notifySvc.RegisterInboxRetention(js); err != nil {
		return err
	}
	// Dashboards (UI13): their widgets are checked like the saved searches.
	dashboards := dashboard.New(rec)
	rec.Clocks, rec.CheckRecord = slaSvc, records.Checks(slaSvc.Check, recurSvc.Check, records.CheckApprovalTemplate, notifySvc.Check, savedsearch.Check(rec), forms.Check(rec), portal.Check(rec), snippets.Check(rec), dashboards.Check)
	// The text modules (SD8) fill in their placeholders like the notifications.
	snippetSvc := snippets.New(rec)
	snippetSvc.PublicURL = cfg.PublicURL
	after := rec.AfterWrite
	rec.AfterWrite = func(ctx context.Context, typeName string) {
		after(ctx, typeName)
		slaSvc.Wrote(ctx, typeName)
		ev.Wake(ctx)
	}
	// Files (PL18): the file system below ITSM_FILES_DIR; without it the endpoints answer 503.
	var (
		fileSvc   *files.Service
		bulkAsync *bulk.Async
	)
	if cfg.FilesDir != "" {
		store, err := files.NewFS(cfg.FilesDir)
		if err != nil {
			return err
		}
		fileSvc = files.New(rt, store, rec, orgSvc.Setting, logger)
		if err := files.RegisterCleanupJob(js, fileSvc); err != nil {
			return err
		}
		// The asynchronous bulk (PL32) keeps its streams in the same store.
		bulkAsync = bulk.NewAsync(store, js, rec, az.Load, orgSvc.Setting, logger)
		bulkAsync.Queue = cfg.BulkQueue
		bulkAsync.Register()
	}
	// Exports (UI5): up to 100,000 rows in the request; larger ones as jobs on the bulk queue,
	// whose files need the file store.
	exportSvc := export.New(rec, fileSvc, js, az.Load, logger)
	exportSvc.Queue = cfg.BulkQueue
	if fileSvc != nil {
		exportSvc.Register()
	}
	q := queries.New(rt.Control())
	checks := []httpapi.Check{
		{Name: "database", Fn: func(ctx context.Context) error {
			_, err := q.DatabaseNow(ctx)
			return err
		}},
		{Name: "valkey", Fn: kv.Ping},
	}
	rels := relations.New(rt, rec)
	handler := httpapi.NewRouter(httpapi.Options{
		RequestTimeout: cfg.RequestTimeout,
		Logger:         logger,
		Version:        platform.Version,
		Checks:         checks,
		API:            cfg.Role.RunsAPI(),
		Records:        rec,
		Auth:           authSvc,
		Org:            orgSvc,
		Settings:       settingsSvc,
		Dashboards:     dashboards,
		Groups:         query.NewGroups(rt),
		Authz:          az,
		Jobs:           js,
		Events:         ev,
		Authn:          an,
		TrustProxy:     cfg.TrustProxy,
		Relations:      rels,
		Files:          fileSvc,
		Webhooks:       hooks,
		Search:         search.NewPostgres(rec),
		SecLog:         secLog,
		KV:             kv,
		Presence:       presence.New(kv),
		Prefs:          prefs.New(rt, rec, orgSvc.Setting),
		Forms:          forms.New(rec).WithRelations(rels),
		Portal:         portal.New(rec),
		Snippets:       snippetSvc,
		Customer:       customer.New(rt, rec, logger),
		Bulk:           bulkAsync,
		Export:         exportSvc,
		Rules:          ruleEngine,
		Calendars:      calendars,
		SLA:            slaSvc,
		Notify:         notifySvc,
		WebDir:         cfg.WebDir,
	})

	// The caches are loaded before the first request (PL58): only the very first load runs in a
	// request, and a burst of writes at the start must not find them empty. A database without
	// the core package yet has nothing to load; the requests load then.
	if _, err := rec.Types(ctx); err != nil {
		logger.Warn("itsm: types not loaded before the start", "error", err.Error())
	}
	for name, warm := range map[string]func(context.Context) error{"sla": slaSvc.Warm, "rules": ruleEngine.Warm, "settings": settingsSvc.Warm} {
		if err := warm(ctx); err != nil {
			logger.Warn("itsm: cache not loaded before the start", "cache", name, "error", err.Error())
		}
	}
	logger.Info("itsm starting", "version", platform.Version, "role", string(cfg.Role), "addr", cfg.HTTPAddr)

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Go(func() {
		errs <- httpapi.Serve(ctx, cfg.HTTPAddr, handler, cfg.ShutdownTimeout, logger)
	})
	// Rights change on any instance reach this one through Valkey (PL9).
	go func() { _ = az.Run(ctx) }()
	// The types in memory hear of the changes of other instances and of the command line (PL53).
	go rec.WatchTypes(ctx, logger)
	go func() { _ = ruleEngine.Run(ctx) }()
	go func() { _ = slaSvc.Run(ctx) }()
	if err := js.Start(ctx); err != nil {
		stop()
		return err
	}
	wg.Go(func() {
		<-ctx.Done()
		stopCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		errs <- js.Stop(stopCtx)
	})
	// The dispatcher of the outbox runs in the worker role (PL24, E26).
	if cfg.Role.RunsWorker() {
		wg.Go(func() {
			errs <- ev.RunDispatcher(ctx)
		})
	}

	var first error
	go func() {
		wg.Wait()
		close(errs)
	}()
	for err := range errs {
		if err != nil && first == nil {
			first = err
			stop()
		}
	}
	logger.Info("itsm stopped")
	return first
}

func migrate(getenv func(string) string, stdout io.Writer) error {
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	logger := platform.NewLogger(stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := db.Migrate(ctx, cfg.DatabaseURL, logger); err != nil {
		return err
	}
	if !cfg.SeparateDatabases() {
		return nil
	}
	// With a database per tenant every tenant of the directory is migrated too (PL47d).
	control, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer control.Close()
	return db.MigrateTenants(ctx, control, cfg.TenantURL, logger)
}

func healthcheck(getenv func(string) string) error {
	addr := getenv("ITSM_HTTP_ADDR")
	if addr == "" {
		addr = platform.DefaultHTTPAddr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("ITSM_HTTP_ADDR %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("healthz answered " + resp.Status)
	}
	return nil
}
