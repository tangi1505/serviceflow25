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
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/db/queries"
	"group24.de/itsm/internal/meta"
)

// --- costs and times (KF18; Hauptplan E97; architektur.md B1; PL20) ---
//
// A type with the trait costs has cost entries in the core table cost: time (minutes at an
// hourly rate), fixed or material. The sums are the computed fields time_spent and
// cost_total on the record's row. Every write of an entry locks the record's row, writes
// the entry, recomputes both sums from the table and leaves an audit row and an event, all
// in one transaction. The record's version stays: booking time must not break the edit of
// someone who has the record open (If-Match). Reading needs read on the record, writing
// needs update on it.

// Kinds of cost entries (GLPI CommonITILCost: time, fixed, material).
const (
	CostTime     = "time"
	CostFixed    = "fixed"
	CostMaterial = "material"
)

// Settings of the costs (E97): the currency of the installation (root node) and the hourly
// rate a time entry takes when it names none (the record's node, inherited upwards).
const (
	SettingCurrency   = "costs.currency"
	SettingHourlyRate = "costs.hourly_rate"
	DefaultCurrency   = "EUR"
)

// Cost is one entry as the API shows it. Minutes and rate belong to time entries; their
// amount is minutes × rate / 60, rounded to cents.
type Cost struct {
	ID          uuid.UUID  `json:"id"`
	Kind        string     `json:"kind"`
	Description *string    `json:"description"`
	SpentOn     string     `json:"spent_on"`
	Person      *uuid.UUID `json:"person"`
	Minutes     *int32     `json:"minutes,omitempty"`
	Rate        *float64   `json:"rate,omitempty"`
	Amount      float64    `json:"amount"`
	Version     int64      `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   *uuid.UUID `json:"updated_by,omitempty"`
}

// CostList is the entries of a record with their sums and the currency.
type CostList struct {
	Currency  string  `json:"currency"`
	TimeSpent float64 `json:"time_spent"`
	CostTotal float64 `json:"cost_total"`
	Entries   []Cost  `json:"entries"`
}

// costFields are the writable fields of an entry, checked with the rules of record fields.
var (
	minuteMin, minuteMax = 1.0, 525600.0 // one minute to one year
	moneyMin             = 0.0
	rateMax              = 9999999999.99
	amountMax            = 999999999999.99
	costFields           = []meta.Field{
		{Name: "description", Type: meta.FieldText, MaxLength: 1000},
		{Name: "spent_on", Type: meta.FieldDate},
		{Name: "person", Type: meta.FieldReference, To: "core.person"},
		{Name: "minutes", Type: meta.FieldNumber, Min: &minuteMin, Max: &minuteMax},
		{Name: "rate", Type: meta.FieldNumber, Min: &moneyMin, Max: &rateMax},
		{Name: "amount", Type: meta.FieldNumber, Min: &moneyMin, Max: &amountMax},
	}
)

// costType is a type with the trait costs.
func (s *Service) costType(ctx context.Context, typeName string) (meta.Type, error) {
	t, err := s.Type(ctx, typeName)
	if err != nil {
		return meta.Type{}, err
	}
	if !t.HasTrait(meta.TraitCosts) {
		return meta.Type{}, fmt.Errorf("%s has no costs (trait costs): %w", typeName, ErrNotFound)
	}
	return t, nil
}

// Costs lists the entries of a record, by day of work, with their sums.
func (s *Service) Costs(ctx context.Context, typeName string, id uuid.UUID) (CostList, error) {
	if _, err := s.costType(ctx, typeName); err != nil {
		return CostList{}, err
	}
	if _, err := s.Get(ctx, typeName, id); err != nil {
		return CostList{}, err
	}
	rows, err := queries.New(s.pool).CostsOfRecord(ctx, queries.CostsOfRecordParams{TypeName: typeName, RecordID: id})
	if err != nil {
		return CostList{}, fmt.Errorf("records: costs: %w", err)
	}
	list := CostList{Currency: s.currency(ctx), Entries: make([]Cost, 0, len(rows))}
	for _, row := range rows {
		c := costFromRow(queries.GetCostRow(row))
		if c.Minutes != nil {
			list.TimeSpent += float64(*c.Minutes)
		}
		list.CostTotal += c.Amount
		list.Entries = append(list.Entries, c)
	}
	list.CostTotal = cents(list.CostTotal)
	return list, nil
}

// AddCost books an entry on a record.
func (s *Service) AddCost(ctx context.Context, typeName string, id uuid.UUID, input map[string]any, actor Actor) (Cost, error) {
	t, err := s.costType(ctx, typeName)
	if err != nil {
		return Cost{}, err
	}
	var out Cost
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.costRecord(ctx, tx, t, id)
		if err != nil {
			return err
		}
		c, err := s.costValues(ctx, rec, Cost{}, input, true)
		if err != nil {
			return err
		}
		if c.ID, err = uuid.NewV7(); err != nil {
			return err
		}
		q := queries.New(tx)
		p := queries.InsertCostParams{ID: c.ID, TypeName: typeName, RecordID: id, Kind: c.Kind, Amount: c.Amount, CreatedBy: nullActor(actor)}
		p.Description, p.SpentOn, p.Person, p.Minutes, p.Rate = costColumns(c)
		if err := q.InsertCost(ctx, p); err != nil {
			return fmt.Errorf("records: add cost: %w", err)
		}
		if out, err = getCost(ctx, q, typeName, id, c.ID); err != nil {
			return err
		}
		return s.costWritten(ctx, tx, t, id, rec, "cost_add", nil, &out, actor)
	})
	return out, s.written(ctx, typeName, err)
}

// UpdateCost changes the given fields of an entry that still has expectedVersion. The kind
// stays; a time entry's amount follows its minutes and rate.
func (s *Service) UpdateCost(ctx context.Context, typeName string, id, costID uuid.UUID, expectedVersion int64, patch map[string]any, actor Actor) (Cost, error) {
	t, err := s.costType(ctx, typeName)
	if err != nil {
		return Cost{}, err
	}
	var out Cost
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.costRecord(ctx, tx, t, id)
		if err != nil {
			return err
		}
		q := queries.New(tx)
		cur, err := getCost(ctx, q, typeName, id, costID)
		if err != nil {
			return err
		}
		if cur.Version != expectedVersion {
			return fmt.Errorf("%w: cost entry %s is at version %d, you sent %d", ErrConflict, costID, cur.Version, expectedVersion)
		}
		c, err := s.costValues(ctx, rec, cur, patch, false)
		if err != nil {
			return err
		}
		p := queries.UpdateCostParams{ID: costID, Version: expectedVersion, Amount: c.Amount, UpdatedBy: nullActor(actor)}
		p.Description, p.SpentOn, p.Person, p.Minutes, p.Rate = costColumns(c)
		n, err := q.UpdateCost(ctx, p)
		if err != nil {
			return fmt.Errorf("records: change cost: %w", err)
		}
		if n != 1 {
			return fmt.Errorf("%w: cost entry %s changed meanwhile", ErrConflict, costID)
		}
		if out, err = getCost(ctx, q, typeName, id, costID); err != nil {
			return err
		}
		return s.costWritten(ctx, tx, t, id, rec, "cost_change", &cur, &out, actor)
	})
	return out, s.written(ctx, typeName, err)
}

// RemoveCost deletes an entry; the history keeps what it was.
func (s *Service) RemoveCost(ctx context.Context, typeName string, id, costID uuid.UUID, actor Actor) error {
	t, err := s.costType(ctx, typeName)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rec, err := s.costRecord(ctx, tx, t, id)
		if err != nil {
			return err
		}
		q := queries.New(tx)
		cur, err := getCost(ctx, q, typeName, id, costID)
		if err != nil {
			return err
		}
		if _, err := q.DeleteCost(ctx, queries.DeleteCostParams{ID: costID, TypeName: typeName, RecordID: id}); err != nil {
			return fmt.Errorf("records: remove cost: %w", err)
		}
		return s.costWritten(ctx, tx, t, id, rec, "cost_remove", &cur, nil, actor)
	})
	return s.written(ctx, typeName, err)
}

// costRecord locks the record's row for the write of an entry and checks update on it. The
// lock serialises the writes of entries on one record, so every sum is computed after the
// entries before it are committed.
func (s *Service) costRecord(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID) (Record, error) {
	rec, err := s.read(ctx, tx, t, id, false, true)
	if err != nil {
		return nil, err
	}
	if err := checkAccess(ctx, t, rec, authz.ActionUpdate); err != nil {
		return nil, err
	}
	return rec, nil
}

// costWritten recomputes the sums on the record's row and leaves the audit row and the event
// of the entry's write. The record's version and updated_at stay.
func (s *Service) costWritten(ctx context.Context, tx pgx.Tx, t meta.Type, id uuid.UUID, rec Record, action string, prev, next *Cost, actor Actor) error {
	sql := fmt.Sprintf(`UPDATE %s SET %s = s.minutes, %s = s.amount
FROM (SELECT coalesce(sum(minutes), 0) AS minutes, coalesce(sum(amount), 0) AS amount
      FROM cost WHERE type_name = $1 AND record_id = $2) s
WHERE id = $2`, table(t), pgx.Identifier{meta.FieldTimeSpent}.Sanitize(), pgx.Identifier{meta.FieldCostTotal}.Sanitize())
	if _, err := tx.Exec(ctx, sql, t.QualifiedName(), id); err != nil {
		return fmt.Errorf("records: cost sums: %w", err)
	}
	var before, after any
	entry := next
	if prev != nil {
		before = *prev
		entry = prev
	}
	if next != nil {
		after = *next
		entry = next
	}
	q := queries.New(tx)
	version := rec["version"].(int64)
	if err := audit(ctx, q, t, id, version, action, &change{meta.TraitCosts, before, after}, actor); err != nil {
		return err
	}
	return event(ctx, q, t, id, rec, action, version, []string{meta.FieldTimeSpent, meta.FieldCostTotal},
		map[string]any{"cost": entry.ID.String()}, actor)
}

// costValues checks the input of an entry against cur (empty on create) and returns the
// entry as it will be written.
func (s *Service) costValues(ctx context.Context, rec Record, cur Cost, input map[string]any, create bool) (Cost, error) {
	var ps []Problem
	known := map[string]bool{"kind": true}
	for _, f := range costFields {
		known[f.Name] = true
	}
	for name := range input {
		if !known[name] {
			ps = append(ps, Problem{Field: name, Code: "unknown", Message: "is not a field of a cost entry"})
		}
	}
	out := cur
	if create {
		kind, given := input["kind"]
		switch k, _ := kind.(string); {
		case !given || kind == nil:
			ps = append(ps, Problem{Field: "kind", Code: "required", Message: "is required: time, fixed or material"})
		case k == CostTime || k == CostFixed || k == CostMaterial:
			out.Kind = k
		default:
			ps = append(ps, Problem{Field: "kind", Code: "choice", Message: fmt.Sprintf("%v is not one of time, fixed, material", kind)})
		}
	} else if _, given := input["kind"]; given {
		ps = append(ps, Problem{Field: "kind", Code: "read_only", Message: "cannot change; remove the entry and book a new one"})
	}
	isTime := out.Kind == CostTime
	for _, f := range costFields {
		v, given := input[f.Name]
		if !given {
			continue
		}
		cv, p := coerce(f, v)
		if p != nil {
			ps = append(ps, *p)
			continue
		}
		switch f.Name {
		case "description":
			out.Description = nil
			if cv != nil {
				d := cv.(string)
				out.Description = &d
			}
		case "spent_on":
			if cv == nil {
				ps = append(ps, Problem{Field: f.Name, Code: "required", Message: "is required"})
				continue
			}
			out.SpentOn = cv.(time.Time).Format(dateFormat)
		case "person":
			out.Person = nil
			if cv != nil {
				pid := cv.(uuid.UUID)
				ok, err := s.exists(ctx, f.To, pid)
				if err != nil {
					return Cost{}, err
				}
				if !ok {
					ps = append(ps, Problem{Field: f.Name, Code: "reference", Message: fmt.Sprintf("no %s with id %s", f.To, pid)})
					continue
				}
				out.Person = &pid
			}
		case "minutes":
			if !isTime {
				ps = append(ps, Problem{Field: f.Name, Code: "not_allowed", Message: "only time entries have minutes"})
				continue
			}
			n, _ := cv.(float64)
			if cv == nil || n != math.Trunc(n) {
				ps = append(ps, Problem{Field: f.Name, Code: "type", Message: "must be a whole number of minutes"})
				continue
			}
			m := int32(n) // at most 525600, checked above
			out.Minutes = &m
		case "rate":
			if !isTime {
				ps = append(ps, Problem{Field: f.Name, Code: "not_allowed", Message: "only time entries have an hourly rate"})
				continue
			}
			out.Rate = nil // null takes the setting costs.hourly_rate again
			if cv != nil {
				r := cents(cv.(float64))
				out.Rate = &r
			}
		case "amount":
			if isTime {
				ps = append(ps, Problem{Field: f.Name, Code: "read_only", Message: "of a time entry is minutes × rate / 60"})
				continue
			}
			if cv == nil {
				ps = append(ps, Problem{Field: f.Name, Code: "required", Message: "is required"})
				continue
			}
			out.Amount = cents(cv.(float64))
		}
	}
	if out.SpentOn == "" {
		out.SpentOn = time.Now().Format(dateFormat)
	}
	switch {
	case isTime && out.Minutes == nil:
		if _, given := input["minutes"]; !given {
			ps = append(ps, Problem{Field: "minutes", Code: "required", Message: "is required for a time entry"})
		}
	case isTime:
		if out.Rate == nil {
			r := s.hourlyRate(ctx, rec)
			out.Rate = &r
		}
		out.Amount = cents(float64(*out.Minutes) * *out.Rate / 60)
	case create && out.Kind != "":
		if _, given := input["amount"]; !given {
			ps = append(ps, Problem{Field: "amount", Code: "required", Message: "is required for a fixed or material entry"})
		}
	}
	if len(ps) > 0 {
		return Cost{}, &ValidationError{Problems: ps}
	}
	return out, nil
}

// currency is the setting costs.currency of the root node.
func (s *Service) currency(ctx context.Context) string {
	var c string
	if raw := s.setting(ctx, "root", SettingCurrency); raw != nil && json.Unmarshal(raw, &c) == nil && c != "" {
		return c
	}
	return DefaultCurrency
}

// hourlyRate is the setting costs.hourly_rate at the record's node, 0 without one.
func (s *Service) hourlyRate(ctx context.Context, rec Record) float64 {
	path, _ := rec[orgPathField].(string)
	var r float64
	if raw := s.setting(ctx, path, SettingHourlyRate); raw != nil && json.Unmarshal(raw, &r) == nil && r >= 0 {
		return cents(r)
	}
	return 0
}

func (s *Service) setting(ctx context.Context, path, key string) json.RawMessage {
	if s.Settings == nil {
		return nil
	}
	raw, _, err := s.Settings(ctx, path, key)
	if err != nil {
		return nil
	}
	return raw
}

func getCost(ctx context.Context, q *queries.Queries, typeName string, id, costID uuid.UUID) (Cost, error) {
	row, err := q.GetCost(ctx, queries.GetCostParams{ID: costID, TypeName: typeName, RecordID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Cost{}, fmt.Errorf("cost entry %s of %s %s: %w", costID, typeName, id, ErrNotFound)
		}
		return Cost{}, fmt.Errorf("records: cost: %w", err)
	}
	return costFromRow(row), nil
}

func costFromRow(r queries.GetCostRow) Cost {
	c := Cost{ID: r.ID, Kind: r.Kind, Amount: r.Amount, Version: r.Version,
		SpentOn: r.SpentOn.Time.Format(dateFormat), CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.Description.Valid {
		d := r.Description.String
		c.Description = &d
	}
	if r.Person.Valid {
		p := r.Person.UUID
		c.Person = &p
	}
	if r.Kind == CostTime {
		m := r.Minutes.Int32
		rate := r.Rate
		c.Minutes, c.Rate = &m, &rate
	}
	if r.CreatedBy.Valid {
		u := r.CreatedBy.UUID
		c.CreatedBy = &u
	}
	if r.UpdatedBy.Valid {
		u := r.UpdatedBy.UUID
		c.UpdatedBy = &u
	}
	return c
}

// costColumns are the nullable columns of an entry as sqlc wants them.
func costColumns(c Cost) (pgtype.Text, pgtype.Date, uuid.NullUUID, pgtype.Int4, pgtype.Float8) {
	var d pgtype.Text
	if c.Description != nil {
		d = pgtype.Text{String: *c.Description, Valid: true}
	}
	day, _ := time.Parse(dateFormat, c.SpentOn)
	var p uuid.NullUUID
	if c.Person != nil {
		p = uuid.NullUUID{UUID: *c.Person, Valid: true}
	}
	var m pgtype.Int4
	var r pgtype.Float8
	if c.Kind == CostTime {
		m = pgtype.Int4{Int32: *c.Minutes, Valid: true}
		r = pgtype.Float8{Float64: *c.Rate, Valid: true}
	}
	return d, pgtype.Date{Time: day, Valid: true}, p, m, r
}

func nullActor(a Actor) uuid.NullUUID {
	if a == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *a, Valid: true}
}

// cents rounds an amount of money to two decimals.
func cents(x float64) float64 { return math.Round(x*100) / 100 }
