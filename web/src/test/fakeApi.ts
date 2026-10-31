// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A fake of the server for the Vitest tests: session (PL5), types and records (PL3, PL4) with
// pages by key and version conflicts, in memory. It answers fetch(Request) as well as
// fetch(url, init), because openapi-fetch sends a Request.
import { vi } from "vitest";

import { hiddenOf } from "@/portal/conditions";

import type {
  Clock,
  Field,
  FieldStates,
  FormOffer,
  RecordData,
  TimelineItem,
  TypeDefinition,
} from "@/api/client";

// formTypeDef is the core type of the forms (UI11) as /api/v1/types gives it; tests add it to
// extraTypes when they build a form.
export const formTypeDef: TypeDefinition = {
  name: "form",
  package: "core",
  label: { de: "Formular", en: "Form" },
  title_field: "name",
  traits: ["history"],
  fields: [
    { name: "name", type: "text", label: { de: "Name" }, required: true, max_length: 150 },
    { name: "description", type: "text", label: { de: "Beschreibung" }, max_length: 2000 },
    {
      name: "target_type",
      type: "text",
      label: { de: "Legt an" },
      required: true,
      max_length: 130,
    },
    { name: "active", type: "boolean", label: { de: "Im Portal angeboten" }, default: false },
    {
      name: "definition",
      type: "text",
      label: { de: "Aufbau" },
      required: true,
      max_length: 200000,
    },
  ],
};

export const me = {
  id: "01a15a3c-8545-7691-b780-9b915e555b1c",
  username: "pascal",
  display_name: "Pascal",
  password_expired: false,
  session_expires_at: "2026-10-21T08:00:00Z",
};

export const probeFields: Field[] = [
  {
    name: "title",
    type: "text",
    label: { de: "Titel", en: "Title" },
    required: true,
    max_length: 200,
  },
  { name: "priority", type: "number", label: { de: "Priorität" }, min: 1, max: 5, default: 3 },
  { name: "active", type: "boolean", label: { de: "Aktiv" }, default: true },
  { name: "due_on", type: "date", label: { de: "Fällig am" } },
  { name: "seen_at", type: "datetime", label: { de: "Gesehen" } },
  {
    name: "kind",
    type: "choice",
    label: { de: "Art" },
    options: [
      { value: "question", label: { de: "Frage" } },
      { value: "fault", label: { de: "Störung" } },
      { value: "wish", label: { de: "Wunsch" } },
    ],
    default: "question",
  },
  { name: "parent", type: "reference", label: { de: "Übergeordnet" }, to: "probe.probe" },
];

export const probeType: TypeDefinition = {
  name: "probe",
  package: "probe",
  label: { de: "Probe", en: "Probe" },
  title_field: "title",
  fields: probeFields,
  // The record page (UI3) shows the timeline and the roles.
  traits: ["history", "timeline"],
  // In the portal (UI10): title and kind to create with.
  portal: { fields: ["title", "kind"] },
  actors: [{ name: "requester", label: { de: "Anforderer" }, to: ["core.person"] }],
  states: [
    { name: "new", label: { de: "Neu" }, initial: true },
    { name: "in_progress", label: { de: "In Arbeit" } },
    { name: "done", label: { de: "Erledigt" } },
  ],
  transitions: [
    { name: "start", label: { de: "Beginnen" }, from: ["new"], to: "in_progress" },
    { name: "finish", label: { de: "Erledigen" }, from: ["in_progress"], to: "done" },
    {
      name: "reopen",
      label: { de: "Wieder öffnen" },
      from: ["done"],
      to: "new",
      when: "record.priority <= 3",
    },
  ],
};

// matches mirrors the where of the JSON query for the operators the filters write (UI4).
function matches(rec: RecordData, where: unknown): boolean {
  if (!where || typeof where !== "object") {
    return true;
  }
  const node = where as Record<string, unknown>;
  if (Array.isArray(node["and"])) {
    return (node["and"] as unknown[]).every((w) => matches(rec, w));
  }
  if (Array.isArray(node["or"])) {
    return (node["or"] as unknown[]).some((w) => matches(rec, w));
  }
  const v = rec[String(node["field"])];
  const [op, want] = Object.entries(node).find(([k]) => k !== "field") ?? ["", undefined];
  switch (op) {
    case "eq":
      return v === want;
    case "neq":
      return v !== want;
    case "in":
      return Array.isArray(want) && want.includes(v);
    case "contains":
      return String(v ?? "")
        .toLowerCase()
        .includes(String(want).toLowerCase());
    case "starts":
      return String(v ?? "").startsWith(String(want));
    case "gte":
      return v !== null && v !== undefined && String(v) >= String(want);
    case "lte":
      return v !== null && v !== undefined && String(v) <= String(want);
    case "null":
      return v === null || v === undefined || v === "";
  }
  return false;
}

// applies mirrors the server: state first, then the one condition the probe type has.
function applies(name: string, rec: RecordData): { allowed: boolean; reason?: string; to: string } {
  const tr = probeType.transitions!.find((t) => t.name === name)!;
  if (!tr.from.includes(String(rec["state"]))) {
    return { allowed: false, reason: "state", to: tr.to };
  }
  if (tr.when && Number(rec["priority"]) > 3) {
    return { allowed: false, reason: "condition", to: tr.to };
  }
  return { allowed: true, to: tr.to };
}

export type FakeState = {
  signedIn: boolean;
  records: RecordData[];
  // newStates are the field states of a new record (WF2); a record in state "closed" is locked.
  newStates?: Partial<FieldStates>;
  // timeline, clocks and persons of the record page (UI3), by record id; privateRefused makes
  // a private entry fail with 403 like for someone without read_private.
  timeline?: Record<string, TimelineItem[]>;
  clocks?: Record<string, Clock[]>;
  persons?: Record<string, RecordData>;
  privateRefused?: boolean;
  // extraTypes come after probe in /api/v1/types (the person type for the picker, UI28).
  extraTypes?: TypeDefinition[];
  // searches are the saved searches the session may read (UI4); new ones are its own.
  searches?: RecordData[];
  // workplace false makes the session a user of the portal (UI10); personId is its person.
  workplace?: boolean;
  personId?: string;
  // settings are the person's settings (UI9); orgSettings the values at nodes, orgForbidden
  // makes setting them 403.
  settings?: Record<string, unknown>;
  orgSettings?: Record<string, Record<string, unknown>>;
  orgForbidden?: boolean;
  // others are who else has a record open (UI8), as the presence endpoint answers.
  others?: { account: string; name: string; editing: boolean; since: string; at: string }[];
  // forms are the forms offered to the session (UI11); formRecords the records of core.form.
  forms?: FormOffer[];
  formRecords?: RecordData[];
  // tenants are the tenants the session may switch between (PL44); tenant the one it is in.
  tenants?: { path: string; label: Record<string, string> }[];
  tenant?: string;
  // exportJob makes an export a job (UI5, more than 100,000 rows); jobState is its state.
  exportJob?: boolean;
  jobState?: string;
  calls: { method: string; path: string; query: URLSearchParams; body?: string }[];
};

export function makeRecords(n: number): RecordData[] {
  const out: RecordData[] = [];
  for (let i = 1; i <= n; i++) {
    const stamp = new Date(Date.UTC(2026, 9, 20, 8, 0, i)).toISOString();
    out.push({
      id: `01a15a3c-8545-7691-b780-${String(i).padStart(12, "0")}`,
      version: 1,
      state: "new",
      title: `Probe ${String(i).padStart(6, "0")}`,
      priority: 1 + (i % 5),
      active: i % 7 !== 0,
      due_on: null,
      seen_at: null,
      kind: ["question", "fault", "wish"][i % 3],
      parent: null,
      created_at: stamp,
      updated_at: stamp,
    });
  }
  return out;
}

export function newState(records = 0): FakeState {
  return { signedIn: false, records: makeRecords(records), calls: [] };
}

export function fakeApi(state: FakeState) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(new URL(String(input), "http://test"), init);
    const url = new URL(req.url);
    const method = req.method;
    const bodyText = method === "GET" || method === "HEAD" ? "" : await req.text();
    state.calls.push({ method, path: url.pathname, query: url.searchParams, body: bodyText });
    const json = (status: number, body: unknown, headers: Record<string, string> = {}) =>
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json", ...headers },
      });
    const problem = (status: number, code: string, extra: Record<string, unknown> = {}) =>
      new Response(JSON.stringify({ type: `/problems/${code}`, title: code, status, ...extra }), {
        status,
        headers: { "Content-Type": "application/problem+json" },
      });
    if (method !== "GET" && req.headers.get("X-Requested-With") !== "itsm") {
      return problem(403, "forbidden");
    }
    if (url.pathname === "/api/v1/version") {
      return json(200, { version: "test" });
    }
    if (url.pathname === "/api/v1/auth/login") {
      const body = JSON.parse(bodyText) as { username: string; password: string };
      if (body.username === "pascal" && body.password === "Secret-1!") {
        state.signedIn = true;
        return json(200, me);
      }
      return problem(401, "invalid_credentials");
    }
    if (!state.signedIn) {
      return problem(401, "unauthorized");
    }
    const session = () => ({
      ...me,
      workplace: state.workplace ?? true,
      person_id: state.personId,
      tenant: state.tenant ?? "root",
      tenants: state.tenants,
    });
    if (url.pathname === "/api/v1/auth/me") {
      return json(200, session());
    }
    if (url.pathname === "/api/v1/auth/context" && method === "POST") {
      const body = JSON.parse(bodyText) as { tenant?: string };
      if (body.tenant && !(state.tenants ?? []).some((t) => t.path === body.tenant)) {
        return problem(422, "validation");
      }
      state.tenant = body.tenant ?? state.tenant;
      return json(200, session());
    }
    if (url.pathname === "/api/v1/auth/logout") {
      state.signedIn = false;
      return new Response(null, { status: 204 });
    }
    if (url.pathname === "/api/v1/types") {
      return json(200, [probeType, ...(state.extraTypes ?? [])]);
    }
    if (url.pathname.endsWith(":field-states") && !url.pathname.includes("probe.probe")) {
      // Other types have no field states here.
      return json(200, {
        state: "",
        required: [],
        hidden: [],
        read_only: [],
        locked: false,
        defaults: {},
      });
    }
    if (url.pathname === "/api/v1/records/probe.probe:field-states" && method === "POST") {
      const body = JSON.parse(bodyText) as { id?: string };
      const rec = body.id ? state.records.find((r) => r["id"] === body.id) : undefined;
      if (body.id && !rec) {
        return problem(404, "not_found");
      }
      const locked = rec?.["state"] === "closed";
      const fields = probeFields.map((f) => f.name);
      return json(200, {
        state: String(rec?.["state"] ?? "new"),
        required: [],
        hidden: [],
        read_only: locked ? fields : [],
        locked,
        defaults: {},
        // A new record lands at the root; one node, no choice (PL46).
        ...(rec
          ? {}
          : { node: "root", nodes: [{ path: "root", label: { de: "Wurzel", en: "Root" } }] }),
        ...(rec ? {} : state.newStates),
      });
    }
    if (url.pathname === "/api/v1/search") {
      // The persons whose name contains the text, as the full-text search finds them.
      const q = (url.searchParams.get("q") ?? "").toLowerCase();
      const hits = Object.values(state.persons ?? {})
        .filter((p) =>
          `${String(p["first_name"])} ${String(p["last_name"])}`.toLowerCase().includes(q),
        )
        .map((p) => ({
          type: "core.person",
          id: String(p["id"]),
          title: String(p["last_name"]),
          rank: 1,
          snippet: "",
        }));
      return json(200, hits);
    }
    if (url.pathname === "/api/v1/me/settings") {
      // Own values and defaults as the server keeps them (UI9); the tree is left out here.
      state.settings ??= {};
      if (method === "PATCH") {
        const body = JSON.parse(bodyText) as Record<string, unknown>;
        for (const [k, v] of Object.entries(body)) {
          if (k === "defaults") {
            const d = { ...((state.settings["defaults"] as Record<string, unknown>) ?? {}) };
            for (const [t, vals] of Object.entries(v as Record<string, unknown>)) {
              if (vals === null) {
                delete d[t];
              } else {
                d[t] = vals;
              }
            }
            state.settings["defaults"] = d;
          } else if (v === null) {
            delete state.settings[k];
          } else {
            state.settings[k] = v;
          }
        }
      }
      const val = (k: string, fallback: string) =>
        state.settings![k]
          ? { value: state.settings![k], source: "own" }
          : { value: fallback, source: "default" };
      return json(200, {
        language: val("language", "de"),
        time_zone: val("time_zone", "Europe/Berlin"),
        theme: val("theme", "neutral"),
        defaults: state.settings["defaults"] ?? {},
      });
    }
    if (url.pathname === "/api/v1/org/nodes" && method === "GET") {
      return json(200, [
        { path: "root", name: "root", label: { de: "Gesamt" } },
        { path: "root.it", name: "it", label: { de: "IT" }, parent: "root" },
      ]);
    }
    const orgSetting = url.pathname.match(
      /^\/api\/v1\/org\/nodes\/([^/]+)\/settings(?:\/([^/]+))?$/,
    );
    if (orgSetting) {
      state.orgSettings ??= {};
      const node = orgSetting[1]!;
      const key = orgSetting[2];
      if (method === "GET") {
        const own = state.orgSettings[node] ?? {};
        return json(
          200,
          Object.entries(own).map(([k, value]) => ({ key: k, value, source: node })),
        );
      }
      if (state.orgForbidden) {
        return problem(403, "forbidden");
      }
      const own = (state.orgSettings[node] ??= {});
      if (method === "PUT") {
        own[key!] = JSON.parse(bodyText) as unknown;
      } else {
        delete own[key!];
      }
      return new Response(null, { status: 204 });
    }
    if (url.pathname.endsWith("/presence")) {
      return method === "DELETE"
        ? new Response(null, { status: 204 })
        : json(200, { others: state.others ?? [] });
    }
    if (url.pathname === "/api/v1/files" && method === "POST") {
      // An upload (UI7): a temporary file of the session.
      return json(201, {
        id: "01a15a3c-8545-7691-b780-f11e00000002",
        name: url.searchParams.get("name") ?? "file",
        content_type: "image/png",
        size: bodyText.length,
        sha256: "",
        state: "temporary",
        created_at: new Date().toISOString(),
      });
    }
    if (url.pathname === "/api/v1/records/probe.probe:batch" && method === "POST") {
      // Each row on its own, as the server does (UI6): the version must match.
      const body = JSON.parse(bodyText) as {
        rows: {
          id: string;
          version: number;
          set?: RecordData;
          transition?: string;
          trash?: boolean;
        }[];
      };
      const results = body.rows.map((row) => {
        const rec = state.records.find((r) => r["id"] === row.id);
        if (!rec) {
          return { id: row.id, status: "not_found" };
        }
        if (Number(rec["version"]) !== row.version) {
          return { id: row.id, status: "conflict", message: "version changed" };
        }
        if (row.trash) {
          state.records = state.records.filter((r) => r !== rec);
          return { id: row.id, status: "ok", version: row.version + 1 };
        }
        if (row.transition) {
          const result = applies(row.transition, rec);
          if (!result.allowed) {
            return { id: row.id, status: "refused", message: String(result.reason) };
          }
          rec["state"] = result.to;
        }
        Object.assign(rec, row.set ?? {}, { version: row.version + 1 });
        return { id: row.id, status: "ok", version: row.version + 1 };
      });
      return json(200, { results });
    }
    if (url.pathname === "/api/v1/records/probe.probe:export" && method === "POST") {
      if (state.exportJob) {
        return json(202, {
          id: 77,
          kind: "export.file",
          state: "available",
          attempt: 0,
          max_attempts: 3,
          queue: "bulk",
          created_at: new Date().toISOString(),
          scheduled_at: new Date().toISOString(),
        });
      }
      return new Response("\ufeffTitel\r\nProbe 000001\r\n", {
        status: 200,
        headers: {
          "Content-Type": "text/csv; charset=utf-8",
          "Content-Disposition": 'attachment; filename="probe-probe-2026-10-29-1200.csv"',
        },
      });
    }
    if (url.pathname === "/api/v1/jobs/77") {
      const done = state.jobState === "completed";
      return json(200, {
        id: 77,
        kind: "export.file",
        state: state.jobState ?? "running",
        attempt: 1,
        max_attempts: 3,
        queue: "bulk",
        created_at: new Date().toISOString(),
        scheduled_at: new Date().toISOString(),
        ...(done
          ? {
              output: { rows: 123456, file: "01a15a3c-8545-7691-b780-f11e00000001", name: "x.csv" },
            }
          : {}),
      });
    }
    if (url.pathname === "/api/v1/records/probe.probe:query" && method === "POST") {
      const body = JSON.parse(bodyText) as { where?: unknown; limit?: number };
      const items = state.records.filter((r) => matches(r, body.where)).slice(0, body.limit ?? 50);
      return json(200, { items });
    }
    if (url.pathname === "/api/v1/forms") {
      return json(200, state.forms ?? []);
    }
    const form = url.pathname.match(/^\/api\/v1\/forms\/([^/:]+)(:submit)?$/);
    if (form) {
      const f = (state.forms ?? []).find((x) => x.id === form[1]);
      if (!f) {
        return problem(404, "not_found");
      }
      if (!form[2]) {
        return json(200, f);
      }
      // Submitting (UI11): required questions, then the record with the bound answers and the
      // free ones as its first entry, as the server does.
      const { answers } = JSON.parse(bodyText) as { answers: Record<string, unknown> };
      // Hidden questions (UI12) do not count, as on the server.
      const hidden = hiddenOf(f.definition, answers).fields;
      const questions = f.definition.sections
        .flatMap((x) => x.questions)
        .filter((q) => !hidden.has(q.id));
      const errors = questions
        .filter((q) => {
          const v = answers[q.id];
          const required =
            q.required || (q.field && probeFields.find((x) => x.name === q.field)?.required);
          return required && (v === undefined || v === null || v === "");
        })
        .map((q) => ({ field: q.id, code: "required", message: "is required" }));
      if (errors.length > 0) {
        return problem(422, "validation", { errors });
      }
      const values: RecordData = {};
      const lines: string[] = [];
      for (const q of questions) {
        const v = answers[q.id];
        if (q.field) {
          if (v !== null && v !== undefined && v !== "") {
            values[q.field] = v;
          }
        } else if (v !== null && v !== undefined && v !== "") {
          lines.push(`${q.label?.["de"] ?? q.id}: ${String(v)}`);
        }
      }
      const stamp = new Date().toISOString();
      const rec: RecordData = {
        id: `01a15a3c-8545-7691-b780-f${String(state.records.length + 1).padStart(11, "0")}`,
        version: 1,
        state: "new",
        kind: "question",
        ...values,
        ...(state.personId
          ? { actors: { requester: [{ type: "core.person", id: state.personId }] } }
          : {}),
        created_at: stamp,
        updated_at: stamp,
        created_by: me.id,
      };
      state.records.push(rec);
      if (lines.length > 0) {
        state.timeline ??= {};
        const entry = {
          id: `01a15a3c-8545-7691-b780-d${String(state.records.length).padStart(11, "0")}`,
          body: lines.join("\n"),
          format: "text" as const,
          private: false,
          created_at: stamp,
          created_by: me.id,
        };
        state.timeline[String(rec["id"])] = [
          { kind: "entry", at: stamp, actor: me.id, actor_name: me.display_name, entry },
        ];
      }
      return json(201, rec);
    }
    const formRecord = url.pathname.match(/^\/api\/v1\/records\/core\.form(?:\/([^/]+))?$/);
    if (formRecord) {
      state.formRecords ??= [];
      const id = formRecord[1];
      if (!id && method === "POST") {
        const rec: RecordData = {
          ...(JSON.parse(bodyText) as RecordData),
          id: `01a15a3c-8545-7691-b780-c${String(state.formRecords.length + 1).padStart(11, "0")}`,
          version: 1,
          created_by: me.id,
        };
        state.formRecords.push(rec);
        return json(201, rec);
      }
      const rec = state.formRecords.find((x) => x["id"] === id);
      if (!rec) {
        return problem(404, "not_found");
      }
      if (method === "PATCH") {
        Object.assign(rec, JSON.parse(bodyText) as RecordData, {
          version: Number(rec["version"]) + 1,
        });
      }
      return json(200, rec);
    }
    const saved = url.pathname.match(/^\/api\/v1\/records\/core\.saved_search(?:\/([^/]+))?$/);
    if (saved) {
      state.searches ??= [];
      const id = saved[1];
      if (!id && method === "GET") {
        const target = url.searchParams.get("filter[target_type]");
        return json(200, {
          items: state.searches.filter((x) => !target || x["target_type"] === target),
        });
      }
      if (!id && method === "POST") {
        const rec: RecordData = {
          ...(JSON.parse(bodyText) as RecordData),
          id: `01a15a3c-8545-7691-b780-5${String(state.searches.length + 1).padStart(11, "0")}`,
          version: 1,
          created_by: me.id,
        };
        state.searches.push(rec);
        return json(201, rec);
      }
      const rec = state.searches.find((x) => x["id"] === id);
      if (!rec) {
        return problem(404, "not_found");
      }
      if (method === "PATCH") {
        Object.assign(rec, JSON.parse(bodyText) as RecordData, {
          version: Number(rec["version"]) + 1,
        });
      }
      return json(200, rec);
    }
    const person = url.pathname.match(/^\/api\/v1\/records\/core\.person\/([^/]+)$/);
    if (person && method === "GET") {
      const p = state.persons?.[person[1]!];
      return p ? json(200, p) : problem(404, "not_found");
    }
    const timeline = url.pathname.match(/^\/api\/v1\/records\/probe\.probe\/([^/]+)\/timeline$/);
    if (timeline) {
      const id = timeline[1]!;
      state.timeline ??= {};
      const items = (state.timeline[id] ??= []);
      if (method === "GET") {
        return json(200, items);
      }
      const body = JSON.parse(bodyText) as { body: string; format?: string; private?: boolean };
      if (body.private && state.privateRefused) {
        return problem(403, "forbidden");
      }
      const at = new Date().toISOString();
      const entry = {
        id: `01a15a3c-8545-7691-b780-e${String(items.length + 1).padStart(11, "0")}`,
        body: body.body,
        format: (body.format ?? "text") as "text" | "html",
        private: body.private === true,
        created_at: at,
        created_by: me.id,
      };
      items.push({ kind: "entry", at, actor: me.id, actor_name: me.display_name, entry });
      return json(201, entry);
    }
    const clocks = url.pathname.match(/^\/api\/v1\/records\/probe\.probe\/([^/]+)\/clocks$/);
    if (clocks) {
      return json(200, state.clocks?.[clocks[1]!] ?? []);
    }
    const list = url.pathname.match(/^\/api\/v1\/records\/probe\.probe$/);
    const one = url.pathname.match(/^\/api\/v1\/records\/probe\.probe\/([^/]+)$/);
    const transitions = url.pathname.match(
      /^\/api\/v1\/records\/probe\.probe\/([^/]+)\/transitions$/,
    );
    const transition = url.pathname.match(/^\/api\/v1\/records\/probe\.probe\/([^/]+):transition$/);
    if (transitions) {
      const rec = state.records.find((r) => r["id"] === transitions[1]);
      if (!rec) {
        return problem(404, "not_found");
      }
      return json(
        200,
        probeType.transitions!.map((t) => ({
          name: t.name,
          label: t.label,
          ...applies(t.name, rec),
        })),
      );
    }
    if (transition && method === "POST") {
      const rec = state.records.find((r) => r["id"] === transition[1]);
      if (!rec) {
        return problem(404, "not_found");
      }
      const body = JSON.parse(bodyText) as { transition: string };
      const result = applies(body.transition, rec);
      if (!result.allowed) {
        return problem(409, "conflict", { detail: `${body.transition}: ${result.reason}` });
      }
      Object.assign(rec, { state: result.to, version: Number(rec["version"]) + 1 });
      return json(200, rec, { ETag: `"${rec["version"]}"` });
    }
    if (list && method === "GET") {
      const limit = Number(url.searchParams.get("limit") ?? 50);
      const sort = url.searchParams.get("sort") ?? "id";
      const desc = sort.startsWith("-");
      const column = desc ? sort.slice(1) : sort;
      const sorted = [...state.records].sort((a, b) => {
        const x = String(a[column]);
        const y = String(b[column]);
        return (x < y ? -1 : x > y ? 1 : 0) * (desc ? -1 : 1);
      });
      const after = url.searchParams.get("after");
      const start = after ? sorted.findIndex((r) => r["id"] === after) + 1 : 0;
      const items = sorted.slice(start, start + limit);
      const last = items[items.length - 1];
      const next = start + limit < sorted.length && last ? String(last["id"]) : undefined;
      return json(200, { items, ...(next ? { next } : {}) });
    }
    if (list && method === "POST") {
      const body = JSON.parse(bodyText) as RecordData;
      if (!body["title"]) {
        return problem(422, "validation", {
          errors: [{ field: "title", code: "required", message: "title is required" }],
        });
      }
      const stamp = new Date().toISOString();
      const rec: RecordData = {
        id: `01a15a3c-8545-7691-b780-${String(state.records.length + 1).padStart(12, "0")}`,
        version: 1,
        state: "new",
        priority: 3,
        active: true,
        kind: "question",
        due_on: null,
        seen_at: null,
        parent: null,
        ...body,
        created_at: stamp,
        updated_at: stamp,
        created_by: me.id,
      };
      state.records.push(rec);
      return json(201, rec, { ETag: '"1"' });
    }
    if (one) {
      const rec = state.records.find((r) => r["id"] === one[1]);
      if (!rec) {
        return problem(404, "not_found");
      }
      if (method === "GET") {
        return json(200, rec, { ETag: `"${rec["version"]}"` });
      }
      if (method === "PATCH") {
        const ifMatch = req.headers.get("If-Match");
        if (!ifMatch) {
          return problem(428, "precondition_required");
        }
        if (ifMatch !== `"${rec["version"]}"`) {
          return problem(409, "conflict", { detail: "version changed" });
        }
        const body = JSON.parse(bodyText) as RecordData;
        Object.assign(rec, body, {
          version: Number(rec["version"]) + 1,
          updated_at: new Date().toISOString(),
        });
        return json(200, rec, { ETag: `"${rec["version"]}"` });
      }
    }
    return problem(404, "not_found");
  });
}
