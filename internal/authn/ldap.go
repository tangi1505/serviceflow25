// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package authn

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/records"
)

// --- LDAP directories (PL12, G184, E89) ---
//
// A directory is read, never written: persons and groups become core.person, core.group and
// core.group_member records at the directory's node. The stable id of an entry (entryUUID,
// objectGUID) keeps a renamed entry the same record; an entry that is gone deactivates its
// record and blocks linked accounts. The sync runs as the job ldap.sync (hourly), through
// `itsm ldap sync` and through POST /api/v1/directories/{name}:sync.

// Attributes maps the fields of persons and groups to LDAP attributes; empty means the
// default of DefaultAttributes (OpenLDAP, lldap; Active Directory needs objectGUID,
// sAMAccountName and member).
type Attributes struct {
	ID          string `json:"id,omitempty"`
	Username    string `json:"username,omitempty"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	Mobile      string `json:"mobile,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
	GroupMember string `json:"group_member,omitempty"`
}

// DefaultAttributes are the attributes of OpenLDAP and lldap.
var DefaultAttributes = Attributes{
	ID: "entryUUID", Username: "uid", FirstName: "givenName", LastName: "sn", Email: "mail",
	Phone: "telephoneNumber", Mobile: "mobile", GroupName: "cn", GroupMember: "member",
}

func (a Attributes) withDefaults() Attributes {
	d := DefaultAttributes
	pick := func(v, def string) string {
		if v != "" {
			return v
		}
		return def
	}
	return Attributes{
		ID: pick(a.ID, d.ID), Username: pick(a.Username, d.Username), FirstName: pick(a.FirstName, d.FirstName),
		LastName: pick(a.LastName, d.LastName), Email: pick(a.Email, d.Email), Phone: pick(a.Phone, d.Phone),
		Mobile: pick(a.Mobile, d.Mobile), GroupName: pick(a.GroupName, d.GroupName), GroupMember: pick(a.GroupMember, d.GroupMember),
	}
}

// Directory is an LDAP directory as the API shows it (never the password).
type Directory struct {
	Name         string          `json:"name"`
	URL          string          `json:"url"`
	BindDN       string          `json:"bind_dn"`
	BaseDN       string          `json:"base_dn"`
	PersonFilter string          `json:"person_filter"`
	GroupFilter  string          `json:"group_filter"`
	Attributes   Attributes      `json:"attributes"`
	OrgPath      string          `json:"org_path"`
	StartTLS     bool            `json:"start_tls"`
	SkipVerify   bool            `json:"skip_verify"`
	PageSize     int32           `json:"page_size"`
	Enabled      bool            `json:"enabled"`
	LastSyncAt   *time.Time      `json:"last_sync_at,omitempty"`
	LastResult   json.RawMessage `json:"last_result,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	UpdatedBy    *uuid.UUID      `json:"updated_by,omitempty"`
}

// DirectoryInput is a directory as the API takes it; nil pointers keep the defaults.
type DirectoryInput struct {
	URL          string
	BindDN       string
	BaseDN       string
	PersonFilter *string
	GroupFilter  *string
	Attributes   *Attributes
	OrgPath      *string
	StartTLS     *bool
	SkipVerify   *bool
	PageSize     *int32
	Enabled      *bool
}

// SyncResult is what one run did.
type SyncResult struct {
	Directory string    `json:"directory"`
	StartedAt time.Time `json:"started_at"`
	Duration  string    `json:"duration"`
	Persons   Counts    `json:"persons"`
	Groups    Counts    `json:"groups"`
	Members   struct {
		Added   int `json:"added"`
		Removed int `json:"removed"`
	} `json:"members"`
	Warnings []string `json:"warnings,omitempty"`
}

// Counts of one kind of entry.
type Counts struct {
	Seen        int `json:"seen"`
	Created     int `json:"created"`
	Updated     int `json:"updated"`
	Deactivated int `json:"deactivated"`
	Reactivated int `json:"reactivated"`
}

func directoryOf(r queries.LdapDirectory) Directory {
	d := Directory{Name: r.Name, URL: r.Url, BindDN: r.BindDn, BaseDN: r.BaseDn, PersonFilter: r.PersonFilter, GroupFilter: r.GroupFilter,
		OrgPath: r.OrgPath, StartTLS: r.StartTls, SkipVerify: r.SkipVerify, PageSize: r.PageSize, Enabled: r.Enabled,
		CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	_ = json.Unmarshal(r.Attributes, &d.Attributes)
	if r.LastSyncAt.Valid {
		at := r.LastSyncAt.Time
		d.LastSyncAt = &at
	}
	if len(r.LastResult) > 0 {
		d.LastResult = json.RawMessage(r.LastResult)
	}
	if r.UpdatedBy.Valid {
		by := r.UpdatedBy.UUID
		d.UpdatedBy = &by
	}
	return d
}

// Directories lists the directories.
func (s *Service) Directories(ctx context.Context) ([]Directory, error) {
	rows, err := queries.New(s.pool).ListDirectories(ctx)
	if err != nil {
		return nil, fmt.Errorf("authn: directories: %w", err)
	}
	out := make([]Directory, 0, len(rows))
	for _, r := range rows {
		out = append(out, directoryOf(queries.LdapDirectory(r)))
	}
	return out, nil
}

// Directory returns one directory.
func (s *Service) Directory(ctx context.Context, name string) (Directory, error) {
	r, err := queries.New(s.pool).GetDirectory(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return Directory{}, fmt.Errorf("%w: directory %s", ErrNotFound, name)
	}
	if err != nil {
		return Directory{}, fmt.Errorf("authn: directory: %w", err)
	}
	return directoryOf(queries.LdapDirectory(r)), nil
}

func (s *Service) checkDirectory(ctx context.Context, in DirectoryInput, current *Directory) (queries.InsertDirectoryParams, error) {
	var p queries.InsertDirectoryParams
	if in.URL == "" || in.BindDN == "" || in.BaseDN == "" {
		return p, fmt.Errorf("%w: a directory needs url, bind_dn and base_dn", ErrInvalid)
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") || u.Host == "" {
		return p, fmt.Errorf("%w: url must be ldap://host[:port] or ldaps://host[:port]", ErrInvalid)
	}
	d := Directory{PersonFilter: "(objectClass=person)", GroupFilter: "(objectClass=groupOfUniqueNames)", OrgPath: "root", PageSize: 500, Enabled: true}
	if current != nil {
		d = *current
	}
	if in.PersonFilter != nil {
		d.PersonFilter = *in.PersonFilter
	}
	if in.GroupFilter != nil {
		d.GroupFilter = *in.GroupFilter
	}
	if in.Attributes != nil {
		d.Attributes = *in.Attributes
	}
	if in.OrgPath != nil {
		d.OrgPath = *in.OrgPath
	}
	if in.StartTLS != nil {
		d.StartTLS = *in.StartTLS
	}
	if in.SkipVerify != nil {
		d.SkipVerify = *in.SkipVerify
	}
	if in.PageSize != nil {
		d.PageSize = *in.PageSize
	}
	if in.Enabled != nil {
		d.Enabled = *in.Enabled
	}
	for _, f := range []string{d.PersonFilter, d.GroupFilter} {
		if _, err := ldap.CompileFilter(f); err != nil {
			return p, fmt.Errorf("%w: filter %q: %w", ErrInvalid, f, err)
		}
	}
	if d.PageSize < 0 || d.PageSize > 10000 {
		return p, fmt.Errorf("%w: page_size must be between 0 and 10000", ErrInvalid)
	}
	if _, err := s.org.Node(ctx, d.OrgPath); err != nil {
		return p, fmt.Errorf("%w: no node %q", ErrInvalid, d.OrgPath)
	}
	attrs, _ := json.Marshal(d.Attributes)
	return queries.InsertDirectoryParams{
		Url: in.URL, BindDn: in.BindDN, BaseDn: in.BaseDN, PersonFilter: d.PersonFilter, GroupFilter: d.GroupFilter,
		Attributes: attrs, OrgPath: d.OrgPath, StartTls: d.StartTLS, SkipVerify: d.SkipVerify, PageSize: d.PageSize, Enabled: d.Enabled,
	}, nil
}

// CreateDirectory stores a directory.
func (s *Service) CreateDirectory(ctx context.Context, name string, in DirectoryInput, by *uuid.UUID) (Directory, error) {
	p, err := s.checkDirectory(ctx, in, nil)
	if err != nil {
		return Directory{}, err
	}
	p.Name, p.UpdatedBy = name, nullUUID(by)
	if err := queries.New(s.pool).InsertDirectory(ctx, p); err != nil {
		return Directory{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return s.Directory(ctx, name)
}

// UpdateDirectory replaces the settings of a directory; what the input leaves nil stays.
func (s *Service) UpdateDirectory(ctx context.Context, name string, in DirectoryInput, by *uuid.UUID) (Directory, error) {
	current, err := s.Directory(ctx, name)
	if err != nil {
		return Directory{}, err
	}
	p, err := s.checkDirectory(ctx, in, &current)
	if err != nil {
		return Directory{}, err
	}
	n, err := queries.New(s.pool).UpdateDirectory(ctx, queries.UpdateDirectoryParams{
		Name: name, Url: p.Url, BindDn: p.BindDn, BaseDn: p.BaseDn, PersonFilter: p.PersonFilter, GroupFilter: p.GroupFilter,
		Attributes: p.Attributes, OrgPath: p.OrgPath, StartTls: p.StartTls, SkipVerify: p.SkipVerify, PageSize: p.PageSize,
		Enabled: p.Enabled, UpdatedBy: nullUUID(by),
	})
	if err != nil {
		return Directory{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if n == 0 {
		return Directory{}, fmt.Errorf("%w: directory %s", ErrNotFound, name)
	}
	return s.Directory(ctx, name)
}

// DeleteDirectory removes a directory and its entry mapping; persons and groups stay.
func (s *Service) DeleteDirectory(ctx context.Context, name string) error {
	n, err := queries.New(s.pool).DeleteDirectory(ctx, name)
	if err != nil {
		return fmt.Errorf("authn: delete directory: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: directory %s", ErrNotFound, name)
	}
	return nil
}

// PasswordEnv is the environment variable that holds the bind password of a directory:
// ITSM_LDAP_PASSWORD_<NAME>, the name upper-cased with - as _ (E89).
func PasswordEnv(name string) string {
	return "ITSM_LDAP_PASSWORD_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// SyncAll runs every enabled directory and returns the results; a directory that fails is
// reported, the others still run.
func (s *Service) SyncAll(ctx context.Context) ([]SyncResult, error) {
	list, err := s.Directories(ctx)
	if err != nil {
		return nil, err
	}
	var out []SyncResult
	var errs []error
	for _, d := range list {
		if !d.Enabled {
			continue
		}
		res, err := s.Sync(ctx, d.Name)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Name, err))
			continue
		}
		out = append(out, res)
	}
	return out, errors.Join(errs...)
}

// Sync reads one directory and brings persons, groups and memberships in line with it.
func (s *Service) Sync(ctx context.Context, name string) (SyncResult, error) {
	d, err := s.Directory(ctx, name)
	if err != nil {
		return SyncResult{}, err
	}
	res := SyncResult{Directory: name, StartedAt: time.Now()}
	password, ok := os.LookupEnv(PasswordEnv(name))
	if !ok {
		return res, fmt.Errorf("%w: %s is not set", ErrInvalid, PasswordEnv(name))
	}
	conn, err := s.dial(d)
	if err != nil {
		return res, err
	}
	defer conn.Close()
	if err := conn.Bind(d.BindDN, password); err != nil {
		return res, fmt.Errorf("authn: bind as %s: %w", d.BindDN, err)
	}
	attrs := d.Attributes.withDefaults()
	q := queries.New(s.pool)

	// Persons.
	entries, err := s.search(conn, d, d.PersonFilter, []string{attrs.ID, attrs.Username, attrs.FirstName, attrs.LastName, attrs.Email, attrs.Phone, attrs.Mobile})
	if err != nil {
		return res, err
	}
	known, err := s.knownEntries(ctx, name, "person")
	if err != nil {
		return res, err
	}
	personByDN := map[string]uuid.UUID{}
	seenPersons := map[string]bool{}
	for _, e := range entries {
		id := externalID(e, attrs.ID)
		if id == "" {
			res.Warnings = append(res.Warnings, "person without "+attrs.ID+": "+e.DN)
			continue
		}
		seenPersons[id] = true
		res.Persons.Seen++
		values := map[string]any{"active": true}
		last := e.GetAttributeValue(attrs.LastName)
		if last == "" {
			last = e.GetAttributeValue(attrs.Username)
		}
		values["last_name"] = last
		for field, attr := range map[string]string{"first_name": attrs.FirstName, "email": attrs.Email, "phone": attrs.Phone, "mobile": attrs.Mobile} {
			if v := e.GetAttributeValue(attr); v != "" {
				values[field] = v
			} else {
				values[field] = nil
			}
		}
		rid, err := s.upsertRecord(ctx, q, name, "person", "core.person", id, e.DN, values, d.OrgPath, known[id], &res.Persons)
		if err != nil {
			return res, err
		}
		personByDN[strings.ToLower(e.DN)] = rid
	}
	if err := s.deactivateMissing(ctx, q, name, "person", "core.person", known, seenPersons, &res.Persons, true); err != nil {
		return res, err
	}

	// Groups and their members.
	groups, err := s.search(conn, d, d.GroupFilter, []string{attrs.ID, attrs.GroupName, attrs.GroupMember})
	if err != nil {
		return res, err
	}
	knownGroups, err := s.knownEntries(ctx, name, "group")
	if err != nil {
		return res, err
	}
	seenGroups := map[string]bool{}
	for _, e := range groups {
		id := externalID(e, attrs.ID)
		if id == "" {
			res.Warnings = append(res.Warnings, "group without "+attrs.ID+": "+e.DN)
			continue
		}
		seenGroups[id] = true
		res.Groups.Seen++
		gname := e.GetAttributeValue(attrs.GroupName)
		if gname == "" {
			gname = e.DN
		}
		gid, err := s.upsertRecord(ctx, q, name, "group", "core.group", id, e.DN, map[string]any{"name": gname, "active": true}, d.OrgPath, knownGroups[id], &res.Groups)
		if err != nil {
			return res, err
		}
		want := map[uuid.UUID]bool{}
		for _, dn := range e.GetAttributeValues(attrs.GroupMember) {
			if pid, ok := personByDN[strings.ToLower(dn)]; ok {
				want[pid] = true
			}
		}
		added, removed, err := s.reconcileMembers(ctx, gid, want)
		if err != nil {
			return res, err
		}
		res.Members.Added += added
		res.Members.Removed += removed
	}
	if err := s.deactivateMissing(ctx, q, name, "group", "core.group", knownGroups, seenGroups, &res.Groups, false); err != nil {
		return res, err
	}

	res.Duration = time.Since(res.StartedAt).Round(time.Millisecond).String()
	raw, _ := json.Marshal(res)
	if err := q.SetDirectoryResult(ctx, queries.SetDirectoryResultParams{Name: name, LastResult: raw}); err != nil {
		return res, fmt.Errorf("authn: store sync result: %w", err)
	}
	s.az.Invalidate(ctx)
	s.logger.Info("directory synced", "directory", name, "persons", res.Persons, "groups", res.Groups, "members", res.Members, "duration", res.Duration)
	return res, nil
}

func (s *Service) dial(d Directory) (*ldap.Conn, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: d.SkipVerify} //nolint:gosec // G402: skip_verify is an explicit setting for test directories
	conn, err := ldap.DialURL(d.URL, ldap.DialWithTLSConfig(tlsConfig))
	if err != nil {
		return nil, fmt.Errorf("authn: connect %s: %w", d.URL, err)
	}
	if d.StartTLS {
		if err := conn.StartTLS(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("authn: starttls %s: %w", d.URL, err)
		}
	}
	return conn, nil
}

func (s *Service) search(conn *ldap.Conn, d Directory, filter string, attrs []string) ([]*ldap.Entry, error) {
	req := ldap.NewSearchRequest(d.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, filter, attrs, nil)
	var res *ldap.SearchResult
	var err error
	if d.PageSize > 0 {
		res, err = conn.SearchWithPaging(req, uint32(d.PageSize)) //nolint:gosec // the schema bounds page_size
	} else {
		res, err = conn.Search(req)
	}
	if err != nil {
		return nil, fmt.Errorf("authn: search %s in %s: %w", filter, d.BaseDN, err)
	}
	return res.Entries, nil
}

// externalID reads the stable id; a binary objectGUID becomes its UUID text.
func externalID(e *ldap.Entry, attr string) string {
	raw := e.GetRawAttributeValue(attr)
	if len(raw) == 16 && !isPrintable(raw) {
		if u, err := uuid.FromBytes(raw); err == nil {
			return u.String()
		}
	}
	return strings.TrimSpace(string(raw))
}

func isPrintable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

type knownEntry struct {
	recordID uuid.UUID
	active   bool
}

func (s *Service) knownEntries(ctx context.Context, directory, kind string) (map[string]*knownEntry, error) {
	rows, err := queries.New(s.pool).ListDirectoryEntries(ctx, queries.ListDirectoryEntriesParams{Directory: directory, Kind: kind})
	if err != nil {
		return nil, fmt.Errorf("authn: directory entries: %w", err)
	}
	out := make(map[string]*knownEntry, len(rows))
	for _, r := range rows {
		out[r.ExternalID] = &knownEntry{recordID: r.RecordID, active: r.Active}
	}
	return out, nil
}

// upsertRecord creates or updates the record of an entry and notes the entry.
func (s *Service) upsertRecord(ctx context.Context, q *queries.Queries, directory, kind, typeName, externalID, dn string, values map[string]any, orgPath string, known *knownEntry, c *Counts) (uuid.UUID, error) {
	var rid uuid.UUID
	if known != nil {
		rid = known.recordID
		current, err := s.rec.Get(ctx, typeName, rid)
		if err != nil && !errors.Is(err, records.ErrNotFound) {
			return uuid.Nil, err
		}
		if err == nil {
			updated, err := s.rec.Update(ctx, typeName, rid, current["version"].(int64), values, nil)
			if err != nil {
				return uuid.Nil, fmt.Errorf("authn: update %s %s: %w", typeName, dn, err)
			}
			if updated["version"] != current["version"] {
				c.Updated++
			}
			if !known.active {
				c.Reactivated++
			}
			if err := q.UpsertDirectoryEntry(ctx, queries.UpsertDirectoryEntryParams{Directory: directory, Kind: kind, ExternalID: externalID, RecordID: rid, Dn: dn}); err != nil {
				return uuid.Nil, fmt.Errorf("authn: note entry: %w", err)
			}
			return rid, nil
		}
		// The record was purged by hand: a new one takes its place.
	}
	values["org_path"] = orgPath
	rec, err := s.rec.Create(ctx, typeName, values, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("authn: create %s for %s: %w", typeName, dn, err)
	}
	rid = uuid.MustParse(rec["id"].(string))
	c.Created++
	if err := q.UpsertDirectoryEntry(ctx, queries.UpsertDirectoryEntryParams{Directory: directory, Kind: kind, ExternalID: externalID, RecordID: rid, Dn: dn}); err != nil {
		return uuid.Nil, fmt.Errorf("authn: note entry: %w", err)
	}
	return rid, nil
}

// deactivateMissing sets active=false on records whose entries are gone (and blocks the
// accounts of such persons); the mapping stays, so a returning entry is the same record.
func (s *Service) deactivateMissing(ctx context.Context, q *queries.Queries, directory, kind, typeName string, known map[string]*knownEntry, seen map[string]bool, c *Counts, blockAccounts bool) error {
	for externalID, k := range known {
		if seen[externalID] || !k.active {
			continue
		}
		current, err := s.rec.Get(ctx, typeName, k.recordID)
		if err == nil {
			if _, err := s.rec.Update(ctx, typeName, k.recordID, current["version"].(int64), map[string]any{"active": false}, nil); err != nil {
				return fmt.Errorf("authn: deactivate %s %s: %w", typeName, k.recordID, err)
			}
		} else if !errors.Is(err, records.ErrNotFound) {
			return err
		}
		if blockAccounts {
			accounts, err := q.AccountsOfPerson(ctx, uuid.NullUUID{UUID: k.recordID, Valid: true})
			if err != nil {
				return fmt.Errorf("authn: accounts of person: %w", err)
			}
			for _, a := range accounts {
				if err := s.auth.SetDisabled(ctx, a.Username, true); err != nil {
					return err
				}
			}
		}
		if err := q.DeactivateDirectoryEntry(ctx, queries.DeactivateDirectoryEntryParams{Directory: directory, Kind: kind, ExternalID: externalID}); err != nil {
			return fmt.Errorf("authn: deactivate entry: %w", err)
		}
		c.Deactivated++
	}
	return nil
}

// reconcileMembers makes the members of a group exactly the wanted persons.
func (s *Service) reconcileMembers(ctx context.Context, group uuid.UUID, want map[uuid.UUID]bool) (added, removed int, err error) {
	have := map[uuid.UUID]uuid.UUID{} // person → membership record
	after := ""
	for {
		page, err := s.rec.List(ctx, "core.group_member", records.ListOptions{Limit: 500, After: after, Filter: map[string]string{"group": group.String()}})
		if err != nil {
			return 0, 0, fmt.Errorf("authn: members: %w", err)
		}
		for _, m := range page.Items {
			pid, perr := uuid.Parse(fmt.Sprint(m["person"]))
			mid, merr := uuid.Parse(fmt.Sprint(m["id"]))
			if perr == nil && merr == nil {
				have[pid] = mid
			}
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	for pid, mid := range have {
		if want[pid] {
			continue
		}
		if err := s.rec.Trash(ctx, "core.group_member", mid, nil); err != nil {
			return added, removed, fmt.Errorf("authn: remove member: %w", err)
		}
		if err := s.rec.Purge(ctx, "core.group_member", mid, nil); err != nil {
			return added, removed, fmt.Errorf("authn: remove member: %w", err)
		}
		removed++
	}
	for pid := range want {
		if _, ok := have[pid]; ok {
			continue
		}
		if _, err := s.rec.Create(ctx, "core.group_member", map[string]any{"group": group.String(), "person": pid.String()}, nil); err != nil {
			return added, removed, fmt.Errorf("authn: add member: %w", err)
		}
		added++
	}
	return added, removed, nil
}

// personByEmail finds a person with the email, for linking an identity to a synced person
// (E89); nil when there is none or more than one.
func (s *Service) personByEmail(ctx context.Context, email string) *uuid.UUID {
	if email == "" {
		return nil
	}
	page, err := s.rec.List(ctx, "core.person", records.ListOptions{Limit: 2, Filter: map[string]string{"email": email}})
	if err != nil || len(page.Items) != 1 {
		return nil
	}
	id, err := uuid.Parse(fmt.Sprint(page.Items[0]["id"]))
	if err != nil {
		return nil
	}
	return &id
}
