// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package presence

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/cache"
)

// Who is on a record: the others, editing first; a beat keeps its start; after 60 seconds
// without a beat someone is gone; leaving takes one off at once.
func TestPresence(t *testing.T) {
	addr := os.Getenv("ITSM_TEST_VALKEY_ADDR")
	if addr == "" {
		t.Skip("ITSM_TEST_VALKEY_ADDR not set")
	}
	kv, err := cache.Open(t.Context(), addr)
	if err != nil {
		t.Fatal(err)
	}
	defer kv.Close()
	s := New(kv)
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	rec, anna, ben, carl := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	if others, err := s.Beat(t.Context(), "probe.probe", rec, anna, "Anna", false); err != nil || len(others) != 0 {
		t.Fatalf("first: %v %v", others, err)
	}
	now = now.Add(10 * time.Second)
	if _, err := s.Beat(t.Context(), "probe.probe", rec, carl, "Carl", true); err != nil {
		t.Fatal(err)
	}
	others, err := s.Beat(t.Context(), "probe.probe", rec, ben, "Ben", false)
	if err != nil || len(others) != 2 || others[0].Name != "Carl" || !others[0].Editing || others[1].Name != "Anna" {
		t.Fatalf("Ben sees %+v %v", others, err)
	}
	// Anna beats again: she keeps her start.
	now = now.Add(15 * time.Second)
	if _, err := s.Beat(t.Context(), "probe.probe", rec, ben, "Ben", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Beat(t.Context(), "probe.probe", rec, anna, "Anna", true); err != nil {
		t.Fatal(err)
	}
	others, _ = s.Beat(t.Context(), "probe.probe", rec, ben, "Ben", false)
	for _, e := range others {
		if e.Name == "Anna" && (!e.Editing || !e.Since.Equal(time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC))) {
			t.Fatalf("Anna: %+v", e)
		}
	}
	// Carl has not beaten for more than 60 seconds; Anna leaves.
	now = now.Add(50 * time.Second)
	if err := s.Leave(t.Context(), "probe.probe", rec, anna); err != nil {
		t.Fatal(err)
	}
	if others, _ := s.Beat(t.Context(), "probe.probe", rec, ben, "Ben", false); len(others) != 0 {
		t.Fatalf("after leave and timeout: %+v", others)
	}
}
