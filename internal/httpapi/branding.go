// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"errors"
	"io"
	"net/http"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/branding"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/org"
	"group24.de/itsm/internal/records"
)

// The appearance of the organisation (UI39, UI-Plan E51): the sign-in page reads the one of the
// root without a session, the workplace the one of its tenant; the page Erscheinungsbild
// changes the settings through the settings of the node and the logo here.

func (s *server) brandingReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Branding == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the appearance is not configured", nil)
		return false
	}
	return true
}

// brandingOut is the appearance as the API shows it; the logo addresses carry their hash.
func brandingOut(b branding.Branding, base string) gen.Branding {
	out := gen.Branding{Name: b.Name, Accent: b.Accent, PrimaryAccent: b.PrimaryAccent}
	for _, v := range branding.Variants {
		sha, ok := b.Logos[v]
		if !ok {
			continue
		}
		u := base + "/" + v + "?v=" + sha
		if v == "light" {
			out.LogoLight = &u
		} else {
			out.LogoDark = &u
		}
	}
	return out
}

func (s *server) writeBranding(w http.ResponseWriter, r *http.Request, node, base string) {
	b, err := s.o.Branding.Of(r.Context(), node)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, brandingOut(b, base))
}

// writeLogo serves an image only as an image: its own type, never sniffed, and a sandbox, so an
// SVG opened on its own runs no script.
func (s *server) writeLogo(w http.ResponseWriter, r *http.Request, node, variant string, version *string) {
	l, err := s.o.Branding.Logo(r.Context(), node, variant)
	if errors.Is(err, branding.ErrNotFound) {
		writeProblem(w, r, http.StatusNotFound, "not_found", "there is no logo for "+variant, nil)
		return
	}
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", l.ContentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	if version != nil && *version == l.Sha {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(l.Data)
}

// GetBranding is the appearance of the root for the sign-in page.
func (s *server) GetBranding(w http.ResponseWriter, r *http.Request) {
	if !s.brandingReady(w, r) {
		return
	}
	s.writeBranding(w, r, org.Root, "/api/v1/branding/logo")
}

// GetBrandingLogo is a logo of the root for the sign-in page.
func (s *server) GetBrandingLogo(w http.ResponseWriter, r *http.Request, variant gen.GetBrandingLogoParamsVariant, p gen.GetBrandingLogoParams) {
	if !s.brandingReady(w, r) {
		return
	}
	s.writeLogo(w, r, org.Root, string(variant), p.V)
}

// GetMyBranding is the appearance of the session's tenant.
func (s *server) GetMyBranding(w http.ResponseWriter, r *http.Request) {
	if !s.brandingReady(w, r) {
		return
	}
	s.writeBranding(w, r, authz.From(r.Context()).CurrentTenant(), "/api/v1/me/branding/logo")
}

// GetMyBrandingLogo is a logo of the session's tenant.
func (s *server) GetMyBrandingLogo(w http.ResponseWriter, r *http.Request, variant gen.GetMyBrandingLogoParamsVariant, p gen.GetMyBrandingLogoParams) {
	if !s.brandingReady(w, r) {
		return
	}
	s.writeLogo(w, r, authz.From(r.Context()).CurrentTenant(), string(variant), p.V)
}

// appearanceNode lets only the root or a tenant node have an appearance (E51).
func (s *server) appearanceNode(w http.ResponseWriter, r *http.Request, path string) bool {
	if path == org.Root {
		return true
	}
	n, err := s.o.Org.Node(r.Context(), path)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	if !n.Tenant {
		writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "only the root or a tenant node has a logo: it holds for the whole tenant", nil)
		return false
	}
	return true
}

// SetOrgLogo keeps the logo of a variant at the root or a tenant node.
func (s *server) SetOrgLogo(w http.ResponseWriter, r *http.Request, path gen.OrgPath, variant gen.SetOrgLogoParamsVariant) {
	if !s.mayOrg(w, r, "manage_settings", path) || !s.brandingReady(w, r) || !s.appearanceNode(w, r, path) {
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, branding.MaxLogo+1))
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "bad_request", "the image could not be read", nil)
		return
	}
	if _, err := s.o.Branding.SetLogo(r.Context(), path, string(variant), data, actor(r)); err != nil {
		if errors.Is(err, branding.ErrInvalid) {
			writeProblem(w, r, http.StatusUnprocessableEntity, "validation", "", []records.Problem{{Field: "logo_" + string(variant), Code: "invalid", Message: err.Error()}})
			return
		}
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.changed(r, "branding.logo.set", path+" "+string(variant), nil, map[string]any{"bytes": len(data)}, nil)
	s.writeBranding(w, r, path, "/api/v1/me/branding/logo")
}

// RemoveOrgLogo takes the node's own logo of a variant away.
func (s *server) RemoveOrgLogo(w http.ResponseWriter, r *http.Request, path gen.OrgPath, variant gen.RemoveOrgLogoParamsVariant) {
	if !s.mayOrg(w, r, "manage_settings", path) || !s.brandingReady(w, r) || !s.appearanceNode(w, r, path) {
		return
	}
	if err := s.o.Branding.RemoveLogo(r.Context(), path, string(variant)); err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	s.changed(r, "branding.logo.remove", path+" "+string(variant), nil, nil, nil)
	w.WriteHeader(http.StatusNoContent)
}
