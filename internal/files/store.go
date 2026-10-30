// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Store keeps the content of files under a key (Hauptplan E22): the file system in Compose
// and development, the S3 API in the cluster later (V23). Put writes the whole stream or
// nothing; Delete removes a key and whatever a broken Put left behind.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader) (int64, error)
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, key string) error
}

// ErrNoObject says the store has nothing under the key.
var ErrNoObject = errors.New("files: no such object")

// FS is a Store in a directory. A Put streams into <key>.part and renames it at the end, so
// that a reader never sees half a file; a crash leaves the .part file for the cleanup job.
type FS struct {
	root string
}

// NewFS returns a store below dir, which must exist and be writable.
func NewFS(dir string) (*FS, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("files: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("files: %s is not a directory", dir)
	}
	return &FS{root: dir}, nil
}

func (s *FS) path(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if key == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("files: bad key %q", key)
	}
	return filepath.Join(s.root, clean), nil
}

// Put writes r under key.
func (s *FS) Put(ctx context.Context, key string, r io.Reader) (int64, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return 0, fmt.Errorf("files: %w", err)
	}
	part := p + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640) //nolint:gosec // G304: the path is built from an internal key below the root
	if err != nil {
		return 0, fmt.Errorf("files: %w", err)
	}
	n, err := io.Copy(f, ctxReader{ctx: ctx, r: r})
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(part, p)
	}
	if err != nil {
		_ = os.Remove(part)
		return n, err
	}
	return n, nil
}

// Open returns the content under key.
func (s *FS) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p) //nolint:gosec // G304: the path is built from an internal key below the root
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, key)
	}
	return f, err
}

// Delete removes the content under key and a leftover .part file; missing ones are fine.
func (s *FS) Delete(_ context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	for _, name := range []string{p, p + ".part"} {
		if err := os.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("files: %w", err)
		}
	}
	return nil
}

// ctxReader stops a copy when the request ends.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
