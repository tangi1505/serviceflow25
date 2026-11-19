// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"errors"
	"testing"
	"time"
)

// TestPasswordChecksAreBounded: with every place taken, a further check waits argonWait and
// gives ErrBusy instead of taking 19 MiB more (AP6); a free place lets it run again.
func TestPasswordChecksAreBounded(t *testing.T) {
	slots, wait := argonSlots, argonWait
	t.Cleanup(func() { argonSlots, argonWait = slots, wait })
	argonSlots, argonWait = make(chan struct{}, 1), 50*time.Millisecond

	hash, err := HashPassword("Wolke-Tisch-7!")
	if err != nil {
		t.Fatal(err)
	}
	argonSlots <- struct{}{}
	start := time.Now()
	if _, err := HashPassword("Wolke-Tisch-7!"); !errors.Is(err, ErrBusy) {
		t.Fatalf("hash with no place: %v", err)
	}
	if _, err := VerifyPassword(hash, "Wolke-Tisch-7!"); !errors.Is(err, ErrBusy) {
		t.Fatalf("verify with no place: %v", err)
	}
	if took := time.Since(start); took < 100*time.Millisecond || took > time.Second {
		t.Fatalf("two waits of 50 ms took %s", took)
	}
	<-argonSlots
	if ok, err := VerifyPassword(hash, "Wolke-Tisch-7!"); err != nil || !ok {
		t.Fatalf("verify with a place: %v %v", ok, err)
	}
}
