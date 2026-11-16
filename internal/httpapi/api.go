// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// maxBody bounds a JSON request body (1 MiB); files go through their own endpoint (PL18).
const maxBody = 1 << 20

// server implements the generated interface (architektur.md §6; Hauptplan E11, E60, E70, E71).
type server struct {
	o Options
}

var _ gen.ServerInterface = (*server)(nil)

// --- system ---

func (s *server) GetHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, gen.Status{Status: "ok"})
}

func (s *server) GetReadyz(w http.ResponseWriter, r *http.Request) {
	readiness(s.o.Checks, s.o.Logger)(w, r)
}

func (s *server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.o.Version})
}

// GetOpenAPI serves the fixed description extended by every installed type.
func (s *server) GetOpenAPI(w http.ResponseWriter, r *http.Request) {
	types, err := s.o.Records.Types(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	for i := range types {
		types[i] = records.VisibleType(r.Context(), types[i])
	}
	doc, err := effectiveSpec(types)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// --- types ---

func (s *server) ListTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.o.Records.Types(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	// Only the types the session may read at all; the fields of each are narrowed to the
	// readable ones (PL9).
	a := authz.From(r.Context())
	visible := make([]meta.Type, 0, len(types))
	for _, t := range types {
		if a.IsSystem() || a.Allowed(t.QualifiedName(), authz.ActionRead) {
			visible = append(visible, s.o.Records.Timed(r.Context(), records.VisibleType(r.Context(), t)))
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

// ListPackages names the installed packages with label and symbol (UI36); every session may
// read them, the navigation groups the types by them.
func (s *server) ListPackages(w http.ResponseWriter, r *http.Request) {
	list, err := s.o.Records.Packages(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetType(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	t, err := s.o.Records.Type(r.Context(), typeName)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, s.o.Records.Timed(r.Context(), records.VisibleType(r.Context(), t)))
}

// --- records ---

func (s *server) ListRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, p gen.ListRecordsParams) {
	o := records.ListOptions{}
	if p.Limit != nil {
		o.Limit = *p.Limit
	}
	if p.After != nil {
		o.After = *p.After
	}
	if p.Sort != nil {
		o.Sort = *p.Sort
	}
	if p.Fields != nil && *p.Fields != "" {
		o.Fields = strings.Split(*p.Fields, ",")
	}
	if p.Count != nil {
		o.Count = *p.Count
	}
	if p.Filter != nil {
		o.Filter = *p.Filter
	}
	page, err := s.o.Records.List(r.Context(), typeName, o)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if page.Items == nil {
		page.Items = []records.Record{}
	}
	if !s.localize(w, r, typeName, page.Items...) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// QueryRecords is the JSON query (PL14, architektur.md §6.3): records as a page, or rows
// when the query groups or aggregates.
func (s *server) QueryRecords(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, _ gen.QueryRecordsParams) {
	var q records.Query
	if !decode(w, r, &q) {
		return
	}
	page, rows, err := s.o.Records.Query(r.Context(), typeName, q)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if rows != nil {
		writeJSON(w, http.StatusOK, rows)
		return
	}
	if !s.localize(w, r, typeName, page.Items...) {
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *server) CreateRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName) {
	input, ok := readBody(w, r)
	if !ok {
		return
	}
	rec, err := s.o.Records.Create(r.Context(), typeName, input, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !s.localize(w, r, typeName, rec) {
		return
	}
	w.Header().Set("ETag", etag(rec))
	w.Header().Set("Location", fmt.Sprintf("/api/v1/records/%s/%s", typeName, rec["id"]))
	writeJSON(w, http.StatusCreated, rec)
}

func (s *server) GetRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, _ gen.GetRecordParams) {
	rec, err := s.o.Records.Get(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !s.localize(w, r, typeName, rec) {
		return
	}
	w.Header().Set("ETag", etag(rec))
	writeJSON(w, http.StatusOK, rec)
}

func (s *server) UpdateRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, p gen.UpdateRecordParams) {
	if p.IfMatch == nil || strings.TrimSpace(*p.IfMatch) == "" {
		writeProblem(w, r, http.StatusPreconditionRequired, "precondition_required",
			"send If-Match with the ETag of the record as you last saw it", nil)
		return
	}
	version, err := parseETag(*p.IfMatch)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "If-Match must be the ETag of the record, e.g. \"3\"", nil)
		return
	}
	patch, ok := readBody(w, r)
	if !ok {
		return
	}
	rec, err := s.o.Records.Update(r.Context(), typeName, id, version, patch, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	if !s.localize(w, r, typeName, rec) {
		return
	}
	w.Header().Set("ETag", etag(rec))
	writeJSON(w, http.StatusOK, rec)
}

func (s *server) TrashRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	s.noContent(w, r, s.o.Records.Trash(r.Context(), typeName, id, actor(r)))
}

func (s *server) RestoreRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	s.noContent(w, r, s.o.Records.Restore(r.Context(), typeName, id, actor(r)))
}

func (s *server) PurgeRecord(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	s.noContent(w, r, s.o.Records.Purge(r.Context(), typeName, id, actor(r)))
}

func (s *server) RecordHistory(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if _, err := s.o.Records.Type(r.Context(), typeName); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	rows, err := s.o.Records.History(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	out := make([]gen.AuditEntry, 0, len(rows))
	for _, a := range rows {
		e := gen.AuditEntry{Id: a.ID, Version: a.Version, Action: gen.AuditEntryAction(a.Action), At: a.At.Time}
		if a.Field.Valid {
			f := a.Field.String
			e.Field = &f
			_ = json.Unmarshal(a.OldValue, &e.Old)
			_ = json.Unmarshal(a.NewValue, &e.New)
		}
		if a.Actor.Valid {
			id := a.Actor.UUID
			e.Actor = &id
		}
		if a.Rule.Valid {
			rule := a.Rule.String
			e.Rule = &rule
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) noContent(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func readBody(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var input map[string]any
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBody))
	dec.UseNumber()
	if err := dec.Decode(&input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be a JSON object: "+err.Error(), nil)
		return nil, false
	}
	if input == nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "body must be a JSON object", nil)
		return nil, false
	}
	return input, true
}

func etag(rec records.Record) string {
	return fmt.Sprintf("\"%d\"", rec["version"])
}

func parseETag(s string) (int64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "W/")
	s = strings.Trim(s, "\"")
	return strconv.ParseInt(s, 10, 64)
}

// effectiveSpec adds to the embedded description, per installed type: a schema and the
// concrete record paths, e.g. /api/v1/records/probe.probe (architektur.md §6.1).
func effectiveSpec(types []meta.Type) (*openapi3.T, error) {
	doc, err := gen.GetSpec()
	if err != nil {
		return nil, err
	}
	const generic = "/api/v1/records/{typeName}"
	for _, t := range types {
		name := t.QualifiedName()
		doc.Components.Schemas[name] = &openapi3.SchemaRef{Value: typeSchema(t)}
		ref := "#/components/schemas/" + name
		suffixes := []string{"", ":query", ":bulk", ":field-states", "/{id}", "/{id}:restore", "/{id}:purge", "/{id}/relations", "/{id}/graph", "/{id}/impact"}
		// Sub-resources come with the traits of the type (PL17).
		for _, sub := range t.SubResources() {
			suffixes = append(suffixes, "/{id}/"+sub)
			switch sub {
			case "documents":
				suffixes = append(suffixes, "/{id}/documents/{fileId}")
			case "costs":
				suffixes = append(suffixes, "/{id}/costs/{costId}")
			case "history":
				// Versions come from the history (PL19, E96).
				suffixes = append(suffixes, "/{id}/versions", "/{id}/versions/{n}", "/{id}/versions/{n}:restore", "/{id}/compare")
			}
		}
		for _, suffix := range suffixes {
			src := doc.Paths.Find(generic + suffix)
			if src == nil {
				continue
			}
			cp := clonePathItem(src, name, ref)
			doc.Paths.Set("/api/v1/records/"+name+suffix, cp)
		}
	}
	return doc, nil
}

func typeSchema(t meta.Type) *openapi3.Schema {
	s := openapi3.NewObjectSchema()
	s.Title = t.Label["en"]
	s.Description = t.Description["en"]
	add := func(name string, sch *openapi3.Schema, readOnly bool) {
		sch.ReadOnly = readOnly
		s.Properties[name] = &openapi3.SchemaRef{Value: sch}
	}
	add("id", openapi3.NewUUIDSchema(), true)
	add("version", openapi3.NewInt64Schema(), true)
	add("state", openapi3.NewStringSchema().WithNullable(), true)
	add("org_path", openapi3.NewStringSchema(), true)
	add("source", openapi3.NewStringSchema().WithNullable(), true)
	add("external_key", openapi3.NewStringSchema().WithNullable(), true)
	add("created_at", openapi3.NewDateTimeSchema(), true)
	add("created_by", openapi3.NewUUIDSchema().WithNullable(), true)
	add("updated_at", openapi3.NewDateTimeSchema(), true)
	add("updated_by", openapi3.NewUUIDSchema().WithNullable(), true)
	add("deleted_at", openapi3.NewDateTimeSchema().WithNullable(), true)
	// The deadline the SLA clocks leave for the list (UI34).
	due := openapi3.NewDateTimeSchema().WithNullable()
	due.Description = "the next deadline of a running SLA clock"
	add("sla_due_at", due, true)
	warn := openapi3.NewDateTimeSchema().WithNullable()
	warn.Description = "when that clock has used three quarters of its target"
	add("sla_warn_at", warn, true)
	paused := openapi3.NewBoolSchema()
	paused.Description = "no clock runs, but one is paused"
	add("sla_paused", paused, true)
	if t.IsTree() {
		// Derived by the core next to the fields of a tree list (PL16; records.fullNameColumn).
		full := openapi3.NewStringSchema()
		full.Description = `the names from the top down, joined by " > "`
		add("full_name", full, true)
	}
	for _, f := range t.Fields {
		var sch *openapi3.Schema
		switch f.Type {
		case meta.FieldText:
			sch = openapi3.NewStringSchema()
			if f.MaxLength > 0 {
				sch.WithMaxLength(int64(f.MaxLength))
			}
		case meta.FieldNumber:
			sch = openapi3.NewFloat64Schema()
			if f.Min != nil {
				sch.WithMin(*f.Min)
			}
			if f.Max != nil {
				sch.WithMax(*f.Max)
			}
		case meta.FieldBoolean:
			sch = openapi3.NewBoolSchema()
		case meta.FieldDate:
			sch = openapi3.NewDateTimeSchema()
			sch.Format = "date"
		case meta.FieldDateTime:
			sch = openapi3.NewDateTimeSchema()
		case meta.FieldChoice:
			vals := make([]any, len(f.Options))
			for i, o := range f.Options {
				vals[i] = o.Value
			}
			sch = openapi3.NewStringSchema().WithEnum(vals...)
		case meta.FieldReference:
			sch = openapi3.NewUUIDSchema()
			sch.Description = "id of a " + f.To
		case meta.FieldSequence:
			// The running number the core gives (PL49), shown with its prefix.
			sch = openapi3.NewInt64Schema().WithNullable()
			sch.Description = "running number, shown as " + f.Prefix + "<number>"
		default:
			sch = openapi3.NewSchema()
		}
		if f.Translatable {
			// The value of one language, or every language with ?translations=all (PL21).
			byLang := openapi3.NewObjectSchema()
			for _, l := range meta.Languages {
				byLang.Properties[l] = &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithNullable()}
			}
			sch = &openapi3.Schema{OneOf: openapi3.SchemaRefs{{Value: sch}, {Value: byLang}},
				Description: "translatable: the value in the language of Accept-Language, German without a translation"}
		}
		sch.Title = f.Label["en"]
		if f.Computed {
			// Kept by the core, never written: a sum (the costs, PL20) is never null, anything
			// else is until the core sets it (the outcome of an approval, WF6; the durations of a
			// resolution, UI14a).
			sch.Description = "computed by the core"
			sch.Nullable = !f.StartsAtZero()
		} else if f.Formula != "" {
			// Calculated on every write (PL22): null when a field the formula uses is empty.
			sch.Description = "calculated: " + f.Formula
			sch.Nullable = true
		} else if !f.Required {
			sch.Nullable = true
		} else {
			s.Required = append(s.Required, f.Name)
		}
		if f.Default != nil {
			sch.Default = f.Default
		}
		add(f.Name, sch, f.Calculated())
	}
	if t.HasTrait(meta.TraitActors) && len(t.Actors) > 0 {
		// The roles at the record (WF3): per role a list of holders; a write names the roles
		// it changes, null or [] clears one.
		roles := openapi3.NewObjectSchema()
		roles.Description = `the roles at the record; the team of the record is the set of its holders`
		for _, r := range t.Actors {
			types := make([]any, len(r.To))
			for i, name := range r.To {
				types[i] = name
			}
			ref := openapi3.NewObjectSchema()
			ref.Properties["type"] = &openapi3.SchemaRef{Value: openapi3.NewStringSchema().WithEnum(types...)}
			ref.Properties["id"] = &openapi3.SchemaRef{Value: openapi3.NewUUIDSchema()}
			ref.Required = []string{"type", "id"}
			list := openapi3.NewArraySchema().WithItems(ref).WithNullable()
			list.Title = r.Label["en"]
			if r.Single {
				list.WithMaxItems(1)
			}
			roles.Properties[r.Name] = &openapi3.SchemaRef{Value: list}
		}
		add("actors", roles, false)
	}
	return s
}

// clonePathItem copies a generic path item, drops the typeName parameter and points the
// record bodies and responses at the type's schema.
func clonePathItem(src *openapi3.PathItem, typeName, ref string) *openapi3.PathItem {
	b, _ := json.Marshal(src)
	var cp openapi3.PathItem
	_ = json.Unmarshal(b, &cp)
	for _, op := range cp.Operations() {
		op.OperationID = op.OperationID + "_" + strings.ReplaceAll(typeName, ".", "_")
		var params openapi3.Parameters
		for _, p := range op.Parameters {
			if p.Ref == "#/components/parameters/typeName" || (p.Value != nil && p.Value.Name == "typeName") {
				continue
			}
			params = append(params, p)
		}
		op.Parameters = params
		if op.RequestBody != nil && op.RequestBody.Value != nil {
			// Only a record body becomes the type's schema; a query, a bulk call or a cost
			// keeps its own (PL31).
			for _, mt := range op.RequestBody.Value.Content {
				if mt.Schema != nil && mt.Schema.Ref == "#/components/schemas/Record" {
					mt.Schema = &openapi3.SchemaRef{Ref: ref}
				}
			}
		}
		for _, resp := range op.Responses.Map() {
			if resp.Value == nil {
				continue
			}
			for _, mt := range resp.Value.Content {
				if mt.Schema == nil {
					continue
				}
				switch mt.Schema.Ref {
				case "#/components/schemas/Record":
					mt.Schema = &openapi3.SchemaRef{Ref: ref}
				case "#/components/schemas/Page":
					page := openapi3.NewObjectSchema()
					page.Required = []string{"items"}
					page.Properties["items"] = &openapi3.SchemaRef{Value: openapi3.NewArraySchema().WithItems(&openapi3.Schema{})}
					page.Properties["items"].Value.Items = &openapi3.SchemaRef{Ref: ref}
					page.Properties["next"] = &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
					mt.Schema = &openapi3.SchemaRef{Value: page}
				}
			}
		}
	}
	return &cp
}
