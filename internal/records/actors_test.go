// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package records

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/schema"
)

const actorType = "actorstest.ticket"

// setupActors installs the package actorstest: ticket with the roles requester (one person),
// assigned (persons or teams) and vendor (suppliers, the way a supplier type of a later package
// joins), other with a role watcher of its own, and the holder types person, team, supplier.
func setupActors(t *testing.T) (*Service, context.Context, *pgxpool.Pool) {
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
		"DROP TABLE IF EXISTS rec_actorstest_ticket", "DROP TABLE IF EXISTS rec_actorstest_other",
		"DROP TABLE IF EXISTS rec_actorstest_person", "DROP TABLE IF EXISTS rec_actorstest_team",
		"DROP TABLE IF EXISTS rec_actorstest_supplier",
		"DELETE FROM package WHERE name = 'actorstest'",
		"DELETE FROM audit WHERE type_name LIKE 'actorstest.%'",
		"DELETE FROM event WHERE type_name LIKE 'actorstest.%'",
		"DELETE FROM actor WHERE type_name LIKE 'actorstest.%'",
		"DELETE FROM schema_change WHERE type_name LIKE 'actorstest.%'",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	holder := func(name string) []byte {
		return []byte("name: " + name + "\nlabel: {de: H, en: H}\ntitle_field: name\nfields:\n  - {name: name, type: text, label: {de: N, en: N}}\n")
	}
	p, err := meta.LoadFS(fstest.MapFS{
		"package.yaml":        {Data: []byte("name: actorstest\nversion: 0.1.0\nlabel: {de: R, en: R}\n")},
		"types/person.yaml":   {Data: holder("person")},
		"types/team.yaml":     {Data: holder("team")},
		"types/supplier.yaml": {Data: holder("supplier")},
		"types/ticket.yaml": {Data: []byte(`name: ticket
label: {de: Vorgang, en: Ticket}
traits: [history, actors]
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
actors:
  - {name: requester, label: {de: Anforderer, en: Requester}, to: [actorstest.person], single: true}
  - {name: assigned, label: {de: Bearbeiter, en: Assigned}, to: [actorstest.person, actorstest.team]}
  - {name: vendor, label: {de: Lieferant, en: Supplier}, to: [actorstest.supplier]}
`)},
		"types/other.yaml": {Data: []byte(`name: other
label: {de: Anderes, en: Other}
traits: [actors]
fields:
  - {name: title, type: text, label: {de: Titel, en: Title}}
actors:
  - {name: watcher, label: {de: Beobachter, en: Watcher}, to: [actorstest.person]}
`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(p, nil); err != nil {
		t.Fatal(err)
	}
	if err := meta.Store(ctx, pool, p); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Apply(ctx, pool, p.Types, quiet); err != nil {
		t.Fatal(err)
	}
	return New(db.Single(pool)), ctx, pool
}

func holderRef(typ string, rec Record) map[string]any {
	return map[string]any{"type": typ, "id": rec["id"]}
}

func roleOf(rec Record, role string) []any {
	roles, _ := rec[actorsField].(map[string]any)
	list, _ := roles[role].([]any)
	return list
}

func problemCodes(err error) map[string]string {
	out := map[string]string{}
	if ve, ok := AsValidationError(err); ok {
		for _, p := range ve.Problems {
			out[p.Field] = p.Code
		}
	}
	return out
}

// The core of WF3: roles are written with the record, belong to its version (history, event,
// versions, restore), are checked against the type, filter lists and queries, and give the
// scope involved.
func TestRolesAtTheRecord(t *testing.T) {
	s, ctx, pool := setupActors(t)
	mk := func(typ, name string) Record {
		t.Helper()
		r, err := s.Create(ctx, "actorstest."+typ, map[string]any{"name": name}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	anna, ben := mk("person", "Anna"), mk("person", "Ben")
	net, desk := mk("team", "Netz"), mk("team", "Desk")
	acme := mk("supplier", "Acme")

	// Create with roles: every role of the type comes back, the holders sorted, repeats gone.
	rec, err := s.Create(ctx, actorType, map[string]any{"title": "Drucker", "actors": map[string]any{
		"requester": []any{holderRef("actorstest.person", anna)},
		"assigned":  []any{holderRef("actorstest.team", net), holderRef("actorstest.person", ben), holderRef("actorstest.team", net)},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rid := id(t, rec)
	if len(roleOf(rec, "requester")) != 1 || len(roleOf(rec, "assigned")) != 2 || roleOf(rec, "vendor") == nil || len(roleOf(rec, "vendor")) != 0 {
		t.Fatalf("roles of the new record: %v", rec[actorsField])
	}
	if first, _ := roleOf(rec, "assigned")[0].(map[string]any); first["type"] != "actorstest.person" {
		t.Fatalf("holders are sorted by type: %v", roleOf(rec, "assigned"))
	}
	var payload []byte
	if err := pool.QueryRow(ctx, "SELECT payload FROM event WHERE type_name = $1 AND record_id = $2 AND kind = 'create'", actorType, rid).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Changed []string `json:"changed"`
	}
	_ = json.Unmarshal(payload, &ev)
	if !contains(ev.Changed, "actors.requester") || !contains(ev.Changed, "actors.assigned") || contains(ev.Changed, "actors.vendor") {
		t.Fatalf("event of the create names the roles: %v", ev.Changed)
	}

	// Checks: a role of another type, a holder type the role does not take, an unknown id, two
	// holders for a single role, a list that is no list.
	_, err = s.Create(ctx, actorType, map[string]any{"title": "x", "actors": map[string]any{
		"watcher":   []any{holderRef("actorstest.person", anna)},
		"vendor":    []any{holderRef("actorstest.team", net)},
		"assigned":  []any{map[string]any{"type": "actorstest.person", "id": uuid.NewString()}},
		"requester": []any{holderRef("actorstest.person", anna), holderRef("actorstest.person", ben)},
	}}, nil)
	codes := problemCodes(err)
	if codes["actors.watcher"] != "unknown" || codes["actors.vendor"] != "type" || codes["actors.assigned"] != "reference" || codes["actors.requester"] != "single" {
		t.Fatalf("checks of the roles: %v", err)
	}
	if _, err = s.Create(ctx, actorType, map[string]any{"title": "x", "actors": []any{}}, nil); problemCodes(err)["actors"] != "type" {
		t.Fatalf("actors must be an object: %v", err)
	}
	if _, err = s.Create(ctx, "actorstest.person", map[string]any{"name": "x", "actors": map[string]any{}}, nil); problemCodes(err)["actors"] != "unknown" {
		t.Fatalf("a type without the trait has no roles: %v", err)
	}

	// Update: a changed role is a new version with an audit row; the same set again is nothing;
	// null clears; the supplier joins.
	rec, err = s.Update(ctx, actorType, rid, 1, map[string]any{"actors": map[string]any{"assigned": []any{holderRef("actorstest.team", desk)}}}, nil)
	if err != nil || rec["version"] != int64(2) || len(roleOf(rec, "assigned")) != 1 || len(roleOf(rec, "requester")) != 1 {
		t.Fatalf("change assigned: %v %v", rec, err)
	}
	rec, err = s.Update(ctx, actorType, rid, 2, map[string]any{"actors": map[string]any{"assigned": []any{holderRef("actorstest.team", desk)}}}, nil)
	if err != nil || rec["version"] != int64(2) {
		t.Fatalf("the same holders again change nothing: %v %v", rec["version"], err)
	}
	rec, err = s.Update(ctx, actorType, rid, 2, map[string]any{"title": "Drucker 3. OG", "actors": map[string]any{"requester": nil, "vendor": []any{holderRef("actorstest.supplier", acme)}}}, nil)
	if err != nil || rec["version"] != int64(3) || len(roleOf(rec, "requester")) != 0 || len(roleOf(rec, "vendor")) != 1 || len(roleOf(rec, "assigned")) != 1 {
		t.Fatalf("clear requester, add vendor: %v %v", rec, err)
	}
	var oldVal, newVal []byte
	if err := pool.QueryRow(ctx, "SELECT old_value, new_value FROM audit WHERE type_name = $1 AND record_id = $2 AND version = 2 AND field = 'actors.assigned'", actorType, rid).Scan(&oldVal, &newVal); err != nil {
		t.Fatal(err)
	}
	var oldList, newList []map[string]any
	_ = json.Unmarshal(oldVal, &oldList)
	_ = json.Unmarshal(newVal, &newList)
	if len(oldList) != 2 || len(newList) != 1 || newList[0]["id"] != desk["id"] {
		t.Fatalf("audit of version 2: %s → %s", oldVal, newVal)
	}

	// Versions: version 1 shows the first holders; the comparison names the role; restoring
	// version 1 brings them back as version 4.
	v1, err := s.Version(ctx, actorType, rid, 1)
	if err != nil || len(roleOf(v1, "assigned")) != 2 || len(roleOf(v1, "requester")) != 1 || len(roleOf(v1, "vendor")) != 0 {
		t.Fatalf("version 1: %v %v", v1, err)
	}
	cmp, err := s.Compare(ctx, actorType, rid, 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	fields := []string{}
	for _, c := range cmp.Changes {
		fields = append(fields, c.Field)
	}
	if !contains(fields, "actors.assigned") || !contains(fields, "actors.requester") || !contains(fields, "actors.vendor") || !contains(fields, "title") {
		t.Fatalf("comparison 1..3: %v", fields)
	}
	rec, err = s.RestoreVersion(ctx, actorType, rid, 1, 3, nil)
	if err != nil || rec["version"] != int64(4) || len(roleOf(rec, "assigned")) != 2 || len(roleOf(rec, "vendor")) != 0 || rec["title"] != "Drucker" {
		t.Fatalf("restore version 1: %v %v", rec, err)
	}

	// Filters: a second ticket for Desk only; lists and queries by role, any role, null.
	other, err := s.Create(ctx, actorType, map[string]any{"title": "Monitor", "actors": map[string]any{"assigned": []any{holderRef("actorstest.team", desk)}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	titles := func(p Page) []string {
		out := []string{}
		for _, r := range p.Items {
			out = append(out, r["title"].(string))
		}
		return out
	}
	page, err := s.List(ctx, actorType, ListOptions{Filter: map[string]string{"actors.assigned": desk["id"].(string)}})
	if err != nil || len(page.Items) != 1 || page.Items[0]["title"] != "Monitor" {
		t.Fatalf("filter assigned = Desk: %v %v", titles(page), err)
	}
	page, err = s.List(ctx, actorType, ListOptions{Filter: map[string]string{"actors": anna["id"].(string) + "," + desk["id"].(string)}, Sort: "created_at"})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("filter any role = Anna or Desk: %v %v", titles(page), err)
	}
	if _, err = s.List(ctx, actorType, ListOptions{Filter: map[string]string{"actors.watcher": anna["id"].(string)}}); problemCodes(err)["actors.watcher"] != "unknown" {
		t.Fatalf("filter on a role of another type: %v", err)
	}
	page, _, err = s.Query(ctx, actorType, Query{Where: json.RawMessage(`{"and": [{"field": "actors.vendor", "null": true}, {"field": "actors.assigned", "in": ["` + ben["id"].(string) + `"]}]}`)})
	if err != nil || len(page.Items) != 1 || page.Items[0]["title"] != "Drucker" {
		t.Fatalf("query vendor null and assigned Ben: %v %v", titles(page), err)
	}
	page, err = s.List(ctx, actorType, ListOptions{Fields: []string{"title", "actors"}, Filter: map[string]string{"actors.requester": anna["id"].(string)}})
	if err != nil || len(page.Items) != 1 || len(roleOf(page.Items[0], "requester")) != 1 {
		t.Fatalf("list with the field actors: %v %v", page.Items, err)
	}

	// The scope involved and @me / @my_groups: Ben is assigned to the first ticket himself;
	// a member of Desk sees the second through the team.
	benPerson := uuid.MustParse(ben["id"].(string))
	grant := authz.Grant{TypeName: actorType, Action: authz.ActionRead, Scope: authz.ScopeInvolved, OrgPath: "root", Role: "r"}
	asBen := authz.WithAccess(ctx, &authz.Access{AccountID: uuid.New(), PersonID: &benPerson, Grants: []authz.Grant{grant}})
	page, err = s.List(asBen, actorType, ListOptions{})
	if err != nil || len(page.Items) != 1 || page.Items[0]["title"] != "Drucker" {
		t.Fatalf("Ben sees what he holds a role at: %v %v", titles(page), err)
	}
	if _, err := s.Get(asBen, actorType, id(t, other)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Ben does not see the Desk ticket: %v", err)
	}
	deskMember := authz.WithAccess(ctx, &authz.Access{AccountID: uuid.New(), PersonID: &benPerson,
		Groups: []uuid.UUID{uuid.MustParse(desk["id"].(string))}, Grants: []authz.Grant{grant}})
	page, err = s.List(deskMember, actorType, ListOptions{Filter: map[string]string{"actors.assigned": "@my_groups"}})
	if err != nil || len(page.Items) != 1 || page.Items[0]["title"] != "Monitor" {
		t.Fatalf("a member of Desk finds the Desk ticket under my groups: %v %v", titles(page), err)
	}
	page, err = s.List(deskMember, actorType, ListOptions{Filter: map[string]string{"actors.assigned": "@me"}})
	if err != nil || len(page.Items) != 1 || page.Items[0]["title"] != "Drucker" {
		t.Fatalf("@me: %v %v", titles(page), err)
	}
	page, err = s.List(deskMember, actorType, ListOptions{Count: true})
	if err != nil || len(page.Items) != 2 || page.Total == nil || *page.Total != 2 {
		t.Fatalf("involved reaches both, in person and through the team: %v %v", titles(page), err)
	}
	if _, err := s.Get(deskMember, actorType, id(t, other)); err != nil {
		t.Fatalf("the team gives the member the Desk ticket: %v", err)
	}

	// Field rights: without "actors" the roles are hidden and cannot be written.
	fieldsOnly := authz.Grant{TypeName: actorType, Action: authz.Any, Scope: authz.ScopeAll, OrgPath: "root", Role: "f", Fields: &authz.Fields{Read: []string{"title"}, Write: []string{"title"}}}
	limited := authz.WithAccess(ctx, &authz.Access{AccountID: uuid.New(), Grants: []authz.Grant{fieldsOnly}})
	got, err := s.Get(limited, actorType, rid)
	if _, shown := got[actorsField]; err != nil || shown {
		t.Fatalf("roles hidden without the field right: %v %v", got, err)
	}
	if _, err := s.Update(limited, actorType, rid, 4, map[string]any{"actors": map[string]any{"vendor": nil}}, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("roles not writable without the field right: %v", err)
	}

	// Purge takes the rows of the table actor with the record.
	if err := s.Trash(ctx, actorType, rid, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Purge(ctx, actorType, rid, nil); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM actor WHERE type_name = $1 AND record_id = $2", actorType, rid).Scan(&n); err != nil || n != 0 {
		t.Fatalf("actor rows after purge: %d %v", n, err)
	}
}

// A role needs the trait, the trait a role; holders must be types that exist.
func TestRolesInTheDefinition(t *testing.T) {
	load := func(ticket string) error {
		p, err := meta.LoadFS(fstest.MapFS{
			"package.yaml":      {Data: []byte("name: rolecheck\nversion: 0.1.0\nlabel: {de: R, en: R}\n")},
			"types/ticket.yaml": {Data: []byte("name: ticket\nlabel: {de: V, en: T}\n" + ticket + "fields:\n  - {name: title, type: text, label: {de: T, en: T}}\n")},
		})
		if err != nil {
			return err
		}
		return meta.Validate(p, map[string]bool{"core.person": true})
	}
	for _, c := range []struct{ yaml, want string }{
		{"traits: [actors]\n", "needs at least one role"},
		{"actors:\n  - {name: a, label: {de: A, en: A}, to: [core.person]}\n", "needs the trait actors"},
		{"traits: [actors]\nactors:\n  - {name: a, label: {de: A, en: A}, to: [core.nobody]}\n", "neither in this package nor installed"},
		{"traits: [actors]\nactors:\n  - {name: a, label: {de: A, en: A}, to: []}\n", "at least one type"},
		{"traits: [actors]\nactors:\n  - {name: a, label: {de: A, en: A}, to: [core.person]}\n  - {name: a, label: {de: A, en: A}, to: [core.person]}\n", "defined twice"},
	} {
		if err := load(c.yaml); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: want %q, got %v", c.yaml, c.want, err)
		}
	}
	if err := load("traits: [actors]\nactors:\n  - {name: a, label: {de: A, en: A}, to: [core.person, rolecheck.ticket]}\n"); err != nil {
		t.Fatalf("a valid role: %v", err)
	}
}
