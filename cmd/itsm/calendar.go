// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/google/uuid"

	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/platform"
	"group24.de/itsm/internal/records"
)

// itsm calendar holidays <calendar-id> <state> <year>: the public holidays of a German state
// in a year into a calendar of the core package (WF8); days it has stay as they are.
func calendarCmd(args []string, getenv func(string) string, stdout, _ io.Writer) error {
	if len(args) != 4 || args[0] != "holidays" {
		return errors.New("usage: itsm calendar holidays <calendar-id> <state, e.g. NW> <year>")
	}
	id, err := uuid.Parse(args[1])
	if err != nil {
		return fmt.Errorf("calendar id %q: %w", args[1], err)
	}
	year, err := strconv.Atoi(args[3])
	if err != nil {
		return fmt.Errorf("year %q: %w", args[3], err)
	}
	cfg, err := platform.LoadConfig(getenv)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rt, closeDBs, err := openDatabases(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeDBs()
	n, err := calendar.New(records.New(rt), org.New(rt).Setting).GenerateHolidays(ctx, id, year, args[2], nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d holidays of %s %d added to calendar %s\n", n, args[2], year, id)
	return err
}
