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

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/webhooks"
)

// --- webhooks (PL25, KF13, Hauptplan E100) ---
//
// Reading the subscriptions needs "read" on the type "core.webhook", changing them
// "manage_webhooks" (admin has both through * *). A subscription belongs to the account that
// creates it; its rights decide which events are sent.

const webhooksType = "core.webhook"

func (s *server) webhooksReady(w http.ResponseWriter, r *http.Request, action string) bool {
	if s.o.Webhooks == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "webhooks are not configured on this instance", nil)
		return false
	}
	if a := authz.From(r.Context()); !a.Allowed(webhooksType, action) {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "webhooks need the right "+action+" on core.webhook", nil)
		return false
	}
	return true
}

func (s *server) webhookError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, webhooks.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, webhooks.ErrInvalid):
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", err.Error(), nil)
	default:
		writeError(w, r, s.o.Logger, err)
	}
}

func (s *server) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	if !s.webhooksReady(w, r, "read") {
		return
	}
	list, err := s.o.Webhooks.List(r.Context())
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.webhooksReady(w, r, "manage_webhooks") {
		return
	}
	var in webhooks.Input
	if !decode(w, r, &in) {
		return
	}
	owner := actor(r)
	if owner == nil {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "a webhook needs a signed-in owner", nil)
		return
	}
	hook, err := s.o.Webhooks.Create(r.Context(), in, *owner, owner)
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, hook)
}

func (s *server) GetWebhook(w http.ResponseWriter, r *http.Request, id gen.WebhookId) {
	if !s.webhooksReady(w, r, "read") {
		return
	}
	hook, err := s.o.Webhooks.Get(r.Context(), id)
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (s *server) UpdateWebhook(w http.ResponseWriter, r *http.Request, id gen.WebhookId) {
	if !s.webhooksReady(w, r, "manage_webhooks") {
		return
	}
	var in webhooks.Input
	if !decode(w, r, &in) {
		return
	}
	hook, err := s.o.Webhooks.Update(r.Context(), id, in, actor(r))
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (s *server) DeleteWebhook(w http.ResponseWriter, r *http.Request, id gen.WebhookId) {
	if !s.webhooksReady(w, r, "manage_webhooks") {
		return
	}
	if err := s.o.Webhooks.Delete(r.Context(), id); err != nil {
		s.webhookError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) RotateWebhookSecret(w http.ResponseWriter, r *http.Request, id gen.WebhookId) {
	if !s.webhooksReady(w, r, "manage_webhooks") {
		return
	}
	hook, err := s.o.Webhooks.Rotate(r.Context(), id, actor(r))
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (s *server) WebhookDeliveries(w http.ResponseWriter, r *http.Request, id gen.WebhookId, p gen.WebhookDeliveriesParams) {
	if !s.webhooksReady(w, r, "read") {
		return
	}
	limit := 0
	if p.Limit != nil {
		limit = *p.Limit
	}
	list, err := s.o.Webhooks.Deliveries(r.Context(), id, limit)
	if err != nil {
		s.webhookError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
