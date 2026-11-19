// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (Hauptplan E73): the OWASP minimum of 2026 for interactive logins,
// 19 MiB memory, two passes, one lane. A hash takes a few tens of milliseconds.
const (
	argonTime    = 2
	argonMemory  = 19 * 1024
	argonThreads = 1
	argonKeyLen  = 32
	saltLen      = 16
)

// MinPasswordLen is the minimum length in characters (E73).
const MinPasswordLen = 8

// ErrWeakPassword says the password breaks the rules (E73, settings of PL28); the error itself
// is a WeakPasswordError with the rules it breaks.
var ErrWeakPassword = errors.New("password breaks the rules of the installation")

// CheckPassword applies the rules of E73: at least 8 characters, with upper- and lower-case
// letters, a digit and a special character. The fuller rules per customer come with PL28.
func CheckPassword(pw string) error {
	var upper, lower, digit, special bool
	n := 0
	for _, r := range pw {
		n++
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			special = true
		}
	}
	if n < MinPasswordLen || !upper || !lower || !digit || !special {
		return ErrWeakPassword
	}
	return nil
}

// ErrBusy says that too many password checks run at once in this instance (AP6): the caller
// answers 503 with a short Retry-After instead of letting a flood of sign-ins take the memory.
var ErrBusy = errors.New("auth: too many password checks at once")

// Each Argon2 computation takes 19 MiB and a core for some 25 ms. Without a bound, 1,000 wrong
// sign-ins a second filled 6 GiB and the instance was killed (AP1, 2026-11-18). So as many run
// at once as the instance has cores; a further one waits at most argonWait for a place.
var (
	argonSlots = make(chan struct{}, runtime.GOMAXPROCS(0))
	argonWait  = time.Second
)

// checking runs one Argon2 computation in a place of argonSlots, or gives ErrBusy.
func checking(compute func()) error {
	select {
	case argonSlots <- struct{}{}:
	default:
		t := time.NewTimer(argonWait)
		defer t.Stop()
		select {
		case argonSlots <- struct{}{}:
		case <-t.C:
			return ErrBusy
		}
	}
	defer func() { <-argonSlots }()
	compute()
	return nil
}

// HashPassword returns the Argon2id hash in PHC string format.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	var key []byte
	if err := checking(func() { key = argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen) }); err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// VerifyPassword compares a password with a PHC hash in constant time. The parameters come from
// the hash, so hashes made with earlier parameters keep working.
func VerifyPassword(hash, pw string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("auth: unsupported password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("auth: unsupported argon2 version")
	}
	var memory, times uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &times, &threads); err != nil {
		return false, fmt.Errorf("auth: hash parameters: %w", err)
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("auth: hash salt: %w", err)
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("auth: hash key: %w", err)
	}
	if len(want) != argonKeyLen {
		return false, errors.New("auth: unsupported hash length")
	}
	var got []byte
	if err := checking(func() { got = argon2.IDKey([]byte(pw), salt, times, memory, threads, argonKeyLen) }); err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
