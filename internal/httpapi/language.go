// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"group24.de/itsm/internal/meta"
	"group24.de/itsm/internal/records"
)

// --- languages of translated values (PL21, KF19, Hauptplan E98) ---

// withLanguage puts the language of the request on the context: Accept-Language for reading,
// Content-Language for plain values written to translatable fields, ?translations=all for
// every language at once.
func withLanguage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := records.Lang{
			Read: pickLanguage(r.Header.Get("Accept-Language")),
			All:  r.URL.Query().Get("translations") == "all",
		}
		if cl := strings.TrimSpace(r.Header.Get("Content-Language")); cl != "" {
			// An unknown language is reported by the write that uses it, field by field.
			l.Write = primaryTag(strings.Split(cl, ",")[0])
		}
		next.ServeHTTP(w, r.WithContext(records.WithLang(r.Context(), l)))
	})
}

// pickLanguage chooses the core language with the highest weight in Accept-Language (RFC 9110
// §12.5.4); German when none fits.
func pickLanguage(header string) string {
	best, bestQ := meta.DefaultLanguage, 0.0
	for _, part := range strings.Split(header, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		lang := primaryTag(tag)
		if q > bestQ && meta.IsLanguage(lang) {
			best, bestQ = lang, q
		}
	}
	return best
}

// primaryTag is the language of a tag in lower case: "en-GB" → "en".
func primaryTag(tag string) string {
	lang, _, _ := strings.Cut(strings.TrimSpace(tag), "-")
	return strings.ToLower(lang)
}

// localize puts the values of the request's language into the records and names the language
// in Content-Language; false means the error is written.
func (s *server) localize(w http.ResponseWriter, r *http.Request, typeName string, recs ...records.Record) bool {
	lang, err := s.o.Records.Localize(r.Context(), typeName, recs...)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return false
	}
	w.Header().Add("Vary", "Accept-Language")
	if !records.LangOf(r.Context()).All {
		w.Header().Set("Content-Language", lang)
	}
	return true
}
