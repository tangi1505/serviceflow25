// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"group24.de/itsm/internal/records"
)

// The types of the core package and the setting of the organisation tree.
const (
	TypeCalendar   = "core.calendar"
	TypeHours      = "core.calendar_hours"
	TypeHoliday    = "core.calendar_holiday"
	SettingDefault = "calendar.default"
)

// ErrInvalid says a calendar's records do not make a calendar (a time that is no time, an
// unknown time zone); the message names the record.
var ErrInvalid = errors.New("calendar: invalid")

var weekdays = map[string]time.Weekday{"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday}

// Settings reads a setting of a node (org.Service.Setting).
type Settings func(ctx context.Context, path, key string) (json.RawMessage, string, error)

// Service loads calendars from records with the rights of the context.
type Service struct {
	rec      *records.Service
	settings Settings
}

// New wires the service.
func New(rec *records.Service, settings Settings) *Service {
	return &Service{rec: rec, settings: settings}
}

// Load reads a calendar with its hours and holidays.
func (s *Service) Load(ctx context.Context, id uuid.UUID) (*Calendar, error) {
	rec, err := s.rec.Get(ctx, TypeCalendar, id)
	if err != nil {
		return nil, err
	}
	tz, _ := rec["timezone"].(string)
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("%w: time zone %q of calendar %s is unknown", ErrInvalid, tz, id)
	}
	c := &Calendar{Location: loc, Hours: map[time.Weekday][]Span{}, Holidays: map[string]bool{}}
	hours, err := s.all(ctx, TypeHours, id)
	if err != nil {
		return nil, err
	}
	for _, h := range hours {
		day, ok := weekdays[fmt.Sprint(h["weekday"])]
		from, err1 := ParseClock(fmt.Sprint(h["starts"]))
		to, err2 := ParseClock(fmt.Sprint(h["ends"]))
		if !ok || err1 != nil || err2 != nil || to <= from {
			return nil, fmt.Errorf("%w: working hours %v of calendar %s: %q to %q on %v", ErrInvalid, h["id"], id, h["starts"], h["ends"], h["weekday"])
		}
		c.Hours[day] = append(c.Hours[day], Span{From: from, To: to})
	}
	days, err := s.all(ctx, TypeHoliday, id)
	if err != nil {
		return nil, err
	}
	for _, d := range days {
		if date, ok := d["date"].(string); ok {
			c.Holidays[date] = true
		}
	}
	return c, nil
}

// ForNode reads the calendar the setting calendar.default gives a node of the organisation
// tree; without the setting there is none (nil, uuid.Nil).
func (s *Service) ForNode(ctx context.Context, path string) (*Calendar, uuid.UUID, error) {
	raw, _, err := s.settings(ctx, path, SettingDefault)
	if err != nil || raw == nil {
		return nil, uuid.Nil, err
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return nil, uuid.Nil, fmt.Errorf("%w: setting %s is no calendar id", ErrInvalid, SettingDefault)
	}
	id, err := uuid.Parse(str)
	if err != nil {
		return nil, uuid.Nil, fmt.Errorf("%w: setting %s is no calendar id", ErrInvalid, SettingDefault)
	}
	c, err := s.Load(ctx, id)
	return c, id, err
}

// GenerateHolidays adds the public holidays of a German state in a year to a calendar; days it
// has already stay as they are. It returns how many it added.
func (s *Service) GenerateHolidays(ctx context.Context, id uuid.UUID, year int, state string, actor records.Actor) (int, error) {
	list, err := GermanHolidays(year, state)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if _, err := s.rec.Get(ctx, TypeCalendar, id); err != nil {
		return 0, err
	}
	existing, err := s.all(ctx, TypeHoliday, id)
	if err != nil {
		return 0, err
	}
	have := map[string]bool{}
	for _, d := range existing {
		if date, ok := d["date"].(string); ok {
			have[date] = true
		}
	}
	added := 0
	for _, h := range list {
		date := h.Date.Format(time.DateOnly)
		if have[date] {
			continue
		}
		if _, err := s.rec.Create(ctx, TypeHoliday, map[string]any{"calendar": id.String(), "date": date, "name": h.Name}, actor); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// all reads every record of a type that belongs to the calendar.
func (s *Service) all(ctx context.Context, typeName string, id uuid.UUID) ([]records.Record, error) {
	var out []records.Record
	after := ""
	for {
		page, err := s.rec.List(ctx, typeName, records.ListOptions{Limit: 500, After: after, Filter: map[string]string{"calendar": id.String()}})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if page.Next == "" {
			return out, nil
		}
		after = page.Next
	}
}
