// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Live (UI8, Hauptplan E130): one connection per tab to the event stream (SSE, PL24); pages
// subscribe to what concerns them. The browser reconnects on its own and sends the last id,
// so the server replays what was missed. Without EventSource (tests) nothing streams.
import { createContext, useContext, useEffect, useRef, type ReactNode } from "react";

// LiveEvent is an event as the stream sends it; mine marks a record where the session's
// person or one of their groups holds a role.
export type LiveEvent = {
  position: number;
  kind: string;
  type_name: string;
  record_id: string;
  actor?: string;
  payload?: Record<string, unknown>;
  mine?: boolean;
};

type Listener = (e: LiveEvent) => void;

// The kinds of events the stream names; each comes as a named SSE event.
const kinds = [
  "create",
  "update",
  "transition",
  "trash",
  "restore",
  "purge",
  "comment",
  "breach",
  "escalation",
  "notify",
  "relate",
  "unrelate",
  "attach",
  "detach",
  "cost_add",
  "cost_change",
  "cost_remove",
];

const LiveContext = createContext<Set<Listener> | null>(null);

export function LiveProvider({ children }: { children: ReactNode }) {
  const listeners = useRef(new Set<Listener>());
  useEffect(() => {
    if (typeof EventSource === "undefined") {
      return;
    }
    const source = new EventSource("/api/v1/events/stream");
    const deliver = (msg: MessageEvent<string>) => {
      let e: LiveEvent;
      try {
        e = JSON.parse(msg.data) as LiveEvent;
      } catch {
        return;
      }
      for (const fn of listeners.current) {
        fn(e);
      }
    };
    for (const kind of kinds) {
      source.addEventListener(kind, deliver);
    }
    return () => source.close();
  }, []);
  return <LiveContext.Provider value={listeners.current}>{children}</LiveContext.Provider>;
}

// useLive calls fn for every event while the component lives; fn may change between renders.
export function useLive(fn: Listener) {
  const listeners = useContext(LiveContext);
  const current = useRef(fn);
  current.current = fn;
  useEffect(() => {
    if (!listeners) {
      return;
    }
    const call: Listener = (e) => current.current(e);
    listeners.add(call);
    return () => {
      listeners.delete(call);
    };
  }, [listeners]);
}
