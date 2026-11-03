// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/google/uuid"

	"group24.de/itsm/internal/db"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// API coverage of the packages (PL30, Hauptplan E105; Inventar G208, G209, G213, G214, G216,
// G219, G223): for every type of every package in packages/ the test creates a record from
// the type definition, lists and fetches it and validates each response against the effective
// OpenAPI description (/api/v1/openapi.json) with kin-openapi, the schema of the type
// included. Fields the description does not declare fail too: the test closes the object
// schemas of the components. The organisation tree and the settings of the root node are
// checked the same way. Packages that come later, those of the service desk first, are
// covered as soon as they lie in packages/.
func TestAPICoverage(t *testing.T) {
	api := apiServer(t)
	types := installAllPackages(t, api)
	spec := strictSpec(t, api)

	s := &sampler{t: t, api: api, types: types, made: map[string]string{}, busy: map[string]bool{}}
	t.Cleanup(s.cleanup)
	names := sortedKeys(types)
	for _, name := range names {
		id := s.record(name)
		spec.check(t, api, "/api/v1/types/"+name, "/api/v1/types/{typeName}", map[string]string{"typeName": name})
		spec.check(t, api, "/api/v1/records/"+name+"?limit=500", "/api/v1/records/"+name, nil)
		spec.check(t, api, "/api/v1/records/"+name+"/"+id, "/api/v1/records/"+name+"/{id}", map[string]string{"id": id})
		// Translatable fields come as an object of languages with translations=all (PL21).
		spec.check(t, api, "/api/v1/records/"+name+"/"+id+"?translations=all", "/api/v1/records/"+name+"/{id}", map[string]string{"id": id})
	}

	// Organisation units and settings (Inventar G208, G223), with one setting of the test's own.
	if r := call(t, api, http.MethodPut, "/api/v1/org/nodes/root/settings/apitest.coverage", `{"checked": true}`, nil); r.code != 204 {
		t.Fatalf("set a setting: %d %s", r.code, r.raw)
	}
	t.Cleanup(func() {
		_, _ = api.pool.Exec(context.Background(), "DELETE FROM org_setting WHERE path = 'root' AND key = 'apitest.coverage'")
	})
	spec.check(t, api, "/api/v1/org/nodes", "/api/v1/org/nodes", nil)
	spec.check(t, api, "/api/v1/org/nodes/root", "/api/v1/org/nodes/{path}", map[string]string{"path": "root"})
	spec.check(t, api, "/api/v1/org/nodes/root/settings", "/api/v1/org/nodes/{path}/settings", map[string]string{"path": "root"})

	// The samples go the way of every record: trash, then purge.
	for i := len(s.order) - 1; i >= 0; i-- {
		name := s.order[i]
		base := "/api/v1/records/" + name + "/" + s.made[name]
		if r := call(t, api, http.MethodDelete, base, "", nil); r.code != 204 {
			t.Fatalf("trash %s: %d %s", name, r.code, r.raw)
		}
		if r := call(t, api, http.MethodPost, base+":purge", "", nil); r.code != 204 {
			t.Fatalf("purge %s: %d %s", name, r.code, r.raw)
		}
		s.purged = append(s.purged, name)
	}
	t.Logf("checked %d types against the OpenAPI description: %s", len(names), strings.Join(names, ", "))
}

// installAllPackages installs every package in packages/, dependencies first, and returns
// their types by qualified name.
func installAllPackages(t *testing.T, api *testAPI) map[string]meta.Type {
	t.Helper()
	entries, err := os.ReadDir("../../packages")
	if err != nil {
		t.Fatal(err)
	}
	pending := map[string]meta.Package{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := meta.Load(filepath.Join("../../packages", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		pending[e.Name()] = p
	}
	types := map[string]meta.Type{}
	installed := map[string]bool{}
	for len(pending) > 0 {
		progress := false
		for _, dir := range sortedKeys(pending) {
			p := pending[dir]
			ready := true
			for _, d := range p.Depends {
				ready = ready && installed[d]
			}
			if !ready {
				continue
			}
			// core and probe are in already (apiServer); installing again changes nothing.
			p = installPackage(t, t.Context(), api.rt, filepath.Join("../../packages", dir))
			for _, typ := range p.Types {
				types[typ.QualifiedName()] = typ
			}
			installed[p.Name] = true
			delete(pending, dir)
			progress = true
		}
		if !progress {
			t.Fatalf("packages with dependencies that are not in packages/: %v", sortedKeys(pending))
		}
	}
	return types
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- the description ---

type apiSpec struct{ doc *openapi3.T }

// strictSpec loads the effective description the server hands out, checks that it is valid
// OpenAPI and closes the object schemas of the components: a field the description does not
// declare is an error.
func strictSpec(t *testing.T, api *testAPI) apiSpec {
	t.Helper()
	code, _, raw := fetch(t, api, "/api/v1/openapi.json")
	if code != 200 {
		t.Fatalf("openapi.json: %d", code)
	}
	doc, err := openapi3.NewLoader().LoadFromData(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(t.Context()); err != nil {
		t.Fatalf("the effective description is not valid OpenAPI: %v", err)
	}
	closed := false
	for _, ref := range doc.Components.Schemas {
		s := ref.Value
		if s != nil && s.Type.Is(openapi3.TypeObject) && len(s.Properties) > 0 &&
			s.AdditionalProperties.Has == nil && s.AdditionalProperties.Schema == nil {
			s.AdditionalProperties = openapi3.AdditionalProperties{Has: &closed}
		}
	}
	return apiSpec{doc: doc}
}

// check fetches path and validates status, content type and body against the GET operation
// of template.
func (s apiSpec) check(t *testing.T, api *testAPI, path, template string, params map[string]string) {
	t.Helper()
	item := s.doc.Paths.Value(template)
	if item == nil || item.Get == nil {
		t.Errorf("GET %s: not in the description", template)
		return
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	code, hdr, raw := fetch(t, api, path)
	if code != http.StatusOK {
		t.Errorf("GET %s: %d %s", path, code, raw)
		return
	}
	opts := &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: params,
			Route:      &routers.Route{Spec: s.doc, Path: template, PathItem: item, Method: http.MethodGet, Operation: item.Get},
			Options:    opts,
		},
		Status:  code,
		Header:  hdr,
		Options: opts,
	}
	in.SetBodyBytes(raw)
	if err := openapi3filter.ValidateResponse(t.Context(), in); err != nil {
		t.Errorf("GET %s does not match the description:\n%v\nbody: %.600s", path, err, raw)
	}
}

// fetch is a GET with the session that keeps the whole body (call decodes objects only).
func fetch(t *testing.T, api *testAPI, path string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, api.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(csrfHeader, csrfValue)
	req.Header.Set("Cookie", sessionCookie+"="+api.cookie)
	res, err := api.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, raw
}

// --- sample records ---

// sampler makes one record per type from its definition: every field that may be written gets
// a value, required references point at the sample of their target type. Optional references
// stay empty, because they may point at the type itself (the parent of a tree list).
type sampler struct {
	t      *testing.T
	api    *testAPI
	types  map[string]meta.Type
	made   map[string]string // type → id of its sample
	order  []string          // in order of creation
	busy   map[string]bool   // types being made, against cycles of required references
	purged []string
}

func (s *sampler) record(name string) string {
	t := s.t
	t.Helper()
	if id, ok := s.made[name]; ok {
		return id
	}
	typ, ok := s.types[name]
	if !ok {
		t.Fatalf("a required reference points at %s, which no package in packages/ brings", name)
	}
	if s.busy[name] {
		t.Fatalf("required references run in a circle through %s", name)
	}
	s.busy[name] = true
	body := map[string]any{}
	for _, f := range typ.Fields {
		if f.Calculated() {
			continue
		}
		if f.Type == meta.FieldReference {
			if f.Required {
				body[f.Name] = s.record(f.To)
			}
			continue
		}
		body[f.Name] = sampleValue(f)
	}
	// Fields a type checks beyond their kind (records.CheckRecord) need values that pass.
	for k, v := range checkedSamples[name] {
		body[k] = v
	}
	if len(typ.Actors) > 0 {
		// One holder per role, of the first type that may hold it (WF3).
		roles := map[string]any{}
		for _, r := range typ.Actors {
			roles[r.Name] = []any{map[string]any{"type": r.To[0], "id": s.record(r.To[0])}}
		}
		body["actors"] = roles
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := call(t, s.api, http.MethodPost, "/api/v1/records/"+name, string(raw), nil)
	if r.code != http.StatusCreated {
		t.Fatalf("create a sample of %s from %s: %d %s", name, raw, r.code, r.raw)
	}
	id, _ := r.body["id"].(string)
	s.made[name] = id
	s.order = append(s.order, name)
	delete(s.busy, name)
	return id
}

// checkedSamples are the values of fields whose content a type checks (WF9: an SLA names an
// installed type and has conditions in CEL; WF14: a recurrence has a schedule, a time zone and
// a template; WF15: a notification has events, recipients and placeholders); the sample SLA
// never holds and the samples of recurrence and notification are not active, so they time,
// create and send nothing; a saved search needs an installed type (UI4); a form asks for the
// required title of its target and is not offered (UI11); a notice in the portal names a text
// field of an installed type and is not shown (SD5).
var checkedSamples = map[string]map[string]any{
	"core.sla": {"target_type": "probe.probe", "applies": "false", "pause": "false", "stop": "true"},
	"core.recurrence": {"target_type": "probe.probe", "schedule": "0 7 * * 1", "timezone": "Europe/Berlin",
		"template": `{"title": "apitest"}`, "active": false},
	"core.notification": {"target_type": "probe.probe", "events": "create", "recipients": "actors.assigned",
		"condition": "false", "subject": "apitest", "body": "apitest", "active": false},
	"core.saved_search": {"target_type": "probe.probe", "query": "", "columns": "", "sort": ""},
	"core.form": {"target_type": "probe.probe", "active": false,
		"definition": `{"sections": [{"id": "s1", "title": {"de": "Anliegen"}, "questions": [{"id": "q1", "field": "title"}]}]}`},
	"core.portal_notice": {"target_type": "probe.probe", "query": "", "text_field": "title", "active": false},
}

// sampleValue is a valid value of a field that is not a reference.
func sampleValue(f meta.Field) any {
	switch f.Type {
	case meta.FieldText:
		v := fmt.Sprintf("apitest %s %d", f.Name, time.Now().UnixNano())
		if f.MaxLength > 0 && len(v) > f.MaxLength {
			v = v[len(v)-f.MaxLength:]
		}
		return v
	case meta.FieldNumber:
		v := 1.0
		if f.Min != nil {
			v = *f.Min
		}
		if f.Max != nil && v > *f.Max {
			v = *f.Max
		}
		return v
	case meta.FieldBoolean:
		return true
	case meta.FieldDate:
		return "2026-10-28"
	case meta.FieldDateTime:
		return "2026-10-28T09:00:00Z"
	case meta.FieldChoice:
		if len(f.Options) > 0 {
			return f.Options[0].Value
		}
	}
	return nil
}

// cleanup purges what the test left behind when it stopped early (the test context is gone
// by then, so not over HTTP).
func (s *sampler) cleanup() {
	done := map[string]bool{}
	for _, n := range s.purged {
		done[n] = true
	}
	recs := records.New(db.Single(s.api.pool))
	for i := len(s.order) - 1; i >= 0; i-- {
		name := s.order[i]
		if done[name] {
			continue
		}
		id, err := uuid.Parse(s.made[name])
		if err != nil {
			continue
		}
		_ = recs.Trash(context.Background(), name, id, nil)
		_ = recs.Purge(context.Background(), name, id, nil)
	}
}
