// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"net/http"
	"time"

	"group24.de/itsm/internal/calendar"
	"group24.de/itsm/internal/httpapi/gen"
)

// --- calendars of working time (WF8) ---
//
// The calendars are records of the core package; reading one needs read on core.calendar (an
// invisible calendar is 404), adding holidays needs create on core.calendar_holiday.

func (s *server) calendarsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Calendars == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "calendars are not configured on this instance", nil)
		return false
	}
	return true
}

// calendarError answers a calendar that does not make one (bad hours, unknown time zone, no
// working time at all) with 422, the rest as usual.
func (s *server) calendarError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, calendar.ErrInvalid) || errors.Is(err, calendar.ErrNoHours) {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
		return
	}
	writeError(w, r, s.o.Logger, err)
}

func (s *server) CalendarAdd(w http.ResponseWriter, r *http.Request, id gen.CalendarId) {
	var body gen.CalendarAddJSONBody
	if !s.calendarsReady(w, r) || !decode(w, r, &body) {
		return
	}
	if body.Minutes < 0 {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "minutes must not be negative", nil)
		return
	}
	c, err := s.o.Calendars.Load(r.Context(), id)
	if err != nil {
		s.calendarError(w, r, err)
		return
	}
	end, err := c.Add(body.Start, time.Duration(body.Minutes)*time.Minute)
	if err != nil {
		s.calendarError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"end": end.Format(time.RFC3339)})
}

func (s *server) CalendarBetween(w http.ResponseWriter, r *http.Request, id gen.CalendarId) {
	var body gen.CalendarBetweenJSONBody
	if !s.calendarsReady(w, r) || !decode(w, r, &body) {
		return
	}
	c, err := s.o.Calendars.Load(r.Context(), id)
	if err != nil {
		s.calendarError(w, r, err)
		return
	}
	d, err := c.Between(body.From, body.To)
	if err != nil {
		s.calendarError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"minutes": d.Minutes()})
}

func (s *server) CalendarHolidays(w http.ResponseWriter, r *http.Request, id gen.CalendarId) {
	var body gen.CalendarHolidaysJSONBody
	if !s.calendarsReady(w, r) || !decode(w, r, &body) {
		return
	}
	n, err := s.o.Calendars.GenerateHolidays(r.Context(), id, body.Year, string(body.State), actor(r))
	if err != nil {
		s.calendarError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": n})
}
