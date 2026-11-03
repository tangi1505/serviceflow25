// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package db opens the PostgreSQL pool and migrates the core tables (architektur.md §3.1).
// Core tables are accessed through sqlc (package queries); type tables later through the own
// SQL builder (Hauptplan E43).
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"group24.de/itsm/db/migrations"
)

// Open creates the connection pool and checks that the database answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("db: parse ITSM_DATABASE_URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending migrations of the core tables. A PostgreSQL session lock makes
// it safe when several instances start at the same time.
func Migrate(ctx context.Context, url string, logger *slog.Logger) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return fmt.Errorf("db: open for migration: %w", err)
	}
	defer sqlDB.Close()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("db: migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS,
		goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("db: migration provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	for _, r := range results {
		logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path,
			"duration", r.Duration.String())
	}
	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("db: read version: %w", err)
	}
	logger.Info("database migrated", "version", current, "applied", len(results))
	return migrateRiver(ctx, url, logger)
}

// riverLock is the key of the advisory lock around the River migration. Without it, two
// processes starting at once on an empty database both create River's types and one fails
// (found in the CI on 2026-11-02); goose holds its own lock for the migrations above.
const riverLock int64 = 0x72697665 // "rive"

// migrateRiver applies the tables of the job queue (Hauptplan E19). River versions them in
// its own table river_migration, so they do not clash with the goose versions above.
func migrateRiver(ctx context.Context, url string, logger *slog.Logger) error {
	pool, err := Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	// One process at a time: the lock is held on a connection of its own while the migrator
	// works on the others of the pool.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("db: river migration lock: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", riverLock); err != nil {
		return fmt.Errorf("db: river migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", riverLock)
	}()
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{Logger: logger})
	if err != nil {
		return fmt.Errorf("db: river migrator: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("db: migrate river: %w", err)
	}
	for _, v := range res.Versions {
		logger.Info("river migration applied", "version", v.Version, "name", v.Name)
	}
	return nil
}
