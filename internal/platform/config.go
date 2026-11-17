// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package platform holds configuration, logging and version information shared by all
// building blocks (architektur.md §2, "Plattform").
package platform

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Version is set at build time with -ldflags "-X group24.de/itsm/internal/platform.Version=…".
var Version = "dev"

// DefaultHTTPAddr is the listen address when ITSM_HTTP_ADDR is not set.
const DefaultHTTPAddr = ":8080"

// Role selects what a process runs (Hauptplan E35).
type Role string

const (
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
	RoleAll    Role = "all"
)

// ParseRole validates a role name.
func ParseRole(s string) (Role, error) {
	switch r := Role(strings.ToLower(strings.TrimSpace(s))); r {
	case RoleAPI, RoleWorker, RoleAll:
		return r, nil
	default:
		return "", fmt.Errorf("unknown role %q, want api, worker or all", s)
	}
}

// RunsAPI reports whether the role serves the public API and the web interface.
func (r Role) RunsAPI() bool { return r == RoleAPI || r == RoleAll }

// RunsWorker reports whether the role runs background work.
func (r Role) RunsWorker() bool { return r == RoleWorker || r == RoleAll }

// Config is read from environment variables only (no config files), so that Compose and
// Helm configure the binary the same way.
type Config struct {
	Role            Role
	HTTPAddr        string
	DatabaseURL     string
	ValkeyAddr      string
	FilesDir        string
	WebDir          string
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
	// RequestTimeout ends an API request and its statements in PostgreSQL after this long
	// (PL56), ITSM_REQUEST_TIMEOUT, 10 s unless set, 0 for none; export, bulk stream, event
	// stream and file upload are left out.
	RequestTimeout time.Duration
	// Evaluations is how many evaluations of dashboard widgets run at once on this instance
	// (PL67, ITSM_EVALUATIONS, 4 unless set, 0 for no bound); the others wait for a place, so
	// that lists, searches and writing do not wait behind long counts.
	Evaluations int
	// Workers per queue for the worker role (Hauptplan E85): ITSM_WORKERS_INTERACTIVE,
	// ITSM_WORKERS_DEFAULT, ITSM_WORKERS_BULK; 0 leaves a queue unworked on this instance.
	Workers map[string]int
	// BulkQueue is the River queue of the asynchronous bulk (PL32), ITSM_BULK_QUEUE, "bulk"
	// unless set; ITSM_WORKERS_BULK counts for it. The load stage of the CI uses its own, so
	// that an instance on the same database does not take its jobs (WF3).
	BulkQueue string
	// EventRetention is how long events stay readable (E46), ITSM_EVENT_RETENTION.
	EventRetention time.Duration
	// PublicURL is this server as browsers reach it (ITSM_PUBLIC_URL), for redirects back
	// from an identity provider.
	PublicURL string
	// OIDC is the identity provider (E88); Issuer empty means sign-in through a provider is off.
	OIDC OIDC
	// TrustProxy takes the caller's address from X-Forwarded-For (ITSM_TRUST_PROXY), for the
	// address ranges of API clients behind a reverse proxy (PL13).
	TrustProxy bool
	// SMTP is the mail server of the notifications (WF15); Host empty means no mail.
	SMTP SMTP
	// TenantMode is where the data of the tenants live (PL47d, Hauptplan E141, E144),
	// ITSM_TENANT_MODE: TenantModeShared (the default) keeps everything in ITSM_DATABASE_URL;
	// TenantModeDatabase gives every tenant a database of its own, whose URL only the
	// configuration holds (TenantURL).
	TenantMode string
	// TenantURL reads the URL of a tenant's database by the tenant's name from
	// ITSM_TENANT_<NAME>_DATABASE_URL (TenantDatabaseVar); empty when it is not set.
	TenantURL func(name string) string
}

// The operating modes of the tenants (ITSM_TENANT_MODE).
const (
	TenantModeShared   = "shared"
	TenantModeDatabase = "database"
)

// TenantDatabaseVar is the variable that holds the URL of a tenant's database: the tenant's
// name in capitals, "nord" → ITSM_TENANT_NORD_DATABASE_URL.
func TenantDatabaseVar(name string) string {
	return "ITSM_TENANT_" + strings.ToUpper(name) + "_DATABASE_URL"
}

// SeparateDatabases reports whether the tenants have databases of their own.
func (c Config) SeparateDatabases() bool { return c.TenantMode == TenantModeDatabase }

// SMTP configures the mail out (ITSM_SMTP_HOST, _PORT, _FROM, _USER, _PASSWORD, _STARTTLS).
type SMTP struct {
	Host     string
	Port     int
	From     string
	User     string
	Password string
	StartTLS bool
}

// OIDC configures sign-in through an identity provider (PL11).
type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Name         string
	InternalURL  string
	GroupsClaim  string
	Scopes       []string
}

// queueRe is the rule for the name of a queue set by the environment.
var queueRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// DefaultWorkers is the number of workers per queue of one instance.
var DefaultWorkers = map[string]int{"interactive": 8, "default": 8, "bulk": 2}

// LoadConfig reads the configuration through getenv (os.Getenv in production).
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		Role:            RoleAll,
		HTTPAddr:        DefaultHTTPAddr,
		DatabaseURL:     getenv("ITSM_DATABASE_URL"),
		ValkeyAddr:      getenv("ITSM_VALKEY_ADDR"),
		FilesDir:        getenv("ITSM_FILES_DIR"),
		WebDir:          getenv("ITSM_WEB_DIR"),
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: 15 * time.Second,
		RequestTimeout:  10 * time.Second,
		Evaluations:     4,
		Workers:         map[string]int{},
		EventRetention:  7 * 24 * time.Hour,
	}
	var errs []error
	for queue, n := range DefaultWorkers {
		cfg.Workers[queue] = n
		key := "ITSM_WORKERS_" + strings.ToUpper(queue)
		if v := getenv(key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				errs = append(errs, fmt.Errorf("%s: %q is not a number of workers", key, v))
				continue
			}
			cfg.Workers[queue] = n
		}
	}
	if v := getenv("ITSM_EVALUATIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			errs = append(errs, fmt.Errorf("ITSM_EVALUATIONS: %q is not a number of evaluations", v))
		} else {
			cfg.Evaluations = n
		}
	}
	cfg.BulkQueue = "bulk"
	if v := getenv("ITSM_BULK_QUEUE"); v != "" && v != cfg.BulkQueue {
		if !queueRe.MatchString(v) {
			errs = append(errs, fmt.Errorf("ITSM_BULK_QUEUE: %q is not a queue name (a-z, 0-9, _)", v))
		} else {
			cfg.Workers[v] = cfg.Workers[cfg.BulkQueue]
			delete(cfg.Workers, cfg.BulkQueue)
			cfg.BulkQueue = v
		}
	}
	cfg.PublicURL = strings.TrimRight(getenv("ITSM_PUBLIC_URL"), "/")
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:8080"
	}
	cfg.OIDC = OIDC{
		Issuer: getenv("ITSM_OIDC_ISSUER"), ClientID: getenv("ITSM_OIDC_CLIENT_ID"), ClientSecret: getenv("ITSM_OIDC_CLIENT_SECRET"),
		Name: getenv("ITSM_OIDC_NAME"), InternalURL: getenv("ITSM_OIDC_INTERNAL_URL"), GroupsClaim: getenv("ITSM_OIDC_GROUPS_CLAIM"),
		Scopes: strings.Fields(getenv("ITSM_OIDC_SCOPES")),
	}
	if v := getenv("ITSM_TRUST_PROXY"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ITSM_TRUST_PROXY: %q is not true or false", v))
		}
		cfg.TrustProxy = b
	}
	cfg.SMTP = SMTP{Host: getenv("ITSM_SMTP_HOST"), Port: 25, From: getenv("ITSM_SMTP_FROM"), User: getenv("ITSM_SMTP_USER"),
		Password: getenv("ITSM_SMTP_PASSWORD")}
	if v := getenv("ITSM_SMTP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 65535 {
			errs = append(errs, fmt.Errorf("ITSM_SMTP_PORT: %q is not a port", v))
		}
		cfg.SMTP.Port = n
	}
	if v := getenv("ITSM_SMTP_STARTTLS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ITSM_SMTP_STARTTLS: %q is not true or false", v))
		}
		cfg.SMTP.StartTLS = b
	}
	if cfg.SMTP.Host != "" && cfg.SMTP.From == "" {
		errs = append(errs, errors.New("ITSM_SMTP_FROM is required with ITSM_SMTP_HOST"))
	}
	if cfg.SMTP.User != "" && !cfg.SMTP.StartTLS {
		errs = append(errs, errors.New("ITSM_SMTP_USER needs ITSM_SMTP_STARTTLS=true: no password over a plain connection"))
	}
	if cfg.OIDC.Issuer != "" && cfg.OIDC.ClientID == "" {
		errs = append(errs, errors.New("ITSM_OIDC_CLIENT_ID is required with ITSM_OIDC_ISSUER"))
	}
	if v := getenv("ITSM_EVENT_RETENTION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("ITSM_EVENT_RETENTION: %q is not a duration", v))
		} else {
			cfg.EventRetention = d
		}
	}
	if v := getenv("ITSM_ROLE"); v != "" {
		r, err := ParseRole(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ITSM_ROLE: %w", err))
		}
		cfg.Role = r
	}
	if v := getenv("ITSM_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := getenv("ITSM_LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("ITSM_LOG_LEVEL: %w", err))
		}
	}
	if v := getenv("ITSM_SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("ITSM_SHUTDOWN_TIMEOUT: %w", err))
		}
		cfg.ShutdownTimeout = d
	}
	if v := getenv("ITSM_REQUEST_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			errs = append(errs, fmt.Errorf("ITSM_REQUEST_TIMEOUT: %q is no duration like 10s (0 for none)", v))
		} else {
			cfg.RequestTimeout = d
		}
	}
	cfg.TenantMode = TenantModeShared
	if v := getenv("ITSM_TENANT_MODE"); v != "" {
		if v != TenantModeShared && v != TenantModeDatabase {
			errs = append(errs, fmt.Errorf("ITSM_TENANT_MODE: %q is neither %s nor %s", v, TenantModeShared, TenantModeDatabase))
		}
		cfg.TenantMode = v
	}
	cfg.TenantURL = func(name string) string { return getenv(TenantDatabaseVar(name)) }
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("ITSM_DATABASE_URL is required"))
	}
	if cfg.ValkeyAddr == "" {
		// Valkey is mandatory from the start (Hauptplan E20).
		errs = append(errs, errors.New("ITSM_VALKEY_ADDR is required"))
	}
	return cfg, errors.Join(errs...)
}
