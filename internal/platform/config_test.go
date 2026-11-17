// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package platform

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func required() map[string]string {
	return map[string]string{
		"ITSM_DATABASE_URL": "postgres://itsm@db/itsm",
		"ITSM_VALKEY_ADDR":  "valkey:6379",
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := LoadConfig(env(required()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != RoleAll || cfg.HTTPAddr != DefaultHTTPAddr || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Fatalf("shutdown timeout = %s", cfg.ShutdownTimeout)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Fatalf("request timeout = %s (PL56)", cfg.RequestTimeout)
	}
	if cfg.Workers["interactive"] != 8 || cfg.Workers["default"] != 8 || cfg.Workers["bulk"] != 2 || cfg.EventRetention != 7*24*time.Hour {
		t.Fatalf("unexpected job defaults: %+v", cfg)
	}
}

// ITSM_BULK_QUEUE moves the bulk workers to a queue of its own (WF3): the load stage of the
// CI must not share its jobs with an instance on the same database.
func TestLoadConfigBulkQueue(t *testing.T) {
	m := required()
	cfg, err := LoadConfig(env(m))
	if err != nil || cfg.BulkQueue != "bulk" || cfg.Workers["bulk"] != 2 {
		t.Fatalf("default: %+v %v", cfg, err)
	}
	m["ITSM_BULK_QUEUE"] = "bulkload"
	m["ITSM_WORKERS_BULK"] = "3"
	cfg, err = LoadConfig(env(m))
	if _, old := cfg.Workers["bulk"]; err != nil || cfg.BulkQueue != "bulkload" || cfg.Workers["bulkload"] != 3 || old {
		t.Fatalf("own queue: %+v %v", cfg, err)
	}
	m["ITSM_BULK_QUEUE"] = "Bulk Load"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_BULK_QUEUE") {
		t.Fatalf("bad name: %v", err)
	}
}

// ITSM_TENANT_MODE picks where the tenants' data live (PL47d); the URL of a tenant's database
// comes from a variable of its name.
func TestLoadConfigTenantMode(t *testing.T) {
	m := required()
	cfg, err := LoadConfig(env(m))
	if err != nil || cfg.TenantMode != TenantModeShared || cfg.SeparateDatabases() {
		t.Fatalf("default: %q %v", cfg.TenantMode, err)
	}
	m["ITSM_TENANT_MODE"] = "database"
	m["ITSM_TENANT_NORD_DATABASE_URL"] = "postgres://itsm@db/nord"
	cfg, err = LoadConfig(env(m))
	if err != nil || !cfg.SeparateDatabases() || cfg.TenantURL("nord") != "postgres://itsm@db/nord" || cfg.TenantURL("sued") != "" {
		t.Fatalf("database: %q %v", cfg.TenantMode, err)
	}
	if TenantDatabaseVar("big_city") != "ITSM_TENANT_BIG_CITY_DATABASE_URL" {
		t.Fatal(TenantDatabaseVar("big_city"))
	}
	m["ITSM_TENANT_MODE"] = "separate"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_TENANT_MODE") {
		t.Fatalf("bad mode: %v", err)
	}
}

func TestLoadConfigReadsWorkers(t *testing.T) {
	m := required()
	m["ITSM_WORKERS_BULK"] = "0"
	m["ITSM_WORKERS_INTERACTIVE"] = "16"
	m["ITSM_EVENT_RETENTION"] = "48h"
	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workers["bulk"] != 0 || cfg.Workers["interactive"] != 16 || cfg.Workers["default"] != 8 || cfg.EventRetention != 48*time.Hour {
		t.Fatalf("unexpected workers: %+v", cfg)
	}
	m["ITSM_WORKERS_DEFAULT"] = "-1"
	m["ITSM_EVENT_RETENTION"] = "soon"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_WORKERS_DEFAULT") || !strings.Contains(err.Error(), "ITSM_EVENT_RETENTION") {
		t.Fatalf("bad values must be reported: %v", err)
	}
}

func TestLoadConfigReadsOIDC(t *testing.T) {
	m := required()
	if cfg, err := LoadConfig(env(m)); err != nil || cfg.OIDC.Issuer != "" || cfg.PublicURL != "http://localhost:8080" {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	m["ITSM_OIDC_ISSUER"] = "http://localhost:8091/realms/itsm"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_OIDC_CLIENT_ID") {
		t.Fatalf("issuer without client id must be reported: %v", err)
	}
	m["ITSM_OIDC_CLIENT_ID"] = "itsm"
	m["ITSM_OIDC_SCOPES"] = "openid profile email groups"
	m["ITSM_PUBLIC_URL"] = "http://localhost:8092/"
	cfg, err := LoadConfig(env(m))
	if err != nil || cfg.OIDC.ClientID != "itsm" || len(cfg.OIDC.Scopes) != 4 || cfg.PublicURL != "http://localhost:8092" {
		t.Fatalf("oidc: %+v %v", cfg, err)
	}
}

// The mail server of the notifications (WF15): off without a host; a sender is required, a
// login only over STARTTLS.
func TestLoadConfigReadsSMTP(t *testing.T) {
	m := required()
	if cfg, err := LoadConfig(env(m)); err != nil || cfg.SMTP.Host != "" || cfg.SMTP.Port != 25 {
		t.Fatalf("defaults: %+v %v", cfg.SMTP, err)
	}
	m["ITSM_SMTP_HOST"] = "mailpit"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_SMTP_FROM") {
		t.Fatalf("host without sender: %v", err)
	}
	m["ITSM_SMTP_FROM"] = "ITSM <itsm@localhost>"
	m["ITSM_SMTP_PORT"] = "1025"
	cfg, err := LoadConfig(env(m))
	if err != nil || cfg.SMTP.Port != 1025 || cfg.SMTP.From != "ITSM <itsm@localhost>" || cfg.SMTP.StartTLS {
		t.Fatalf("smtp: %+v %v", cfg.SMTP, err)
	}
	m["ITSM_SMTP_USER"] = "itsm"
	m["ITSM_SMTP_PORT"] = "smtp"
	if _, err := LoadConfig(env(m)); err == nil || !strings.Contains(err.Error(), "ITSM_SMTP_STARTTLS") || !strings.Contains(err.Error(), "ITSM_SMTP_PORT") {
		t.Fatalf("login without STARTTLS and a bad port: %v", err)
	}
	m["ITSM_SMTP_PORT"] = "587"
	m["ITSM_SMTP_STARTTLS"] = "true"
	if cfg, err := LoadConfig(env(m)); err != nil || !cfg.SMTP.StartTLS || cfg.SMTP.User != "itsm" {
		t.Fatalf("login: %+v %v", cfg.SMTP, err)
	}
}

func TestLoadConfigReadsEverything(t *testing.T) {
	m := required()
	m["ITSM_ROLE"] = "Worker"
	m["ITSM_HTTP_ADDR"] = ":9000"
	m["ITSM_LOG_LEVEL"] = "debug"
	m["ITSM_SHUTDOWN_TIMEOUT"] = "3s"
	m["ITSM_REQUEST_TIMEOUT"] = "0"
	m["ITSM_EVALUATIONS"] = "8"
	m["ITSM_FILES_DIR"] = "/data/files"
	m["ITSM_WEB_DIR"] = "/web"
	cfg, err := LoadConfig(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Role != RoleWorker || cfg.HTTPAddr != ":9000" || cfg.LogLevel != slog.LevelDebug ||
		cfg.ShutdownTimeout != 3*time.Second || cfg.RequestTimeout != 0 || cfg.Evaluations != 8 || cfg.FilesDir != "/data/files" || cfg.WebDir != "/web" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigReportsAllErrors(t *testing.T) {
	_, err := LoadConfig(env(map[string]string{"ITSM_ROLE": "boss", "ITSM_LOG_LEVEL": "loud", "ITSM_REQUEST_TIMEOUT": "soon", "ITSM_EVALUATIONS": "-1"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"ITSM_ROLE", "ITSM_LOG_LEVEL", "ITSM_DATABASE_URL", "ITSM_VALKEY_ADDR", "ITSM_REQUEST_TIMEOUT", "ITSM_EVALUATIONS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestRoles(t *testing.T) {
	cases := []struct {
		role        Role
		api, worker bool
	}{
		{RoleAPI, true, false},
		{RoleWorker, false, true},
		{RoleAll, true, true},
	}
	for _, c := range cases {
		if c.role.RunsAPI() != c.api || c.role.RunsWorker() != c.worker {
			t.Errorf("%s: api=%v worker=%v", c.role, c.role.RunsAPI(), c.role.RunsWorker())
		}
	}
}

func TestLoggerWritesJSON(t *testing.T) {
	var buf bytes.Buffer
	NewLogger(&buf, slog.LevelInfo).Info("hello", "k", "v")
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("not JSON: %q", buf.String())
	}
	if line["msg"] != "hello" || line["k"] != "v" {
		t.Fatalf("unexpected log line: %v", line)
	}
}
