// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package recurrence

import (
	"testing"
	"time"

	"group24.de/itsm/internal/meta"
)

func TestFill(t *testing.T) {
	typ := meta.Type{Name: "probe", Package: "probe", Fields: []meta.Field{
		{Name: "title", Type: meta.FieldText}, {Name: "due_on", Type: meta.FieldDate},
		{Name: "check_at", Type: meta.FieldDateTime}, {Name: "note", Type: meta.FieldText}, {Name: "priority", Type: meta.FieldNumber}}}
	berlin, _ := time.LoadLocation("Europe/Berlin")
	date := time.Date(2026, 11, 9, 7, 0, 0, 0, berlin)
	got, err := fill(`{"title": "Prüfung {{date}} KW {{week}} {{month}}/{{year}}", "due_on": "+3d", "check_at": "+4h",
		"note": "+3d", "priority": 2}`, date, typ)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"title": "Prüfung 2026-11-09 KW 46 11/2026", "due_on": "2026-11-12",
		"check_at": "2026-11-09T10:00:00Z", "note": "+3d", "priority": float64(2)}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %v, want %v", k, got[k], v)
		}
	}
	// A week later, over the change to winter time it stays 7:00 local.
	if got, _ := fill(`{"due_on": "+1w"}`, time.Date(2026, 10, 19, 7, 0, 0, 0, berlin), typ); got["due_on"] != "2026-10-26" {
		t.Errorf("+1w: %v", got["due_on"])
	}
	for _, bad := range []string{"kein JSON", "[1, 2]", "null"} {
		if _, err := fill(bad, date, typ); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestSchedule(t *testing.T) {
	sched, loc, err := schedule("0 7 * * 1", "Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	// From Sunday 25 October 2026 (winter time from that night) the next Monday at 7 Berlin.
	next := sched.Next(time.Date(2026, 10, 25, 12, 0, 0, 0, loc))
	if !next.Equal(time.Date(2026, 10, 26, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("next: %v", next)
	}
	for _, bad := range []string{"jeden Montag", "CRON_TZ=UTC 0 7 * * 1", ""} {
		if _, _, err := schedule(bad, "Europe/Berlin"); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
	if _, _, err := schedule("0 7 * * 1", "Mars/Base"); err == nil {
		t.Error("unknown time zone: no error")
	}
}
