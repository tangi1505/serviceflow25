// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"group24.de/itsm/internal/authz"
	"group24.de/itsm/internal/events"
	"group24.de/itsm/internal/httpapi/gen"
	"group24.de/itsm/internal/notify"
	"group24.de/itsm/internal/records"
)

// --- the event stream (PL24) ---
//
// A session sees the events of records it may read (E87); read on core.event sees them all.
// The header ITSM-Last-Position tells the caller where to continue, also when every event
// of the page was hidden.

const lastPositionHeader = "ITSM-Last-Position"

// heartbeat keeps proxies and browsers from closing an idle SSE connection.
const heartbeat = 15 * time.Second

func (s *server) eventsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Events == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "events are not configured on this instance", nil)
		return false
	}
	return true
}

func (s *server) ListEvents(w http.ResponseWriter, r *http.Request, p gen.ListEventsParams) {
	if !s.eventsReady(w, r) {
		return
	}
	var after int64
	if p.After != nil {
		after = *p.After
	}
	limit := 100
	if p.Limit != nil {
		limit = *p.Limit
	}
	list, last, _, err := s.o.Events.Stream(r.Context(), after, limit)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	w.Header().Set(lastPositionHeader, strconv.FormatInt(last, 10))
	writeJSON(w, http.StatusOK, list)
}

func (s *server) GetEventPosition(w http.ResponseWriter, r *http.Request) {
	if !s.eventsReady(w, r) {
		return
	}
	n, err := s.o.Events.Latest(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"position": n})
}

// StreamEvents is Server-Sent Events: a replay from `after` (or the browser's Last-Event-ID
// on reconnect), then live through Valkey. Without a position the stream starts now. Every
// message carries the position as id, the kind as event and the event as JSON data.
func (s *server) StreamEvents(w http.ResponseWriter, r *http.Request, p gen.StreamEventsParams) {
	if !s.eventsReady(w, r) {
		return
	}
	ctx := r.Context()
	after, replay := int64(-1), false
	if p.After != nil {
		after, replay = *p.After, true
	}
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			after, replay = n, true
		}
	}
	if !replay {
		n, err := s.o.Events.Latest(ctx)
		if err != nil {
			writeError(w, r, s.o.Logger, err)
			return
		}
		after = n
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeProblem(w, r, http.StatusInternalServerError, "internal", "streaming is not supported here", nil)
		return
	}
	// Live events arrive while the replay runs; the buffer bridges the gap. Should it
	// overflow, the client is told to reconnect, and Last-Event-ID replays what it missed.
	live := make(chan records.Event, 1024)
	overflow := make(chan struct{}, 1)
	// The inbox of the session's person (WF19): the number of the unread when it changes.
	inbox := make(chan int, 16)
	if acc := authz.From(ctx); s.o.Notify != nil && acc != nil && acc.PersonID != nil {
		person := *acc.PersonID
		go func() {
			_ = s.o.Notify.SubscribeInbox(ctx, func(n notify.InboxNotice) {
				if n.Person != person {
					return
				}
				select {
				case inbox <- n.Unread:
				default:
				}
			})
		}()
	}
	go func() {
		_ = s.o.Events.Subscribe(ctx, func(e records.Event) {
			select {
			case live <- e:
			default:
				select {
				case overflow <- struct{}{}:
				default:
				}
			}
		})
	}()

	// No write deadline: the connection lives as long as the client listens.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	send := func(e records.Event) {
		raw, _ := json.Marshal(e)
		fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", e.Position, e.Kind, raw)
	}
	if replay {
		for {
			list, last, more, err := s.o.Events.Stream(ctx, after, 500)
			if err != nil {
				if errors.Is(err, events.ErrGone) {
					fmt.Fprintf(w, "event: gone\ndata: %s\n\n", strconv.Quote(err.Error()))
				}
				flusher.Flush()
				return
			}
			for _, e := range list {
				// The replay is filtered already; mine is per session (UI8).
				_, e.Mine = s.o.Records.SeeEvent(ctx, e)
				send(e)
			}
			after = last
			if !more {
				break
			}
		}
	}
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	tick := time.NewTicker(heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-overflow:
			fmt.Fprint(w, "event: overflow\ndata: \"reconnect with Last-Event-ID\"\n\n")
			flusher.Flush()
			return
		case e := <-live:
			if e.Position <= after {
				continue
			}
			after = e.Position
			if visible, mine := s.o.Records.SeeEvent(ctx, e); visible {
				e.Mine = mine
				send(e)
				flusher.Flush()
			}
		case n := <-inbox:
			// No id: the position of the stream stays where it is.
			fmt.Fprintf(w, "event: inbox\ndata: {\"unread\":%d}\n\n", n)
			flusher.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// --- the dispatcher's consumers and what failed for good (PL41) ---
//
// Both are matters of operation, readable with "read" on core.job like the job log.

func (s *server) opsReady(w http.ResponseWriter, r *http.Request) bool {
	if s.o.Events == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "unavailable", "the event stream is not configured on this instance", nil)
		return false
	}
	if a := authz.From(r.Context()); !a.Allowed(jobsType, authz.ActionRead) {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "this needs the right read on core.job", nil)
		return false
	}
	return true
}

func (s *server) ListEventConsumers(w http.ResponseWriter, r *http.Request) {
	if !s.opsReady(w, r) {
		return
	}
	// The consumers serve every tenant: in a tenant only with read on core.job at the root (PL45).
	if a := authz.From(r.Context()); a.Bounded() && !a.Installation(jobsType, authz.ActionRead) {
		writeProblem(w, r, http.StatusForbidden, "forbidden", "the consumers serve every tenant: this needs read on core.job at the root", nil)
		return
	}
	list, err := s.o.Events.Consumers(r.Context())
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *server) ListDeliveryFailures(w http.ResponseWriter, r *http.Request, p gen.ListDeliveryFailuresParams) {
	if !s.opsReady(w, r) {
		return
	}
	var before int64
	if p.Before != nil {
		before = *p.Before
	}
	limit := 0
	if p.Limit != nil {
		limit = *p.Limit
	}
	list, err := s.o.Events.Failures(r.Context(), before, limit)
	if err != nil {
		writeError(w, r, s.o.Logger, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
