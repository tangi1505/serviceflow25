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
	"errors"
	"net/http"
	"net/http/httptest"

	"group24.de/itsm/internal/dashboard"
	"group24.de/itsm/internal/httpapi/gen"
)

// Dashboards (UI13): a dashboard with its widgets, saving all at once, the data of a widget.

func (s *server) dashboardsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Dashboards == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "dashboards are not configured", nil)
		return false
	}
	return true
}

func (s *server) GetDashboard(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	v, err := s.o.Dashboards.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *server) SaveDashboard(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	var body struct {
		Version int64            `json:"version"`
		Fields  map[string]any   `json:"fields"`
		Widgets []map[string]any `json:"widgets"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, err := s.o.Dashboards.Save(r.Context(), id, body.Version, body.Fields, body.Widgets, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// CopyDashboard copies a dashboard the session reads into a new one of its own (UI14a).
func (s *server) CopyDashboard(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, err := s.o.Dashboards.Copy(r.Context(), id, body.Name, actor(r))
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *server) WidgetData(w http.ResponseWriter, r *http.Request, id gen.Id, widgetID gen.WidgetId) {
	if !s.dashboardsReady(w, r) {
		return
	}
	var f dashboard.Filters
	if !decode(w, r, &f) {
		return
	}
	d, err := s.o.Dashboards.Data(r.Context(), id, widgetID, f)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// DashboardData streams all widgets of a dashboard (PL67, your decision 2026-11-17: piece by
// piece): one JSON line per widget as soon as it is computed, so that the quick numbers show
// while a long group still counts.
func (s *server) DashboardData(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	var f dashboard.Filters
	if !decode(w, r, &f) {
		return
	}
	type line struct {
		Widget  string          `json:"widget"`
		Data    *dashboard.Data `json:"data,omitempty"`
		Problem json.RawMessage `json:"problem,omitempty"`
	}
	started := false
	start := func() {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			started = true
		}
	}
	rc := http.NewResponseController(w)
	enc := json.NewEncoder(w)
	err := s.o.Dashboards.Bundle(r.Context(), id, f, func(widgetID string, d dashboard.Data, err error) {
		if errors.Is(r.Context().Err(), context.Canceled) {
			// Nobody reads any more (PL65): the period changed or the page closed. Out of
			// time (PL56) the widgets still left get their problem.
			return
		}
		start()
		l := line{Widget: widgetID}
		if err != nil {
			// The problem of the widget as the endpoint of the widget would answer it.
			p := httptest.NewRecorder()
			writeError(p, r, s.o.Logger, err)
			l.Problem = p.Body.Bytes()
		} else {
			l.Data = &d
		}
		_ = enc.Encode(l)
		_ = rc.Flush()
	})
	switch {
	case err != nil && !started:
		writeError(w, r, s.o.Logger, err)
	case r.Context().Err() != nil && !started:
		// Cancelled (499) or out of time (503) before the first widget.
		writeError(w, r, s.o.Logger, r.Context().Err())
	case r.Context().Err() != nil:
		s.o.Logger.Info("request ended while streaming", "method", r.Method, "path", r.URL.Path, "reason", r.Context().Err().Error())
	default:
		start()
	}
}

func (s *server) SidebarWidgets(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	list, err := s.o.Dashboards.Sidebar(r.Context(), typeName, id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) SidebarWidgetData(w http.ResponseWriter, r *http.Request, typeName gen.TypeName, id gen.Id, widgetID gen.WidgetId) {
	if !s.dashboardsReady(w, r) {
		return
	}
	d, err := s.o.Dashboards.SidebarData(r.Context(), typeName, id, widgetID)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *server) TileData(w http.ResponseWriter, r *http.Request, id gen.Id) {
	if !s.dashboardsReady(w, r) {
		return
	}
	d, err := s.o.Dashboards.TileData(r.Context(), id)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
