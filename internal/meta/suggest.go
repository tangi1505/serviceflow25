// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"slices"
	"strings"
)

// Suggest says where the records of a type are suggested (SD18, SD-Plan E12, N13): while a
// record of another type is written, the core searches this type with the text of some of the
// other record's fields. The suggesting type names the others, so they need not know it: an
// article suggests itself for every ticket, the incident knows nothing of knowledge.
//
//	suggest:
//	  label: {de: Wissen, en: Knowledge}
//	  for: ["trait:sd.ticket"]
//	  from: [title, description]
//	  relation: knowledge.uses
//	  apply:
//	    label: {de: Als Lösung übernehmen, en: Use as solution}
//	    set: {solution: "@body", solution_kind: instructed}
//
// For names types or traits as a relation kind does (N11). From are fields of those types;
// a type without one of them is searched with the others. Relation is the kind "link"
// creates, from the record to the suggestion. Apply sets fields of the record: "@<field>" is
// the suggestion's field, anything else the value itself, a select list's entry by its key.
type Suggest struct {
	Label    Label         `json:"label" yaml:"label"`
	For      []string      `json:"for" yaml:"for"`
	From     []string      `json:"from" yaml:"from"`
	Relation string        `json:"relation,omitempty" yaml:"relation"`
	Apply    *SuggestApply `json:"apply,omitempty" yaml:"apply"`
}

// SuggestApply is what using a suggestion sets in the form of the record.
type SuggestApply struct {
	Label Label             `json:"label" yaml:"label"`
	Set   map[string]string `json:"set" yaml:"set"`
}

// SuggestFieldPrefix marks a value of Apply that takes a field of the suggestion.
const SuggestFieldPrefix = "@"

// Suggests reports whether the type's records are suggested for records of t.
func (s Suggest) Suggests(t Type) bool { return relates(s.For, t) }

// validateSuggest checks a type's suggest: a label, at least one type or trait as for
// relation kinds, fields to search with, a relation kind of this package or one it depends on
// that reaches the type, and apply values that name fields of the type.
func validateSuggest(add func(string, string, ...any), where string, t Type, p Package, local, known map[string]bool, trseen map[string]bool) {
	s := t.Suggest
	if s == nil {
		return
	}
	where += ", suggest"
	checkLabel(add, where, "label", s.Label)
	if len(s.For) == 0 {
		add(where, "for needs at least one type or trait")
	}
	for _, name := range s.For {
		if trait, ok := strings.CutPrefix(name, TraitPrefix); ok {
			pkg, tr, dotted := strings.Cut(trait, ".")
			switch {
			case !dotted:
				add(where, "for names %q; a trait goes with its package, as trait:<package>.<trait>", name)
			case pkg == p.Name && !trseen[tr]:
				add(where, "for names %q, which is no trait of this package", name)
			case pkg != p.Name && !slices.Contains(p.Depends, pkg):
				add(where, "for names %q of package %s, which this package does not depend on", name, pkg)
			}
			continue
		}
		if !local[name] && !known[name] {
			add(where, "for names %q, which is neither a type of this package nor installed", name)
		}
	}
	if len(s.From) == 0 {
		add(where, "from needs at least one field to search with")
	}
	for _, f := range s.From {
		if !IsIdent(f) {
			add(where, "from names %q, which is no field name", f)
		}
	}
	if s.Relation != "" {
		pkg, name, dotted := strings.Cut(s.Relation, ".")
		switch {
		case !dotted:
			add(where, "relation %q must be qualified, as <package>.<kind>", s.Relation)
		case pkg == p.Name:
			k, ok := p.relation(name)
			if !ok {
				add(where, "relation %q is no relation kind of this package", s.Relation)
			} else if own := (Type{Name: t.Name, Package: p.Name, Traits: t.Traits}); !relates(k.To, own) {
				add(where, "relation %q does not link to %s", s.Relation, t.Name)
			}
		case !slices.Contains(p.Depends, pkg):
			add(where, "relation %q is of package %s, which this package does not depend on", s.Relation, pkg)
		}
	}
	if a := s.Apply; a != nil {
		checkLabel(add, where+", apply", "label", a.Label)
		if len(a.Set) == 0 {
			add(where+", apply", "set needs at least one field")
		}
		for target, v := range a.Set {
			if !IsIdent(target) {
				add(where+", apply", "set names %q, which is no field name", target)
			}
			if field, ok := strings.CutPrefix(v, SuggestFieldPrefix); ok {
				if _, has := t.Field(field); !has {
					add(where+", apply", "%s takes %q, which is no field of %s", target, v, t.Name)
				}
			}
		}
	}
}

func (p Package) relation(name string) (RelationKind, bool) {
	for _, k := range p.Relations {
		if k.Name == name {
			return k, true
		}
	}
	return RelationKind{}, false
}
