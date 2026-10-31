// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package files keeps files and attaches them to records (architektur.md §2 "Dateien";
// Hauptplan E22, E95; PL18). A file is uploaded first (POST /api/v1/files) and is temporary
// until it is attached to a record of a type with the trait documents; the cleanup job
// removes uploads that never finished and temporary files nobody attached. Metadata live in
// PostgreSQL, the content in a Store. Reading a file needs the right to read one of the
// records it is attached to; a temporary file only its uploader may read.
package files

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// Settings of the root node (E95).
const (
	SettingMaxSize           = "files.max_size"           // bytes, default 100 MiB
	SettingBlockedExtensions = "files.blocked_extensions" // list of extensions without dot
	SettingTemporaryTTL      = "files.temporary_ttl"      // how long an unattached file stays, default 24h
)

// Defaults of the settings.
var (
	DefaultMaxSize           int64 = 100 << 20
	DefaultBlockedExtensions       = []string{"bat", "cmd", "com", "exe", "js", "msi", "ps1", "scr", "vbs"}
	DefaultTemporaryTTL            = 24 * time.Hour
)

// uploadGrace is how long an upload may stream before the cleanup job treats it as broken.
const uploadGrace = time.Hour

var (
	// ErrTooLarge says the upload exceeds files.max_size (413).
	ErrTooLarge = errors.New("files: too large")
	// ErrNoTrait says the type has no trait documents (404).
	ErrNoTrait = errors.New("files: the type has no trait documents")
)

// Scanner checks the content of an upload before it is accepted, a virus scanner for
// instance. An error rejects the file. Which scanner the operation uses is decided in BT.
type Scanner interface {
	Scan(ctx context.Context, name string, r io.Reader) error
}

// NoScan accepts every file.
type NoScan struct{}

// Scan accepts.
func (NoScan) Scan(context.Context, string, io.Reader) error { return nil }

// File is the metadata of a file as the API shows it.
type File struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	ContentType string     `json:"content_type"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
	AttachedAt  *time.Time `json:"attached_at,omitempty"`
	AttachedBy  *uuid.UUID `json:"attached_by,omitempty"`
}

// Limits are the settings that apply to an upload.
type Limits struct {
	MaxSize      int64
	Blocked      []string
	TemporaryTTL time.Duration
}

// Settings reads a setting of a node (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Service uploads, attaches, reads and cleans up files.
type Service struct {
	db      *db.Router
	store   Store
	rec     *records.Service
	logger  *slog.Logger
	Scanner Scanner
	// limits reads the settings; tests replace it.
	limits func(ctx context.Context) Limits
}

// New returns the service. settings may be nil (then the defaults apply).
func New(rt *db.Router, store Store, rec *records.Service, settings Settings, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{db: rt, store: store, rec: rec, logger: logger, Scanner: NoScan{}}
	s.limits = func(ctx context.Context) Limits { return readLimits(ctx, settings) }
	return s
}

// tq is the query handle in the database of the request's tenant (PL47).
func (s *Service) tq(ctx context.Context) *queries.Queries { return queries.New(s.db.Tenant(ctx)) }

// Limits returns the settings that apply now.
func (s *Service) Limits(ctx context.Context) Limits { return s.limits(ctx) }

func readLimits(ctx context.Context, settings Settings) Limits {
	l := Limits{MaxSize: DefaultMaxSize, Blocked: DefaultBlockedExtensions, TemporaryTTL: DefaultTemporaryTTL}
	if settings == nil {
		return l
	}
	get := func(key string) json.RawMessage {
		raw, _, err := settings(ctx, "root", key)
		if err != nil {
			return nil
		}
		return raw
	}
	if raw := get(SettingMaxSize); len(raw) > 0 {
		var n float64
		if json.Unmarshal(raw, &n) == nil && n > 0 {
			l.MaxSize = int64(n)
		}
	}
	if raw := get(SettingBlockedExtensions); len(raw) > 0 {
		var list []string
		if json.Unmarshal(raw, &list) == nil {
			l.Blocked = list
		}
	}
	if raw := get(SettingTemporaryTTL); len(raw) > 0 {
		var str string
		var n float64
		switch {
		case json.Unmarshal(raw, &str) == nil:
			if d, err := time.ParseDuration(str); err == nil && d > 0 {
				l.TemporaryTTL = d
			}
		case json.Unmarshal(raw, &n) == nil && n > 0:
			l.TemporaryTTL = time.Duration(n) * time.Second
		}
	}
	return l
}

// storageKey spreads files over 65,536 directories by the random end of the id.
func storageKey(id uuid.UUID) string {
	s := id.String()
	return s[34:36] + "/" + s[32:34] + "/" + s
}

// cleanName keeps the last path element of a name, without control characters.
func cleanName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" {
		return ""
	}
	if r := []rune(name); len(r) > 255 {
		name = string(r[:255])
	}
	return name
}

func invalid(code, msg string) error {
	return &records.ValidationError{Problems: []records.Problem{{Field: "name", Code: code, Message: msg}}}
}

// limitReader fails with ErrTooLarge once more than max bytes came through.
type limitReader struct {
	r    io.Reader
	left int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.left < 0 {
		return 0, ErrTooLarge
	}
	if int64(len(p)) > l.left+1 {
		p = p[:l.left+1]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		return n, ErrTooLarge
	}
	return n, err
}

// Upload stores a new temporary file. The content type comes from the first bytes, not
// from the caller; a broken stream leaves nothing behind.
func (s *Service) Upload(ctx context.Context, name string, body io.Reader, actor records.Actor) (File, error) {
	name = cleanName(name)
	if name == "" {
		return File{}, invalid("required", "the file needs a name (query parameter name or Content-Disposition)")
	}
	lim := s.limits(ctx)
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	for _, b := range lim.Blocked {
		if ext != "" && strings.EqualFold(strings.TrimPrefix(b, "."), ext) {
			return File{}, invalid("blocked", fmt.Sprintf("files ending in .%s are not accepted (setting %s)", ext, SettingBlockedExtensions))
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return File{}, err
	}
	key := storageKey(id)
	if err := s.tq(ctx).InsertFile(ctx, queries.InsertFileParams{ID: id, Name: name, StorageKey: key, CreatedBy: nullUUID(actor)}); err != nil {
		return File{}, fmt.Errorf("files: insert: %w", err)
	}
	// From here on every failure removes row and content again (with a fresh context: the
	// request may be gone).
	fail := func(err error) (File, error) {
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.store.Delete(cctx, key)
		_ = s.tq(cctx).DeleteFile(cctx, id)
		return File{}, err
	}
	br := bufio.NewReaderSize(body, 4096)
	head, _ := br.Peek(512)
	ctype := http.DetectContentType(head)
	if strings.HasPrefix(ctype, "application/octet-stream") {
		if byExt := mime.TypeByExtension("." + ext); byExt != "" {
			ctype = byExt
		}
	}
	h := sha256.New()
	n, err := s.store.Put(ctx, key, io.TeeReader(&limitReader{r: br, left: lim.MaxSize}, h))
	if err != nil {
		if errors.Is(err, ErrTooLarge) {
			return fail(fmt.Errorf("%w: at most %d bytes (setting %s)", ErrTooLarge, lim.MaxSize, SettingMaxSize))
		}
		return fail(fmt.Errorf("files: upload: %w", err))
	}
	if s.Scanner != nil {
		f, err := s.store.Open(ctx, key)
		if err != nil {
			return fail(err)
		}
		err = s.Scanner.Scan(ctx, name, f)
		_ = f.Close()
		if err != nil {
			return fail(invalid("rejected", "the scanner rejected the file: "+err.Error()))
		}
	}
	row, err := s.tq(ctx).FinishFile(ctx, queries.FinishFileParams{
		ID: id, Size: pgtype.Int8{Int64: n, Valid: true},
		Sha256:      pgtype.Text{String: hex.EncodeToString(h.Sum(nil)), Valid: true},
		ContentType: pgtype.Text{String: ctype, Valid: true},
	})
	if err != nil {
		return fail(fmt.Errorf("files: finish: %w", err))
	}
	return fromRow(row), nil
}

func fromRow(r queries.File) File {
	f := File{ID: r.ID, Name: r.Name, ContentType: r.ContentType.String, Size: r.Size.Int64, SHA256: r.Sha256.String,
		State: r.State, CreatedAt: r.CreatedAt.Time}
	if r.CreatedBy.Valid {
		u := r.CreatedBy.UUID
		f.CreatedBy = &u
	}
	return f
}

func nullUUID(a records.Actor) uuid.NullUUID {
	if a == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *a, Valid: true}
}

// get reads a finished file the session may read: its uploader while it is temporary, else
// whoever may read one of the records it is attached to. Anything else is not found.
func (s *Service) get(ctx context.Context, id uuid.UUID) (queries.File, error) {
	notFound := fmt.Errorf("file %s: %w", id, records.ErrNotFound)
	row, err := s.tq(ctx).GetFile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && row.State == "uploading" {
		return queries.File{}, notFound
	}
	if err != nil {
		return queries.File{}, fmt.Errorf("files: get: %w", err)
	}
	a := authz.From(ctx)
	if a.IsSystem() {
		return row, nil
	}
	if row.CreatedBy.Valid && row.CreatedBy.UUID == a.AccountID && row.State == "temporary" {
		return row, nil
	}
	links, err := s.tq(ctx).AttachmentsOfFile(ctx, id)
	if err != nil {
		return queries.File{}, fmt.Errorf("files: links: %w", err)
	}
	for _, l := range links {
		if l.EntryID.Valid {
			// The image of an entry is as visible as the entry (UI7): a private one needs
			// read_private on the record.
			if _, err := s.rec.GetEntry(ctx, l.TypeName, l.RecordID, l.EntryID.UUID); err == nil {
				return row, nil
			}
			continue
		}
		if _, err := s.rec.Get(ctx, l.TypeName, l.RecordID); err == nil {
			return row, nil
		}
	}
	return queries.File{}, notFound
}

// Get returns the metadata of a file the session may read.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (File, error) {
	row, err := s.get(ctx, id)
	if err != nil {
		return File{}, err
	}
	return fromRow(row), nil
}

// Open returns the metadata and the content of a file the session may read.
func (s *Service) Open(ctx context.Context, id uuid.UUID) (File, io.ReadSeekCloser, error) {
	row, err := s.get(ctx, id)
	if err != nil {
		return File{}, nil, err
	}
	r, err := s.store.Open(ctx, row.StorageKey)
	if err != nil {
		return File{}, nil, err
	}
	return fromRow(row), r, nil
}

// documents checks that the type carries the trait documents.
func (s *Service) documents(ctx context.Context, typeName string) error {
	t, err := s.rec.Type(ctx, typeName)
	if err != nil {
		return err
	}
	if !t.HasTrait(meta.TraitDocuments) {
		return fmt.Errorf("%w: %s", ErrNoTrait, typeName)
	}
	return nil
}

// List returns the files attached to a record, as visible as the record.
func (s *Service) List(ctx context.Context, typeName string, recordID uuid.UUID) ([]File, error) {
	if err := s.documents(ctx, typeName); err != nil {
		return nil, err
	}
	rec, err := s.rec.Get(ctx, typeName, recordID)
	if err != nil {
		return nil, err
	}
	rows, err := s.tq(ctx).FilesOfRecord(ctx, queries.FilesOfRecordParams{TypeName: typeName, RecordID: recordID})
	if err != nil {
		return nil, fmt.Errorf("files: list: %w", err)
	}
	private := s.rec.ReadsPrivate(ctx, typeName, rec)
	out := make([]File, 0, len(rows))
	for _, r := range rows {
		if r.EntryPrivate && !private {
			// An image of a private entry (UI7).
			continue
		}
		f := fromRow(queries.File{ID: r.ID, Name: r.Name, ContentType: r.ContentType, Size: r.Size, Sha256: r.Sha256,
			State: r.State, CreatedAt: r.CreatedAt, CreatedBy: r.CreatedBy})
		at := r.AttachedAt.Time
		f.AttachedAt = &at
		if r.AttachedBy.Valid {
			u := r.AttachedBy.UUID
			f.AttachedBy = &u
		}
		out = append(out, f)
	}
	return out, nil
}

// Attach links a file the session may read to a record it may update; the record's history
// gets an attach row. Attaching twice changes nothing.
func (s *Service) Attach(ctx context.Context, typeName string, recordID, fileID uuid.UUID, actor records.Actor) (File, error) {
	if err := s.documents(ctx, typeName); err != nil {
		return File{}, err
	}
	if _, err := s.rec.Check(ctx, typeName, recordID, authz.ActionUpdate); err != nil {
		return File{}, err
	}
	row, err := s.get(ctx, fileID)
	if err != nil {
		return File{}, err
	}
	aid, err := uuid.NewV7()
	if err != nil {
		return File{}, err
	}
	n, err := s.tq(ctx).InsertAttachment(ctx, queries.InsertAttachmentParams{ID: aid, FileID: fileID, TypeName: typeName, RecordID: recordID, CreatedBy: nullUUID(actor)})
	if err != nil {
		return File{}, fmt.Errorf("files: attach: %w", err)
	}
	if n == 1 {
		if err := s.tq(ctx).MarkFileStored(ctx, fileID); err != nil {
			return File{}, fmt.Errorf("files: attach: %w", err)
		}
		row.State = "stored"
		if err := s.rec.Trace(ctx, typeName, recordID, "attach", map[string]any{"file": fileID.String(), "name": row.Name}, actor); err != nil {
			return File{}, err
		}
	}
	return fromRow(row), nil
}

// AttachEntry links an image of an entry to the record (UI7). The caller wrote the entry,
// which needed comment on the record, not update; the file must be one the caller may read
// (an upload of their own). The link names the entry, so that the image of a private entry
// is as private as the entry; the history of the record names only images of public
// entries, so that a private one does not show through it.
func (s *Service) AttachEntry(ctx context.Context, typeName string, recordID uuid.UUID, entry records.Entry, fileID uuid.UUID, actor records.Actor) (File, error) {
	entryID := entry.ID
	if err := s.documents(ctx, typeName); err != nil {
		return File{}, err
	}
	row, err := s.get(ctx, fileID)
	if err != nil {
		return File{}, err
	}
	aid, err := uuid.NewV7()
	if err != nil {
		return File{}, err
	}
	n, err := s.tq(ctx).InsertAttachment(ctx, queries.InsertAttachmentParams{ID: aid, FileID: fileID, TypeName: typeName, RecordID: recordID,
		CreatedBy: nullUUID(actor), EntryID: uuid.NullUUID{UUID: entryID, Valid: true}})
	if err != nil {
		return File{}, fmt.Errorf("files: attach to entry: %w", err)
	}
	if n == 1 {
		if err := s.tq(ctx).MarkFileStored(ctx, fileID); err != nil {
			return File{}, fmt.Errorf("files: attach to entry: %w", err)
		}
		row.State = "stored"
		if entry.Private {
			return fromRow(row), nil
		}
		if err := s.rec.Trace(ctx, typeName, recordID, "attach", map[string]any{"file": fileID.String(), "name": row.Name, "entry": entryID.String()}, actor); err != nil {
			return File{}, err
		}
	}
	return fromRow(row), nil
}

// Readable says whether the session may read the file (UI7: images of a new entry).
func (s *Service) Readable(ctx context.Context, id uuid.UUID) bool {
	_, err := s.get(ctx, id)
	return err == nil
}

// Detach removes the link between a file and a record; the file stays (orphans are PL36).
func (s *Service) Detach(ctx context.Context, typeName string, recordID, fileID uuid.UUID, actor records.Actor) error {
	if err := s.documents(ctx, typeName); err != nil {
		return err
	}
	if _, err := s.rec.Check(ctx, typeName, recordID, authz.ActionUpdate); err != nil {
		return err
	}
	row, err := s.tq(ctx).GetFile(ctx, fileID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("files: detach: %w", err)
	}
	n, err := s.tq(ctx).DeleteAttachment(ctx, queries.DeleteAttachmentParams{FileID: fileID, TypeName: typeName, RecordID: recordID})
	if err != nil {
		return fmt.Errorf("files: detach: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("file %s is not attached to %s %s: %w", fileID, typeName, recordID, records.ErrNotFound)
	}
	return s.rec.Trace(ctx, typeName, recordID, "detach", map[string]any{"file": fileID.String(), "name": row.Name}, actor)
}

// Cleanup removes uploads older than an hour that never finished and temporary files older
// than files.temporary_ttl that nobody attached, content first, then the row.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	lim := s.limits(ctx)
	now := time.Now()
	removed := 0
	for {
		rows, err := s.tq(ctx).StaleFiles(ctx, queries.StaleFilesParams{
			UploadingBefore: pgtype.Timestamptz{Time: now.Add(-uploadGrace), Valid: true},
			TemporaryBefore: pgtype.Timestamptz{Time: now.Add(-lim.TemporaryTTL), Valid: true},
		})
		if err != nil {
			return removed, fmt.Errorf("files: cleanup: %w", err)
		}
		for _, r := range rows {
			if err := s.store.Delete(ctx, r.StorageKey); err != nil {
				return removed, err
			}
			if err := s.tq(ctx).DeleteFile(ctx, r.ID); err != nil {
				return removed, fmt.Errorf("files: cleanup: %w", err)
			}
			removed++
		}
		if len(rows) < 1000 {
			return removed, nil
		}
	}
}

// ContentDisposition is the header value that offers a download under the file's name.
func ContentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return "attachment; filename=" + strconv.Quote(ascii) + "; filename*=UTF-8''" + pathEscape(name)
}

// pathEscape is RFC 5987 encoding: unreserved characters stay, the rest as %XX of UTF-8.
func pathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("!#$&+-.^_`|~", c) >= 0 {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}
