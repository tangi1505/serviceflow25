// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Package forms runs the forms (UI11; Inventar G130, G131; KF8; Hauptplan E133). A form is a
// record of the core type core.form: a target type, sections with questions as JSON, and
// whether the portal offers it. A question is bound to a field of the target type, or free
// with a type of the core (text, number, boolean, date, datetime, choice); the answers to the
// bound ones become the new record, the free ones its first timeline entry, in one
// transaction. Writing forms goes through the records and the rights on core.form like any
// type; who may create the target type sees the form and submits it. Sections and questions
// show only when their conditions hold (UI12, E134): the portal hides them, Submit drops their
// answers and asks for "required" only where shown. Texts carry German and English; the entry
// is German. A form is offered in the portal, in the workplace or in both (SD9): a note taken
// by the service desk asks for a role (the requester, found by search) instead of filling it
// with the one who submits, and sets fixed values no one sees (the channel "phone").
package forms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// TypeForm is the core type of the forms.
const TypeForm = "core.form"

// requesterRole is the role a submission fills with the person submitting, when the target
// type has it (as the portal does, UI10).
const requesterRole = "requester"

// Definition is the content of a form, kept as JSON in the field definition. Title and Intro
// are the name and the description of the form in other languages (UI12): the record keeps
// them in German only (E98).
type Definition struct {
	Title    meta.Label `json:"title,omitempty"`
	Intro    meta.Label `json:"intro,omitempty"`
	Sections []Section  `json:"sections"`
	// Fixed are values of fields the record gets without a question (SD9); a reference to a
	// select list may name the key of its entry, e.g. {"channel": "phone"}.
	Fixed map[string]any `json:"fixed,omitempty"`
}

// The places a form is offered (the field place of core.form, SD9).
const (
	PlacePortal    = "portal"
	PlaceWorkplace = "workplace"
	PlaceBoth      = "both"
)

// Section groups questions under a title.
type Section struct {
	ID        string      `json:"id"`
	Title     meta.Label  `json:"title"`
	ShowIf    []Condition `json:"show_if,omitempty"`
	Questions []Question  `json:"questions"`
}

// Condition says when a section or question shows (UI12): the answer to an earlier question
// equals Value, differs from it, is given, or is empty. All conditions of an element must hold.
type Condition struct {
	Question string `json:"question"`
	Op       string `json:"op"`
	Value    string `json:"value,omitempty"`
}

// Operators of a condition.
const (
	OpEq       = "eq"
	OpNe       = "ne"
	OpAnswered = "answered"
	OpEmpty    = "empty"
)

// Question is bound to a field of the target type (Field), to a role at the record (Role) or
// free (Type).
type Question struct {
	ID string `json:"id"`
	// Field names the field of the target type the answer goes to.
	Field string `json:"field,omitempty"`
	// Role names the role at the record the answer holds (SD9): the id of a holder of the
	// role's one type of holder, e.g. the requester found by search.
	Role string `json:"role,omitempty"`
	// Type is the type of a free question, one of freeTypes.
	Type meta.FieldType `json:"type,omitempty"`
	// Label is the question; for a bound question it replaces the field's label, if set.
	Label       meta.Label    `json:"label,omitempty"`
	Description meta.Label    `json:"description,omitempty"`
	Required    bool          `json:"required,omitempty"`
	Options     []meta.Option `json:"options,omitempty"`
	ShowIf      []Condition   `json:"show_if,omitempty"`
}

// freeTypes are the field types a free question may have: the answer is text in the entry,
// so references (a record to pick) stay with the bound questions.
var freeTypes = []meta.FieldType{meta.FieldText, meta.FieldNumber, meta.FieldBoolean, meta.FieldDate, meta.FieldDateTime, meta.FieldChoice}

// Parse reads a definition.
func Parse(raw string) (Definition, error) {
	var d Definition
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return Definition{}, fmt.Errorf("must be the JSON of a form, {\"sections\": [...]}: %w", err)
	}
	return d, nil
}

// Problems checks a definition against its target type; the field names of the problems are
// paths into the definition, such as sections[0].questions[1].field.
func Problems(d Definition, target meta.Type) []records.Problem {
	var ps []records.Problem
	add := func(path, code, msg string) {
		ps = append(ps, records.Problem{Field: "definition", Code: code, Message: path + ": " + msg})
	}
	if len(d.Sections) == 0 {
		add("sections", "required", "a form needs at least one section")
	}
	ids, bound, roles := map[string]bool{}, map[string]bool{}, map[string]bool{}
	free := false
	// earlier are the questions before the element being checked: a condition may only name
	// those, so the conditions have no circles.
	earlier := map[string]Question{}
	checkConditions := func(path string, cs []Condition) {
		for k, c := range cs {
			cp := fmt.Sprintf("%s.show_if[%d]", path, k)
			q, ok := earlier[c.Question]
			if !ok {
				add(cp+".question", "unknown", fmt.Sprintf("%q is not a question before this one", c.Question))
				continue
			}
			switch c.Op {
			case OpAnswered, OpEmpty:
				if c.Value != "" {
					add(cp+".value", "invalid", c.Op+" takes no value")
				}
			case OpEq, OpNe:
				if c.Value == "" {
					add(cp+".value", "required", c.Op+" needs a value")
				} else if values := optionValues(q, target); values != nil && !slices.Contains(values, c.Value) {
					add(cp+".value", "choice", fmt.Sprintf("must be one of %s", strings.Join(values, ", ")))
				}
			default:
				add(cp+".op", "choice", "must be eq, ne, answered or empty")
			}
		}
	}
	// needed are the fields the target cannot be created without: their questions always show.
	needed := func(q Question) bool {
		f, ok := target.Field(q.Field)
		return q.Field != "" && ok && f.Required && f.Default == nil
	}
	for i, s := range d.Sections {
		sp := fmt.Sprintf("sections[%d]", i)
		if s.ID == "" || ids[s.ID] {
			add(sp+".id", "unique", "needs an id of its own")
		}
		ids[s.ID] = true
		if strings.TrimSpace(s.Title["de"]) == "" {
			add(sp+".title", "required", "needs a German title")
		}
		checkConditions(sp, s.ShowIf)
		for j, q := range s.Questions {
			qp := fmt.Sprintf("%s.questions[%d]", sp, j)
			if q.ID == "" || ids[q.ID] {
				add(qp+".id", "unique", "needs an id of its own")
			}
			ids[q.ID] = true
			checkConditions(qp, q.ShowIf)
			if needed(q) && (len(q.ShowIf) > 0 || len(s.ShowIf) > 0) {
				add(qp+".show_if", "required", fmt.Sprintf("%s is required by %s and cannot be hidden", q.Field, target.QualifiedName()))
			}
			earlier[q.ID] = q
			kinds := 0
			for _, s := range []string{q.Field, q.Role, string(q.Type)} {
				if s != "" {
					kinds++
				}
			}
			switch {
			case kinds > 1:
				add(qp, "invalid", "is bound to a field, to a role or free with a type, only one of them")
			case q.Role != "":
				role, ok := target.Actor(q.Role)
				switch {
				case !ok:
					add(qp+".role", "unknown", fmt.Sprintf("%s has no role %q", target.QualifiedName(), q.Role))
				case len(role.To) != 1:
					add(qp+".role", "invalid", fmt.Sprintf("%s takes %s; a question needs a role with one type of holder", q.Role, strings.Join(role.To, " and ")))
				case roles[q.Role]:
					add(qp+".role", "unique", fmt.Sprintf("%s is asked twice", q.Role))
				}
				roles[q.Role] = true
				if len(q.Options) > 0 {
					add(qp+".options", "invalid", "a role question has no options")
				}
			case q.Field != "":
				f, ok := target.Field(q.Field)
				switch {
				case !ok:
					add(qp+".field", "unknown", fmt.Sprintf("%s has no field %q", target.QualifiedName(), q.Field))
				case f.Calculated():
					add(qp+".field", "calculated", fmt.Sprintf("%s is calculated and cannot be answered", q.Field))
				case bound[q.Field]:
					add(qp+".field", "unique", fmt.Sprintf("%s is asked twice", q.Field))
				}
				bound[q.Field] = true
				if len(q.Options) > 0 {
					add(qp+".options", "invalid", "a bound question has the options of its field")
				}
			case q.Type != "":
				free = true
				if !slices.Contains(freeTypes, q.Type) {
					add(qp+".type", "choice", "must be text, number, boolean, date, datetime or choice")
				}
				if strings.TrimSpace(q.Label["de"]) == "" {
					add(qp+".label", "required", "a free question needs a German label")
				}
				if q.Type == meta.FieldChoice && len(q.Options) == 0 {
					add(qp+".options", "required", "a choice needs options")
				}
				values := map[string]bool{}
				for _, o := range q.Options {
					if o.Value == "" || values[o.Value] {
						add(qp+".options", "unique", "every option needs a value of its own")
					}
					values[o.Value] = true
				}
			default:
				add(qp, "required", "needs a field of the target type or the type of a free question")
			}
		}
	}
	// Fixed values go to fields no question asks.
	for _, name := range slices.Sorted(maps.Keys(d.Fixed)) {
		f, ok := target.Field(name)
		switch {
		case !ok:
			add("fixed."+name, "unknown", fmt.Sprintf("%s has no field %q", target.QualifiedName(), name))
		case f.Calculated():
			add("fixed."+name, "calculated", fmt.Sprintf("%s is calculated and cannot be set", name))
		case bound[name]:
			add("fixed."+name, "unique", fmt.Sprintf("%s is asked by a question and cannot be fixed too", name))
		}
		bound[name] = true
	}
	// What the target type needs to be created must be asked.
	for _, f := range target.Fields {
		if f.Required && f.Default == nil && !f.Calculated() && !bound[f.Name] {
			add("sections", "required", fmt.Sprintf("%s needs %s: add a question for it", target.QualifiedName(), f.Name))
		}
	}
	if free && !target.HasTrait(meta.TraitTimeline) {
		add("sections", "timeline", fmt.Sprintf("free questions land in the timeline, and %s has none: bind every question to a field", target.QualifiedName()))
	}
	return ps
}

// optionValues are the values a condition on the question may compare with: the options of a
// choice, true and false for a yes/no question, nil for any value.
func optionValues(q Question, target meta.Type) []string {
	typ, options := q.Type, q.Options
	if q.Field != "" {
		f, _ := target.Field(q.Field)
		typ, options = f.Type, f.Options
	}
	switch typ {
	case meta.FieldBoolean:
		return []string{"true", "false"}
	case meta.FieldChoice:
		out := make([]string, 0, len(options))
		for _, o := range options {
			out = append(out, o.Value)
		}
		return out
	}
	return nil
}

// holds says whether all conditions hold for the answers given so far (hidden ones are not
// among them).
func holds(cs []Condition, answers map[string]any) bool {
	for _, c := range cs {
		v, given := answers[c.Question]
		text := answerText(v)
		empty := !given || v == nil || text == ""
		switch c.Op {
		case OpAnswered:
			if empty {
				return false
			}
		case OpEmpty:
			if !empty {
				return false
			}
		case OpEq:
			if empty || text != c.Value {
				return false
			}
		case OpNe:
			if !empty && text == c.Value {
				return false
			}
		}
	}
	return true
}

// answerText is an answer as a condition compares it: true or false, a number without
// trailing zeros, text as it is.
func answerText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return strconv.FormatFloat(f, 'f', -1, 64)
		}
		return x.String()
	}
	return fmt.Sprint(v)
}

// Check is the RecordCheck of the forms.
func Check(rec *records.Service) records.RecordCheck {
	return func(ctx context.Context, t meta.Type, r map[string]any) []records.Problem {
		if t.QualifiedName() != TypeForm {
			return nil
		}
		name, _ := r["target_type"].(string)
		target, err := rec.Type(ctx, name)
		if err != nil {
			return []records.Problem{{Field: "target_type", Code: "unknown", Message: fmt.Sprintf("type %s is not installed", name)}}
		}
		raw, _ := r["definition"].(string)
		d, err := Parse(raw)
		if err != nil {
			return []records.Problem{{Field: "definition", Code: "type", Message: err.Error()}}
		}
		ps := Problems(d, target)
		if len(ps) == 0 {
			if _, err := fixedValues(ctx, rec, target, d.Fixed); err != nil {
				ps = append(ps, records.Problem{Field: "definition", Code: "invalid", Message: "fixed: " + err.Error()})
			}
		}
		return ps
	}
}

// fixedValues are the fixed values of a form as the record takes them: the key of a select
// list's entry becomes its id (records.Service.ListKeys). The values themselves are checked by
// the record on submitting.
func fixedValues(ctx context.Context, rec *records.Service, target meta.Type, fixed map[string]any) (map[string]any, error) {
	return rec.ListKeys(ctx, target, fixed)
}

// Form is a form as the portal gets it.
type Form struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	TargetType  string     `json:"target_type"`
	Definition  Definition `json:"definition"`
	// Place is where the form is offered: portal, workplace or both (SD9).
	Place string `json:"place"`
	// OrgPath is the node of the form: it is offered to whoever may create its target there,
	// and its records are created there (architektur.md §5.1).
	OrgPath string `json:"-"`
}

// Service lists and submits forms.
type Service struct {
	rec *records.Service
}

// New returns the service.
func New(rec *records.Service) *Service { return &Service{rec: rec} }

// ErrNotFound is a form that does not exist, is not offered or not for the caller.
var ErrNotFound = records.ErrNotFound

// offered says whether the caller may submit the form: create its target type at the form's
// node. Creating it elsewhere is no reason to see it (architektur.md §5.1, gap 4).
func offered(ctx context.Context, f Form) bool {
	a := authz.From(ctx)
	if a.IsSystem() {
		return true
	}
	probe := map[string]any{"org_path": f.OrgPath, "created_by": a.AccountID.String()}
	return a.Permits(f.TargetType, authz.ActionCreate, probe)
}

// List returns the forms offered at a place (portal or workplace) whose target type the caller
// may create.
func (s *Service) List(ctx context.Context, place string) ([]Form, error) {
	// The forms themselves are read with the rights of the system: offering one is the
	// decision of whoever wrote it; the caller's right is the one on the target type.
	where := json.RawMessage(`{"field": "active", "eq": true}`)
	page, _, err := s.rec.Query(authz.WithAccess(ctx, authz.System()), TypeForm, records.Query{Where: where, Sort: []string{"name"}, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := []Form{}
	for _, r := range page.Items {
		f, err := formOf(r)
		if err != nil {
			continue // the check refuses it on writing; an old one is not offered
		}
		if (f.Place == place || f.Place == PlaceBoth) && offered(ctx, f) {
			out = append(out, f)
		}
	}
	return out, nil
}

// Get returns one offered form the caller may submit.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Form, error) {
	r, err := s.rec.Get(authz.WithAccess(ctx, authz.System()), TypeForm, id)
	if err != nil {
		return Form{}, err
	}
	f, err := formOf(r)
	if err != nil || r["active"] != true || !offered(ctx, f) {
		return Form{}, fmt.Errorf("form %s: %w", id, ErrNotFound)
	}
	return f, nil
}

func formOf(r records.Record) (Form, error) {
	id, err := uuid.Parse(fmt.Sprint(r["id"]))
	if err != nil {
		return Form{}, err
	}
	raw, _ := r["definition"].(string)
	d, err := Parse(raw)
	if err != nil {
		return Form{}, err
	}
	name, _ := r["name"].(string)
	desc, _ := r["description"].(string)
	target, _ := r["target_type"].(string)
	node, _ := r["org_path"].(string)
	place, _ := r["place"].(string)
	if place == "" {
		place = PlacePortal // a form from before SD9
	}
	return Form{ID: id, Name: name, Description: desc, TargetType: target, Definition: d, OrgPath: node, Place: place}, nil
}

// Submit creates the record of a form from the answers, keyed by question id, with the
// rights of the caller; the person submitting becomes the requester when the type has that
// role and no question asks for it. The fixed values go along. The free answers are the
// first entry of the timeline, written with the record.
func (s *Service) Submit(ctx context.Context, id uuid.UUID, answers map[string]any, actor records.Actor) (records.Record, error) {
	f, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	target, err := s.rec.Type(ctx, f.TargetType)
	if err != nil {
		return nil, err
	}
	// The record lands at the node of the form, never quietly at the root (architektur.md §5.1).
	input, err := fixedValues(ctx, s.rec, target, f.Definition.Fixed)
	if err != nil {
		return nil, err
	}
	if f.OrgPath != "" {
		input["org_path"] = f.OrgPath
	}
	actors := map[string]any{}
	var ps []records.Problem
	var lines []string
	known := map[string]bool{}
	// shown are the answers of the questions that show, in order: the conditions of later
	// elements look at them only.
	shown := map[string]any{}
	for _, sec := range f.Definition.Sections {
		var sectionLines []string
		visible := holds(sec.ShowIf, shown)
		for _, q := range sec.Questions {
			known[q.ID] = true
			if !visible || !holds(q.ShowIf, shown) {
				continue // hidden: its answer is dropped, required does not apply
			}
			v, given := answers[q.ID]
			if given {
				shown[q.ID] = v
			}
			empty := !given || v == nil || v == ""
			if empty && q.Required {
				ps = append(ps, records.Problem{Field: q.ID, Code: "required", Message: "is required"})
				continue
			}
			if q.Field != "" {
				if !empty {
					input[q.Field] = v
				}
				continue
			}
			if q.Role != "" {
				role, _ := target.Actor(q.Role)
				if !empty && len(role.To) == 1 {
					actors[q.Role] = []any{map[string]any{"type": role.To[0], "id": v}}
				}
				continue
			}
			if empty {
				continue
			}
			text, p := freeAnswer(q, v)
			if p != nil {
				ps = append(ps, *p)
				continue
			}
			sectionLines = append(sectionLines, q.Label["de"]+": "+text)
		}
		if len(sectionLines) > 0 {
			lines = append(lines, append([]string{sec.Title["de"]}, sectionLines...)...)
			lines = append(lines, "")
		}
	}
	for qid := range answers {
		if !known[qid] {
			ps = append(ps, records.Problem{Field: qid, Code: "unknown", Message: "is not a question of the form"})
		}
	}
	if len(ps) > 0 {
		return nil, &records.ValidationError{Problems: ps}
	}
	if a := authz.From(ctx); a.PersonID != nil && !asks(f.Definition, requesterRole) {
		if role, ok := target.Actor(requesterRole); ok && slices.Contains(role.To, "core.person") {
			actors[requesterRole] = []any{map[string]any{"type": "core.person", "id": a.PersonID.String()}}
		}
	}
	if len(actors) > 0 {
		input["actors"] = actors
	}
	var entry map[string]any
	if len(lines) > 0 {
		entry = map[string]any{"body": strings.TrimSpace(strings.Join(lines, "\n"))}
	}
	rec, err := s.rec.CreateWithEntry(ctx, f.TargetType, input, entry, actor)
	var verr *records.ValidationError
	if errors.As(err, &verr) {
		// The record's problems named by the questions that asked for the fields.
		byField := map[string]string{}
		for _, sec := range f.Definition.Sections {
			for _, q := range sec.Questions {
				if q.Field != "" {
					byField[q.Field] = q.ID
				}
				if q.Role != "" {
					byField["actors."+q.Role] = q.ID
				}
			}
		}
		for i, p := range verr.Problems {
			if qid, ok := byField[p.Field]; ok {
				verr.Problems[i].Field = qid
			}
		}
	}
	return rec, err
}

// asks says whether a question of the form asks for the role.
func asks(d Definition, role string) bool {
	for _, sec := range d.Sections {
		for _, q := range sec.Questions {
			if q.Role == role {
				return true
			}
		}
	}
	return false
}

// freeAnswer checks the answer to a free question and returns it as German text for the entry
// (E134: the agents read the entries in one language).
func freeAnswer(q Question, v any) (string, *records.Problem) {
	bad := func(code, msg string) (string, *records.Problem) {
		return "", &records.Problem{Field: q.ID, Code: code, Message: msg}
	}
	switch q.Type {
	case meta.FieldBoolean:
		b, ok := v.(bool)
		if !ok {
			return bad("type", "must be true or false")
		}
		if b {
			return "ja", nil
		}
		return "nein", nil
	case meta.FieldNumber:
		// The API reads numbers as json.Number, tests and Go callers pass float64.
		switch v.(type) {
		case float64, json.Number:
			if text := answerText(v); text != "" {
				if _, err := strconv.ParseFloat(text, 64); err == nil {
					return text, nil
				}
			}
		}
		return bad("number", "must be a number")
	case meta.FieldChoice:
		s, _ := v.(string)
		for _, o := range q.Options {
			if o.Value == s {
				return o.Label["de"], nil
			}
		}
		return bad("choice", "is not one of the options")
	default:
		s, ok := v.(string)
		if !ok {
			return bad("type", "must be text")
		}
		if len(s) > 10000 {
			return bad("max_length", "is longer than 10,000 characters")
		}
		return s, nil
	}
}
