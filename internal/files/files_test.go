// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
	"group24.de/itsm/internal/schema"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// brokenReader delivers some bytes and then fails, like a client that hangs up.
type brokenReader struct{ n int }

func (b *brokenReader) Read(p []byte) (int, error) {
	if b.n <= 0 {
		return 0, errors.New("connection reset")
	}
	k := min(len(p), b.n)
	for i := range k {
		p[i] = 'x'
	}
	b.n -= k
	return k, nil
}

func TestFSStoreWritesWholeOrNothing(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if _, err := s.Put(ctx, "ab/cd/one", strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}
	r, err := s.Open(ctx, "ab/cd/one")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	_ = r.Close()
	if string(b) != "hello" {
		t.Fatalf("content %q", b)
	}
	if _, err := s.Put(ctx, "ab/cd/two", &brokenReader{n: 10000}); err == nil {
		t.Fatal("a broken stream must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "ab", "cd", "two.part")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("part file left behind: %v", err)
	}
	if _, err := s.Open(ctx, "ab/cd/two"); !errors.Is(err, ErrNoObject) {
		t.Fatalf("half a file is visible: %v", err)
	}
	for _, bad := range []string{"", "../x", "/etc/passwd"} {
		if _, err := s.Put(ctx, bad, strings.NewReader("x")); err == nil {
			t.Fatalf("key %q accepted", bad)
		}
	}
	if err := s.Delete(ctx, "ab/cd/one"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "ab/cd/one"); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
}

func TestContentDispositionAndNames(t *testing.T) {
	if got := ContentDisposition(`Bericht "März".pdf`); got != `attachment; filename="Bericht _M_rz_.pdf"; filename*=UTF-8''Bericht%20%22M%C3%A4rz%22.pdf` {
		t.Fatalf("disposition: %s", got)
	}
	for in, want := range map[string]string{`C:\temp\a.txt`: "a.txt", "../../etc/passwd": "passwd", " x\ny.log ": "xy.log", "/": ""} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

type fixture struct {
	svc  *Service
	rec  *records.Service
	pool *pgxpool.Pool
	dir  string
	ctx  context.Context
	doc  string // a record of filestest.doc (trait documents)
}

func setup(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("ITSM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ITSM_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	if err := db.Migrate(ctx, url, quiet); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, sql := range []string{
		"DROP TABLE IF EXISTS rec_filestest_doc", "DROP TABLE IF EXISTS rec_filestest_plain",
		"DELETE FROM package WHERE name = 'filestest'",
		"DELETE FROM audit WHERE type_name LIKE 'filestest.%'",
		"DELETE FROM attachment WHERE type_name LIKE 'filestest.%'",
		"DELETE FROM file WHERE name LIKE 'filestest%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":     {Data: []byte("name: filestest\nversion: 0.1.0\nlabel: {de: F, en: F}\n")},
		"types/doc.yaml":   {Data: []byte("name: doc\nlabel: {de: D, en: D}\ntraits: [history, documents]\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		"types/plain.yaml": {Data: []byte("name: plain\nlabel: {de: P, en: P}\nfields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	store, err := NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := records.New(db.Single(pool))
	svc := New(db.Single(pool), store, rec, nil, quiet)
	svc.limits = func(context.Context) Limits {
		return Limits{MaxSize: 1 << 20, Blocked: DefaultBlockedExtensions, TemporaryTTL: 24 * time.Hour}
	}
	doc, err := rec.Create(ctx, "filestest.doc", map[string]any{"title": "with files"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, rec: rec, pool: pool, dir: dir, ctx: ctx, doc: doc["id"].(string)}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// The acceptance of PL18 inside the service: upload, attach, read back with the same SHA-256;
// limits; a broken upload leaves nothing; the cleanup job removes what never finished and
// temporary files nobody attached, and keeps attached ones.
func TestUploadAttachReadAndCleanup(t *testing.T) {
	f := setup(t)
	content := bytes.Repeat([]byte("%PDF-1.7 probe\n"), 5000)
	sum := sha256.Sum256(content)
	up, err := f.svc.Upload(f.ctx, "filestest Bericht.pdf", bytes.NewReader(content), nil)
	if err != nil {
		t.Fatal(err)
	}
	if up.State != "temporary" || up.Size != int64(len(content)) || up.SHA256 != hex.EncodeToString(sum[:]) || up.ContentType != "application/pdf" {
		t.Fatalf("upload: %+v", up)
	}
	docID := uuid.MustParse(f.doc)
	att, err := f.svc.Attach(f.ctx, "filestest.doc", docID, up.ID, nil)
	if err != nil || att.State != "stored" {
		t.Fatalf("attach: %+v %v", att, err)
	}
	if _, err := f.svc.Attach(f.ctx, "filestest.doc", docID, up.ID, nil); err != nil {
		t.Fatalf("attach twice: %v", err)
	}
	list, err := f.svc.List(f.ctx, "filestest.doc", docID)
	if err != nil || len(list) != 1 || list[0].ID != up.ID || list[0].AttachedAt == nil {
		t.Fatalf("list: %+v %v", list, err)
	}
	_, r, err := f.svc.Open(f.ctx, up.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if gs := sha256.Sum256(got); gs != sum {
		t.Fatal("content differs after the round trip")
	}
	hist, err := f.rec.History(f.ctx, "filestest.doc", docID)
	if err != nil || hist[len(hist)-1].Action != "attach" {
		t.Fatalf("history: %v", err)
	}
	// A type without the trait documents takes no files.
	plain, _ := f.rec.Create(f.ctx, "filestest.plain", map[string]any{"title": "x"}, nil)
	if _, err := f.svc.Attach(f.ctx, "filestest.plain", uuid.MustParse(plain["id"].(string)), up.ID, nil); !errors.Is(err, ErrNoTrait) {
		t.Fatalf("attach to a type without documents: %v", err)
	}

	// Limits: blocked extension, no name, too large; none of them leaves anything behind.
	before := countFiles(t, f.dir)
	if _, err := f.svc.Upload(f.ctx, "filestest.EXE", strings.NewReader("MZ"), nil); err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("blocked: %v", err)
	}
	if _, err := f.svc.Upload(f.ctx, "", strings.NewReader("x"), nil); err == nil {
		t.Fatal("upload without a name accepted")
	}
	if _, err := f.svc.Upload(f.ctx, "filestest big.bin", bytes.NewReader(make([]byte, 1<<20+1)), nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large: %v", err)
	}
	if _, err := f.svc.Upload(f.ctx, "filestest broken.txt", &brokenReader{n: 50000}, nil); err == nil {
		t.Fatal("a broken stream was accepted")
	}
	var leftover int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM file WHERE name LIKE 'filestest%' AND id <> $1", up.ID).Scan(&leftover); err != nil || leftover != 0 {
		t.Fatalf("rows after failed uploads: %d %v", leftover, err)
	}
	if after := countFiles(t, f.dir); after != before {
		t.Fatalf("files on disk after failed uploads: %d, before %d", after, before)
	}

	// A crash in the middle of an upload: a row "uploading" and a .part file, two hours old.
	crashID := uuid.Must(uuid.NewV7())
	key := storageKey(crashID)
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO file (id, name, state, storage_key, created_at) VALUES ($1, 'filestest crash.bin', 'uploading', $2, now() - interval '2 hours')", crashID, key); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(f.dir, filepath.FromSlash(key)+".part")
	if err := os.MkdirAll(filepath.Dir(part), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(part, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A temporary file nobody attached, 25 hours old, and a fresh one.
	old, err := f.svc.Upload(f.ctx, "filestest old.txt", strings.NewReader("old"), nil)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := f.svc.Upload(f.ctx, "filestest fresh.txt", strings.NewReader("fresh"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE file SET created_at = now() - interval '25 hours' WHERE id = ANY($1)", []uuid.UUID{old.ID, up.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Cleanup(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(part); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the part file of the crash is still there: %v", err)
	}
	for id, want := range map[uuid.UUID]bool{crashID: false, old.ID: false, fresh.ID: true, up.ID: true} {
		var n int
		if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM file WHERE id = $1", id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if (n == 1) != want {
			t.Errorf("file %s present %v, want %v", id, n == 1, want)
		}
	}
	if _, _, err := f.svc.Open(f.ctx, up.ID); err != nil {
		t.Fatalf("the attached file is gone: %v", err)
	}

	// Detach: the link goes, the history says detach, detaching again is not found.
	if err := f.svc.Detach(f.ctx, "filestest.doc", docID, up.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Detach(f.ctx, "filestest.doc", docID, up.ID, nil); !errors.Is(err, records.ErrNotFound) {
		t.Fatalf("detach twice: %v", err)
	}
	hist, _ = f.rec.History(f.ctx, "filestest.doc", docID)
	if hist[len(hist)-1].Action != "detach" {
		t.Fatalf("last history row: %s", hist[len(hist)-1].Action)
	}
}
