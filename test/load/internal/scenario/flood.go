// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FloodCase is one way of flooding the server at a fixed rate (AP1, API plan §2.3).
type FloodCase struct {
	ID    string
	Title string
	// Request builds one request of the flood.
	Request RequestFunc
}

// floodNobody is a user and a client that do not exist: their checks still compute Argon2,
// as a real check does, so that a wrong name costs as much as a wrong password.
const floodNobody = "flood-nobody"

// FloodSetup prepares the five cases at base: a record to read, a session of its own for the
// flood, and an API client with a token and the role admin, so that its reads answer 200 until
// its limit. The cleanup removes the client again.
func FloodSetup(ctx context.Context, client *http.Client, base string) ([]FloodCase, func(context.Context) error, error) {
	s, err := login(ctx, client, base)
	if err != nil {
		return nil, nil, err
	}
	id, err := s.createProbe(ctx, client, base, "load AP1")
	if err != nil {
		return nil, nil, err
	}
	path := "/api/v1/records/" + probeType + "/" + id

	// An underscore, not a hyphen: the account of a client takes ^[a-z][a-z0-9_]{0,62}$, although
	// the API describes the hyphen as allowed (finding of AP1).
	clientID := fmt.Sprintf("flood_%d", time.Now().Unix())
	token, err := s.floodClient(ctx, client, base, clientID)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/v1/clients/"+clientID, nil)
		if err != nil {
			return err
		}
		s.apply(req)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("remove client %s: %s: %s", clientID, resp.Status, strings.TrimSpace(string(raw)))
		}
		return nil
	}

	loginBody, _ := json.Marshal(map[string]string{"username": floodNobody, "password": "falsch-falsch"}) //nolint:gosec // G101: a wrong password on purpose, no secret
	tokenBody := url.Values{"grant_type": {"client_credentials"}, "client_id": {floodNobody}, "client_secret": {"falsch-falsch"}}.Encode()
	cases := []FloodCase{
		{ID: "F1", Title: "Ohne Anmeldung auf einen geschützten Pfad", Request: get(path)},
		{ID: "F2", Title: "Eine Sitzung liest einen Datensatz", Request: s.get(path)},
		{ID: "F3", Title: "Ein Zugang liest einen Datensatz (Token, Limit 600 je Minute)", Request: func(ctx context.Context, base string) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		}},
		{ID: "F4", Title: "Token mit falschem Geheimnis (/auth/token)", Request: func(ctx context.Context, base string) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/auth/token", strings.NewReader(tokenBody))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return req, nil
		}},
		{ID: "F5", Title: "Anmeldung mit falschem Kennwort (/auth/login)", Request: func(ctx context.Context, base string) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/auth/login", bytes.NewReader(loginBody))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(csrfHeader, csrfValue)
			return req, nil
		}},
	}
	return cases, cleanup, nil
}

// floodClient creates the API client of the flood, gives its person the role admin and returns
// a token of it.
func (s session) floodClient(ctx context.Context, client *http.Client, base, clientID string) (string, error) {
	var created struct {
		PersonID string `json:"person_id"`
		Secret   string `json:"secret"`
	}
	if err := s.postJSON(ctx, client, base+"/api/v1/clients",
		map[string]any{"client_id": clientID, "name": "Flut-Messung AP1"}, http.StatusCreated, &created); err != nil {
		return "", err
	}
	if err := s.postJSON(ctx, client, base+"/api/v1/assignments",
		map[string]any{"role": "admin", "person_id": created.PersonID}, http.StatusCreated, nil); err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {clientID}, "client_secret": {created.Secret}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/auth/token", strings.NewReader(form))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK || json.Unmarshal(raw, &tok) != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("token of %s: %s: %s", clientID, resp.Status, strings.TrimSpace(string(raw)))
	}
	return tok.AccessToken, nil
}

// postJSON posts body as the session and reads the answer into out when it has the status want.
func (s session) postJSON(ctx context.Context, client *http.Client, u string, body any, want int, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	s.apply(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	answer, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		return fmt.Errorf("POST %s: %s: %s", u, resp.Status, strings.TrimSpace(string(answer)))
	}
	if out != nil {
		return json.Unmarshal(answer, out)
	}
	return nil
}
