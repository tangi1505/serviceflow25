// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"group24.de/itsm/internal/db/queries"
)

// Layer of a definition (architektur.md §4).
const (
	LayerBase      = "base"
	LayerExtension = "extension"
	LayerCustomer  = "customer"
)

// KindType is the definition kind of an entity type; forms, workflows and rules follow.
const KindType = "type"

// KindRelation is the definition kind of a relation kind (PL15).
const KindRelation = "relation"

// KindTrait is the definition kind of a package's own trait (PL17).
const KindTrait = "trait"

// Store validates a package against the installed types and writes it in one transaction:
// the manifest, one base-layer definition per type, and the removal of base definitions the
// new version no longer ships. Customer and extension layers are untouched (E37).
func Store(ctx context.Context, pool *pgxpool.Pool, p Package) error {
	q := queries.New(pool)
	known, err := InstalledTypes(ctx, q)
	if err != nil {
		return err
	}
	// A package may be re-installed: its own types count as known for references.
	for _, t := range p.Types {
		known[t.QualifiedName()] = true
	}
	if err := checkPrefix(p); err != nil {
		return err
	}
	if err := Validate(p, known); err != nil {
		return err
	}
	if err := checkSettingUse(ctx, q, p); err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("meta: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := q.WithTx(tx)

	label, _ := json.Marshal(p.Label)
	manifest := p
	manifest.Types = nil
	spec, _ := json.Marshal(manifest)
	if err := qtx.UpsertPackage(ctx, queries.UpsertPackageParams{
		Name: p.Name, Version: p.Version, Label: label, Spec: spec,
	}); err != nil {
		return fmt.Errorf("meta: package %s: %w", p.Name, err)
	}

	keep := make([]string, 0, len(p.Types))
	for _, t := range p.Types {
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := qtx.UpsertDefinition(ctx, queries.UpsertDefinitionParams{
			ID: id, Package: p.Name, Kind: KindType, Name: t.Name, Layer: LayerBase, Spec: b,
		}); err != nil {
			return fmt.Errorf("meta: type %s: %w", t.Name, err)
		}
		keep = append(keep, KindType+":"+t.Name)
	}
	for _, k := range p.Relations {
		b, err := json.Marshal(k)
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := qtx.UpsertDefinition(ctx, queries.UpsertDefinitionParams{
			ID: id, Package: p.Name, Kind: KindRelation, Name: k.Name, Layer: LayerBase, Spec: b,
		}); err != nil {
			return fmt.Errorf("meta: relation %s: %w", k.Name, err)
		}
		keep = append(keep, KindRelation+":"+k.Name)
	}
	for _, tr := range p.Traits {
		b, err := json.Marshal(tr)
		if err != nil {
			return err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := qtx.UpsertDefinition(ctx, queries.UpsertDefinitionParams{
			ID: id, Package: p.Name, Kind: KindTrait, Name: tr.Name, Layer: LayerBase, Spec: b,
		}); err != nil {
			return fmt.Errorf("meta: trait %s: %w", tr.Name, err)
		}
		keep = append(keep, KindTrait+":"+tr.Name)
	}
	if err := qtx.DeleteDefinitionsNotIn(ctx, queries.DeleteDefinitionsNotInParams{Package: p.Name, Keep: keep}); err != nil {
		return fmt.Errorf("meta: prune: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	DefinitionsChanged()
	return nil
}

// definitionsGen counts the changes of the definitions this process knows of (PL53): its own
// writes and the notices on the channel itsm_definitions (records.Service.WatchTypes). The
// types kept in memory are loaded anew when it moved.
var definitionsGen atomic.Int64

// DefinitionsChanged says that the definitions changed: the types are loaded anew.
func DefinitionsChanged() { definitionsGen.Add(1) }

// DefinitionsGeneration is the count of changes this process knows of.
func DefinitionsGeneration() int64 { return definitionsGen.Load() }

// InstalledTypes returns the qualified names of all installed base types.
func InstalledTypes(ctx context.Context, q *queries.Queries) (map[string]bool, error) {
	rows, err := q.ListDefinitions(ctx, KindType)
	if err != nil {
		return nil, fmt.Errorf("meta: list types: %w", err)
	}
	known := make(map[string]bool, len(rows))
	for _, r := range rows {
		known[r.Package+"."+r.Name] = true
	}
	return known, nil
}

// Traits returns the core traits followed by the installed traits of the packages.
func Traits(ctx context.Context, q *queries.Queries) ([]Trait, error) {
	rows, err := q.ListDefinitions(ctx, KindTrait)
	if err != nil {
		return nil, fmt.Errorf("meta: list traits: %w", err)
	}
	out := append([]Trait{}, CoreTraits...)
	for _, r := range rows {
		if r.Layer != LayerBase {
			continue
		}
		var tr Trait
		if err := json.Unmarshal(r.Spec, &tr); err != nil {
			return nil, fmt.Errorf("meta: trait %s.%s: %w", r.Package, r.Name, err)
		}
		tr.Package = r.Package
		out = append(out, tr)
	}
	return out, nil
}

// Types returns the effective installed types in package and name order: the base layer
// with the customer layer applied (PL43); a change that does not fit is left out. The
// extension layer comes with EX.
func Types(ctx context.Context, q *queries.Queries) ([]Type, error) {
	rows, err := q.ListDefinitions(ctx, KindType)
	if err != nil {
		return nil, fmt.Errorf("meta: list types: %w", err)
	}
	customer := map[string]CustomerLayer{}
	customerSpec := map[string][]byte{}
	for _, r := range rows {
		if r.Layer != LayerCustomer {
			continue
		}
		var l CustomerLayer
		if err := json.Unmarshal(r.Spec, &l); err != nil {
			return nil, fmt.Errorf("meta: customer layer %s.%s: %w", r.Package, r.Name, err)
		}
		customer[r.Package+"."+r.Name] = l
		customerSpec[r.Package+"."+r.Name] = r.Spec
	}
	var out []Type
	for _, r := range rows {
		if r.Layer != LayerBase {
			continue
		}
		var t Type
		if err := json.Unmarshal(r.Spec, &t); err != nil {
			return nil, fmt.Errorf("meta: type %s.%s: %w", r.Package, r.Name, err)
		}
		t.Package = r.Package
		if l, ok := customer[t.QualifiedName()]; ok {
			t, _ = ApplyCustomer(t, l)
		}
		// The hash of what the layers store: no marshalling on every read.
		t.Revision = revision(r.Spec, customerSpec[t.QualifiedName()])
		out = append(out, t)
	}
	return out, nil
}

// BaseTypes returns the installed types of the base layer alone, as the packages ship them.
func BaseTypes(ctx context.Context, q *queries.Queries) ([]Type, error) {
	rows, err := q.ListDefinitions(ctx, KindType)
	if err != nil {
		return nil, fmt.Errorf("meta: list types: %w", err)
	}
	var out []Type
	for _, r := range rows {
		if r.Layer != LayerBase {
			continue
		}
		var t Type
		if err := json.Unmarshal(r.Spec, &t); err != nil {
			return nil, fmt.Errorf("meta: type %s.%s: %w", r.Package, r.Name, err)
		}
		t.Package = r.Package
		out = append(out, t)
	}
	return out, nil
}

// checkSettingUse refuses a setting another installed package declares, and a formula that
// reads a setting neither this package nor an installed one declares (SD20).
func checkSettingUse(ctx context.Context, q *queries.Queries, p Package) error {
	rows, err := q.ListPackages(ctx)
	if err != nil {
		return fmt.Errorf("meta: list packages: %w", err)
	}
	declared := map[string]string{}
	for _, r := range rows {
		if r.Name == p.Name {
			continue
		}
		var other Package
		if err := json.Unmarshal(r.Spec, &other); err != nil {
			return fmt.Errorf("meta: package %s: %w", r.Name, err)
		}
		for _, st := range other.Settings {
			declared[st.Key] = r.Name
		}
	}
	var ps []Problem
	for _, st := range p.Settings {
		if by, ok := declared[st.Key]; ok {
			ps = append(ps, Problem{Where: "setting " + st.Key, Msg: "is declared by the package " + by})
		}
		declared[st.Key] = p.Name
	}
	for _, t := range p.Types {
		fs, err := CompileFormulas(t)
		if err != nil {
			continue // Validate reported it
		}
		for _, cf := range fs {
			for _, key := range cf.Formula.Settings {
				if _, ok := declared[key]; !ok {
					ps = append(ps, Problem{Where: "type " + t.Name + ", field " + cf.Field.Name,
						Msg: fmt.Sprintf("the formula reads the setting %q, which no package declares", key)})
				}
			}
		}
	}
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Package: p.Name, Problems: ps}
}
