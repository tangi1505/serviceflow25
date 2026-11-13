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
  Assignment,
  Clock,
  Comparison,
  CustomerChange,
  CustomerLayer,
  Field,
  Label,
  State,
  Transition,
  FieldStates,
  FormOffer,
  PortalNotice,
  Permission,
  Rating,
  Approval,
  ApprovalDraft,
  ApprovalRequest,
  MyApproval,
  Substitute,
  RecordData,
  Relation,
  RelationKind,
  Role,
  SuggestGroup,
  TimelineItem,
  TypeDefinition,
  VersionInfo,
} from "@/api/client";

// The relation kinds of core (PL15) for the relations tab (SD4).
const defaultKinds: RelationKind[] = [
  {
    package: "core",
    name: "related",
    label: { de: "verknüpft mit", en: "related to" },
    inverse: { de: "verknüpft mit", en: "related to" },
    from: ["*"],
    to: ["*"],
  },
  {
    package: "core",
    name: "duplicate_of",
    label: { de: "Duplikat von", en: "duplicate of" },
    inverse: { de: "hat Duplikat", en: "has duplicate" },
    from: ["*"],
    to: ["*"],
  },
];

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
    { name: "active", type: "boolean", label: { de: "Angeboten" }, default: false },
    {
      name: "definition",
      type: "text",
      label: { de: "Aufbau" },
      required: true,
      max_length: 200000,
    },
  ],
};

// approverPerson is the person the fake answers approvals as (WF6).
export const approverPerson = "01a15a3c-8545-7691-b780-9b915e555b1d";

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

// articleType is suggested for the probe (SD18): linked with knowledge.uses, used by taking
// its title and the kind fault; rated and commented on by its readers.
export const articleType: TypeDefinition = {
  name: "article",
  package: "knowledge",
  label: { de: "Artikel" },
  title_field: "title",
  traits: ["timeline", "rating"],
  fields: [
    { name: "title", type: "text", label: { de: "Titel" } },
    { name: "body", type: "text", format: "rich", label: { de: "Text" } },
  ],
  readers: [{ actions: ["read", "comment"] }],
  portal: { read: { label: { de: "Wissen" }, fields: ["body"] } },
  suggest: {
    label: { de: "Wissen", en: "Knowledge" },
    for: ["probe.probe"],
    from: ["title"],
    relation: "knowledge.uses",
    apply: { label: { de: "Als Titel übernehmen" }, set: { title: "@title", kind: "fault" } },
  },
};

// customized applies a customer layer to a type the way the server does (PL43), for the
// operations the editor sends (UI25); refusals the tests need are answered before.
export function customized(base: TypeDefinition, layer?: CustomerLayer): TypeDefinition {
  let fields = base.fields.map((f) => ({ ...f }));
  let states = (base.states ?? []).map((s) => ({ ...s }));
  let transitions = (base.transitions ?? []).map((tr) => ({ ...tr }));
  let layout = base.layout;
  for (const c of layer?.changes ?? []) {
    const i = fields.findIndex((f) => f.name === c.field);
    const f = fields[i];
    switch (c.op) {
      case "add_field":
        fields.push(c.new as unknown as Field);
        break;
      case "change_field": {
        const set = (c.set ?? {}) as Partial<Field>;
        if (f) {
          fields[i] = {
            ...f,
            ...set,
            label: { ...f.label, ...set.label },
            description: { ...f.description, ...set.description },
          };
        }
        break;
      }
      case "add_options":
        if (f) {
          fields[i] = {
            ...f,
            options: [...(f.options ?? []), ...((c.options ?? []) as Field["options"] & object)],
          };
        }
        break;
      case "hide_field":
      case "show_field":
        if (f) {
          fields[i] = { ...f, hidden: c.op === "hide_field" };
        }
        break;
      case "remove_field":
        fields = fields.filter((x) => x.name !== c.field);
        break;
      case "move_field":
        if (f) {
          const rest = fields.filter((x) => x.name !== c.field);
          const at = c.after ? rest.findIndex((x) => x.name === c.after) + 1 : 0;
          rest.splice(at, 0, f);
          fields = rest;
        }
        break;
      // The state model (UI26.1), as far as the editor's tests need it.
      case "add_state":
        states.push(c.state as unknown as State);
        if ((c.state as { initial?: boolean })["initial"]) {
          states = states.map((s) => ({ ...s, initial: s.name === c.field }));
        }
        break;
      case "change_state": {
        const set = (c.set ?? {}) as { label?: Label; initial?: boolean };
        states = states.map((s) =>
          s.name === c.field
            ? { ...s, label: { ...s.label, ...set.label }, initial: set.initial ?? s.initial }
            : set.initial
              ? { ...s, initial: false }
              : s,
        );
        break;
      }
      case "remove_state":
        states = states.filter((s) => s.name !== c.field);
        break;
      case "add_transition":
        transitions.push(c.transition as unknown as Transition);
        break;
      case "change_transition": {
        const set = (c.set ?? {}) as Partial<Transition>;
        transitions = transitions.map((tr) =>
          tr.name === c.field ? { ...tr, ...set, label: { ...tr.label, ...set.label } } : tr,
        );
        break;
      }
      case "remove_transition":
        transitions = transitions.filter((tr) => tr.name !== c.field);
        break;
      case "disable_transition":
      case "enable_transition":
        transitions = transitions.map((tr) =>
          tr.name === c.field ? { ...tr, disabled: c.op === "disable_transition" } : tr,
        );
        break;
      // The layout of the graph (UI26.2) replaces the whole one.
      case "set_layout":
        layout = c.layout as TypeDefinition["layout"];
        break;
    }
  }
  return { ...base, fields, states, transitions, layout };
}

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
  const [op, given] = Object.entries(node).find(([k]) => k !== "field") ?? ["", undefined];
  // "@now" is the moment of the query on a system time (UI34).
  const want = given === "@now" ? new Date().toISOString() : given;
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
    case "gt":
      return v !== null && v !== undefined && String(v) > String(want);
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
  // relations between records and their kinds (SD4); kinds default to related and duplicate.
  relations?: Relation[];
  relationKinds?: RelationKind[];
  // roleTransitions are transitions only a role may take (SD-Plan N8), added to the probe
  // type; holder says whether the signed-in person holds the role. comments keeps their texts.
  roleTransitions?: (Transition & { holder: boolean })[];
  comments?: string[];
  // timed marks the probe type as one with SLA clocks (UI34).
  timed?: boolean;
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
  // extraRecords are the records of extra types by qualified name, to query, read and search
  // (the reading areas of the portal and their tree lists, SD16a).
  extraRecords?: Record<string, RecordData[]>;
  // versions and comparisons ("from-to") of the probe records (SD16b); restoring one bumps the
  // record's version and is answered with 409 while restoreConflict is set.
  versions?: VersionInfo[];
  comparisons?: Record<string, Comparison>;
  restoreConflict?: boolean;
  // searches are the saved searches the session may read (UI4); new ones are its own.
  searches?: RecordData[];
  // dashboards are the dashboards with their widgets (UI13); widgetData what each widget
  // shows by its id, dashboardInvalid answers a save with 422 and these errors.
  dashboards?: { dashboard: RecordData; widgets: RecordData[]; editable: boolean }[];
  // copyForbidden refuses copying a dashboard (UI14a).
  copyForbidden?: boolean;
  widgetData?: Record<string, Record<string, unknown>>;
  dashboardInvalid?: { field: string; code: string; message: string }[];
  // sidebarWidgets stand in the sidebar of every record; tileData is the number of a tile by id
  // (UI13b).
  sidebarWidgets?: RecordData[];
  tileData?: Record<string, Record<string, unknown>>;
  // workplace false makes the session a user of the portal (UI10); personId is its person.
  workplace?: boolean;
  personId?: string;
  // settings are the person's settings (UI9); orgSettings the values at nodes, orgForbidden
  // makes setting them 403, orgInvalid answers a change of those keys with 422 and the message
  // (SD20).
  settings?: Record<string, unknown>;
  orgSettings?: Record<string, Record<string, unknown>>;
  orgForbidden?: boolean;
  orgInvalid?: Record<string, string>;
  // others are who else has a record open (UI8), as the presence endpoint answers.
  others?: { account: string; name: string; editing: boolean; since: string; at: string }[];
  // forms are the forms offered to the session (UI11); formRecords the records of core.form.
  forms?: FormOffer[];
  // orders are the offers ordered, with the answers (SD13).
  orders?: { offer: string; answers: Record<string, unknown> }[];
  notices?: PortalNotice[];
  // snippets are the text modules (SD8) the editor lists; filled in, {{record.title}} becomes
  // the title of the probe.
  snippets?: RecordData[];
  formRecords?: RecordData[];
  // tenants are the tenants the session may switch between (PL44); tenant the one it is in.
  tenants?: { path: string; label: Record<string, string> }[];
  tenant?: string;
  // customize lets the session change types (UI25); customer are the customer layers by type.
  customize?: boolean;
  customer?: Record<string, CustomerLayer>;
  // manageRights offers the role editor (UI27); roles and assignments are what it edits.
  manageRights?: boolean;
  roles?: Role[];
  assignments?: Assignment[];
  // suggest are the groups of suggestions (SD18), answered for any type when the request has
  // values or a text, with linked from relations; ratings are the sums by record id.
  suggest?: SuggestGroup[];
  ratings?: Record<string, Rating>;
  // approvals are the approvals by record id (WF6), newest first; myApprovals the lists
  // "Zu genehmigen"; substitutes those of every person.
  approvals?: Record<string, Approval[]>;
  // approvalTemplates are the drafts of templates by record id (WF7).
  approvalTemplates?: Record<string, ApprovalDraft[]>;
  myApprovals?: { open?: MyApproval[]; done?: MyApproval[] };
  substitutes?: Substitute[];
  // exportJob makes an export a job (UI5, more than 100,000 rows); jobState is its state.
  exportJob?: boolean;
  jobState?: string;
  // calls are the requests in order; lang is their Accept-Language (SD16c).
  calls: {
    method: string;
    path: string;
    query: URLSearchParams;
    body?: string;
    lang?: string | null;
  }[];
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
    state.calls.push({
      method,
      path: url.pathname,
      query: url.searchParams,
      body: bodyText,
      lang: req.headers.get("Accept-Language"),
    });
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
    // Widgets in the sidebar of a record and the numbers of tiles (UI13b).
    const sidebar = url.pathname.match(
      /^\/api\/v1\/records\/([^/]+)\/([^/]+)\/widgets(?:\/([^/]+)\/data)?$/,
    );
    if (sidebar) {
      return sidebar[3]
        ? json(200, state.widgetData?.[sidebar[3]] ?? { kind: "number", value: 0 })
        : json(200, state.sidebarWidgets ?? []);
    }
    const tileData = url.pathname.match(/^\/api\/v1\/portal\/tiles\/([^/]+)\/data$/);
    if (tileData) {
      return json(200, state.tileData?.[tileData[1]!] ?? { kind: "number", value: 0 });
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
      customize: state.customize ?? false,
      manage_rights: state.manageRights ?? false,
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
      return json(200, [
        {
          ...customized(probeType, state.customer?.["probe.probe"]),
          ...(state.timed ? { timed: true } : {}),
          ...(state.roleTransitions
            ? { transitions: [...probeType.transitions!, ...state.roleTransitions] }
            : {}),
        },
        ...(state.extraTypes ?? []),
      ]);
    }
    const versioned = url.pathname.match(
      /^\/api\/v1\/records\/probe\.probe\/([^/]+)\/(versions|compare|versions\/(\d+):restore)$/,
    );
    if (versioned) {
      const record = state.records.find((r) => r["id"] === versioned[1]);
      if (!record) {
        return problem(404, "not_found");
      }
      if (versioned[2] === "versions") {
        return json(200, state.versions ?? []);
      }
      if (versioned[2] === "compare") {
        const key = `${url.searchParams.get("from")}-${url.searchParams.get("to")}`;
        return json(200, state.comparisons?.[key] ?? { from: 0, to: 0, changes: [] });
      }
      if (method === "POST") {
        if (
          state.restoreConflict ||
          req.headers.get("If-Match") !== `"${String(record["version"])}"`
        ) {
          return problem(409, "conflict");
        }
        record["version"] = Number(record["version"]) + 1;
        return json(200, record);
      }
    }
    const extra = url.pathname.match(/^\/api\/v1\/records\/([a-z_]+\.[a-z_]+)(:query|\/[^/:]+)$/);
    const extraList = extra ? state.extraRecords?.[extra[1]!] : undefined;
    if (extra && extraList) {
      if (extra[2] === ":query" && method === "POST") {
        const body = JSON.parse(bodyText) as { where?: unknown; limit?: number; sort?: string[] };
        // One sort column, as the server takes it (the second is always the id).
        if ((body.sort?.length ?? 0) > 1) {
          return problem(422, "validation");
        }
        const items = extraList.filter((r) => matches(r, body.where)).slice(0, body.limit ?? 50);
        return json(200, { items });
      }
      if (method === "GET") {
        const rec = extraList.find((r) => r["id"] === extra[2]!.slice(1));
        return rec ? json(200, rec) : problem(404, "not_found");
      }
    }
    // The condition check of the workflow editor (UI26.1): a condition with a dangling
    // operator is the one the tests use as invalid.
    if (url.pathname.endsWith(":check-condition") && method === "POST") {
      if (!state.customize) {
        return problem(403, "forbidden");
      }
      const when = (JSON.parse(bodyText) as { when: string }).when;
      return /[<>=!]\s*$/.test(when)
        ? json(200, { ok: false, error: "Syntax error: mismatched input '<EOF>'" })
        : json(200, { ok: true });
    }
    // The customer layer (PL43) as the editor uses it (UI25).
    const layerPath = /^\/api\/v1\/types\/([^/]+)\/customer(?:\/([^/]+))?$/.exec(url.pathname);
    if (layerPath || url.pathname === "/api/v1/customer") {
      if (!state.customize) {
        return problem(403, "forbidden");
      }
      state.customer ??= {};
      if (!layerPath) {
        return json(
          200,
          Object.values(state.customer).filter((l) => l.changes.length > 0),
        );
      }
      const typeName = decodeURIComponent(layerPath[1]!);
      const layer = (state.customer[typeName] ??= { type: typeName, version: 0, changes: [] });
      if (method === "GET") {
        return json(200, layer);
      }
      const seen =
        method === "DELETE"
          ? Number(url.searchParams.get("version"))
          : (JSON.parse(bodyText) as { version: number }).version;
      if (seen !== layer.version) {
        return problem(409, "conflict");
      }
      if (method === "DELETE") {
        layer.changes = layer.changes.filter((c) => c.id !== layerPath[2]);
      } else {
        const c = (JSON.parse(bodyText) as { change: CustomerChange }).change;
        if (c.op === "add_field" && !c.field.startsWith("k_")) {
          return problem(422, "validation", {
            errors: [
              {
                field: c.field,
                code: "invalid",
                message: "a field of the customer layer is named k_…",
              },
            ],
          });
        }
        if (c.op === "hide_field" && c.field === "title") {
          return problem(422, "validation", {
            errors: [
              {
                field: c.field,
                code: "invalid",
                message: "title is the title of probe.probe and stays visible",
              },
            ],
          });
        }
        if (
          (c.op === "remove_state" || c.op === "remove_transition") &&
          !c.field.startsWith("k_")
        ) {
          return problem(422, "validation", {
            errors: [
              { field: c.field, code: "invalid", message: `${c.field} belongs to the package` },
            ],
          });
        }
        if (c.op === "set_layout") {
          const names = new Set((customized(probeType, layer).states ?? []).map((s) => s.name));
          const bad = Object.keys(c.layout ?? {}).find((n) => !names.has(n));
          if (bad) {
            return problem(422, "validation", {
              errors: [
                {
                  field: "layout",
                  code: "invalid",
                  message: `layout names ${bad}, which is not a state of probe.probe`,
                },
              ],
            });
          }
        }
        if (c.op === "remove_field" && !c.field.startsWith("k_")) {
          return problem(422, "validation", {
            errors: [
              {
                field: c.field,
                code: "invalid",
                message: `${c.field} belongs to the package: hide it instead`,
              },
            ],
          });
        }
        layer.changes.push({
          ...c,
          id: `c${layer.version + 1}`,
          at: "2026-11-02T09:00:00Z",
          by: me.id,
          by_name: me.display_name,
          status: "active",
        });
      }
      layer.version++;
      return json(200, layer);
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
      // In a type with records of its own: the records whose title or text has the words.
      const within = state.extraRecords?.[url.searchParams.get("types") ?? ""];
      if (within) {
        const q = (url.searchParams.get("q") ?? "").toLowerCase();
        const found = within
          .filter((r) => `${String(r["title"])} ${String(r["body"])}`.toLowerCase().includes(q))
          .map((r) => ({
            type: url.searchParams.get("types")!,
            id: String(r["id"]),
            title: String(r["title"]),
            rank: 1,
            snippet: String(r["body"]),
          }));
        return json(200, found);
      }
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
        color_scheme: val("color_scheme", "system"),
        density: val("density", "normal"),
        start_dashboard: val("start_dashboard", ""),
        defaults: state.settings["defaults"] ?? {},
      });
    }
    // Roles and assignments (PL8) as the role editor uses them (UI27).
    if (
      url.pathname.startsWith("/api/v1/roles") ||
      url.pathname.startsWith("/api/v1/assignments")
    ) {
      if (!state.manageRights) {
        return problem(403, "forbidden");
      }
      state.roles ??= [];
      state.assignments ??= [];
      const roles = state.roles;
      if (url.pathname === "/api/v1/roles") {
        if (method === "POST") {
          const body = JSON.parse(bodyText) as { name: string; label?: Record<string, string> };
          if (roles.some((r) => r.name === body.name)) {
            return problem(409, "conflict");
          }
          const role: Role = {
            id: `role-${body.name}`,
            name: body.name,
            label: body.label ?? {},
            description: {},
            permissions: [],
          };
          roles.push(role);
          return json(201, role);
        }
        return json(200, roles);
      }
      const rolePath = /^\/api\/v1\/roles\/([^/]+)(\/permissions(?:\/([^/]+)\/([^/]+))?)?$/.exec(
        url.pathname,
      );
      if (rolePath) {
        const role = roles.find((r) => r.name === decodeURIComponent(rolePath[1]!));
        if (!role) {
          return problem(404, "not_found");
        }
        if (!rolePath[2]) {
          if (method === "DELETE") {
            state.roles = roles.filter((r) => r !== role);
            state.assignments = state.assignments.filter((a) => a.role !== role.name);
            return new Response(null, { status: 204 });
          }
          return json(200, role);
        }
        if (method === "PUT") {
          const p = JSON.parse(bodyText) as Permission;
          if (p.condition && /[<>=!]\s*$/.test(p.condition)) {
            return problem(422, "validation", {
              errors: [{ field: "condition", code: "invalid", message: "Syntax error" }],
            });
          }
          role.permissions = [
            ...role.permissions.filter(
              (x) => !(x.type_name === p.type_name && x.action === p.action && x.scope === p.scope),
            ),
            p,
          ];
          return new Response(null, { status: 204 });
        }
        if (method === "DELETE") {
          const typeName = decodeURIComponent(rolePath[3]!);
          const action = decodeURIComponent(rolePath[4]!);
          const scope = url.searchParams.get("scope");
          role.permissions = role.permissions.filter(
            (x) =>
              !(x.type_name === typeName && x.action === action && (!scope || x.scope === scope)),
          );
          return new Response(null, { status: 204 });
        }
      }
      if (url.pathname === "/api/v1/assignments") {
        if (method === "POST") {
          const body = JSON.parse(bodyText) as Omit<Assignment, "id">;
          const a: Assignment = {
            id: `assign-${state.assignments.length + 1}`,
            ...body,
            org_path: body.org_path ?? "root",
          };
          state.assignments.push(a);
          return json(201, a);
        }
        return json(200, state.assignments);
      }
      const assignPath = /^\/api\/v1\/assignments\/([^/]+)$/.exec(url.pathname);
      if (assignPath && method === "DELETE") {
        state.assignments = state.assignments.filter((a) => a.id !== assignPath[1]);
        return new Response(null, { status: 204 });
      }
      return problem(404, "not_found");
    }
    if (url.pathname === "/api/v1/org/nodes" && method === "GET") {
      return json(200, [
        { path: "root", name: "root", label: { de: "Gesamt" } },
        { path: "root.it", name: "it", label: { de: "IT" }, parent: "root" },
      ]);
    }
    if (url.pathname === "/api/v1/settings/definitions") {
      // The settings the packages declare (SD20), a few of each kind.
      return json(200, settingDefinitions);
    }
    const orgSetting = url.pathname.match(
      /^\/api\/v1\/org\/nodes\/([^/]+)\/settings(?:\/([^/]+))?$/,
    );
    if (orgSetting) {
      state.orgSettings ??= {};
      const node = orgSetting[1]!;
      const key = orgSetting[2];
      if (method === "GET") {
        // Each key from the nearest node on the way up (inheritance).
        const out: { key: string; value: unknown; source: string }[] = [];
        const parts = node.split(".");
        for (let i = parts.length; i > 0; i--) {
          const at = parts.slice(0, i).join(".");
          for (const [k, value] of Object.entries(state.orgSettings[at] ?? {})) {
            if (!out.some((o) => o.key === k)) out.push({ key: k, value, source: at });
          }
        }
        return json(200, out);
      }
      if (state.orgForbidden) {
        return problem(403, "forbidden");
      }
      const own = (state.orgSettings[node] ??= {});
      if (method === "PATCH") {
        // All changes at once, or none (SD20).
        const changes = JSON.parse(bodyText) as Record<string, unknown>;
        const bad = Object.keys(changes).filter(
          (k) => state.orgInvalid?.[k] && changes[k] !== null,
        );
        if (bad.length > 0) {
          return json(422, {
            type: "/problems/validation",
            title: "Eingabe ungültig",
            status: 422,
            errors: bad.map((k) => ({ field: k, code: "invalid", message: state.orgInvalid![k] })),
          });
        }
        for (const [k, v] of Object.entries(changes)) {
          if (v === null) delete own[k];
          else own[k] = v;
        }
      } else if (method === "PUT") {
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
    if (url.pathname === "/api/v1/records/core.snippet:query" && method === "POST") {
      const body = JSON.parse(bodyText) as { where: { and: { eq?: string }[] } };
      const use = body.where.and[0]?.eq;
      return json(200, { items: (state.snippets ?? []).filter((s) => s["use"] === use) });
    }
    const snippet = url.pathname.match(/^\/api\/v1\/records\/probe\.probe\/([^/]+):snippet$/);
    if (snippet && method === "POST") {
      const body = JSON.parse(bodyText) as { snippet: string };
      const found = (state.snippets ?? []).find((s) => s["id"] === body.snippet);
      const record = state.records.find((r) => r["id"] === snippet[1]);
      if (!found || !record) {
        return problem(404, "not_found");
      }
      const title = String(record["title"]).replace(/&/g, "&amp;").replace(/</g, "&lt;");
      return json(200, { body: String(found["body"]).replace("{{record.title}}", title) });
    }
    if (url.pathname === "/api/v1/records/probe.probe:query" && method === "POST") {
      const body = JSON.parse(bodyText) as { where?: unknown; limit?: number };
      const items = state.records.filter((r) => matches(r, body.where)).slice(0, body.limit ?? 50);
      return json(200, { items });
    }
    if (url.pathname === "/api/v1/forms") {
      // The forms of a place (SD9): portal when none is named, a form for both at either.
      const place = url.searchParams.get("place") ?? "portal";
      return json(
        200,
        (state.forms ?? []).filter((f) => (f.place ?? "portal") === place || f.place === "both"),
      );
    }
    if (url.pathname === "/api/v1/portal/notices") {
      return json(200, state.notices ?? []);
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
      // A role question names the holder (SD9); otherwise the person submitting is the requester.
      const actors: Record<string, { type: string; id: string }[]> = state.personId
        ? { requester: [{ type: "core.person", id: state.personId }] }
        : {};
      for (const q of questions) {
        const v = answers[q.id];
        if (q.role) {
          actors[q.role] = typeof v === "string" && v ? [{ type: "core.person", id: v }] : [];
        } else if (q.field) {
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
        ...f.definition.fixed,
        ...(Object.keys(actors).length > 0 ? { actors } : {}),
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
    // Ordering an offer (SD13): its form with the offer's values before the answers; the
    // record lands among the extra records of the form's target.
    const order = url.pathname.match(/^\/api\/v1\/records\/([a-z_]+\.[a-z_]+)\/([^/:]+):order$/);
    if (order && method === "POST") {
      const def = (state.extraTypes ?? []).find((x) => `${x.package}.${x.name}` === order[1]);
      const offer = (state.extraRecords?.[order[1]!] ?? []).find((r) => r["id"] === order[2]);
      const f = (state.forms ?? []).find((x) => x.id === offer?.[def?.order?.form ?? ""]);
      if (!def?.order || !offer || !f) {
        return problem(404, "not_found");
      }
      const { answers } = JSON.parse(bodyText) as { answers: Record<string, unknown> };
      const questions = f.definition.sections.flatMap((x) => x.questions);
      const errors = questions
        .filter((q) => q.required && (answers[q.id] === undefined || answers[q.id] === ""))
        .map((q) => ({ field: q.id, code: "required", message: "is required" }));
      if (errors.length > 0) {
        return problem(422, "validation", { errors });
      }
      const values: RecordData = {};
      for (const [to, from] of Object.entries(def.order.copy ?? {})) {
        values[to] = offer[from];
      }
      for (const q of questions) {
        if (q.field && answers[q.id] !== undefined) {
          values[q.field] = answers[q.id];
        }
      }
      const rec: RecordData = {
        id: `01a15a3c-8545-7691-b780-e${String(state.calls.length).padStart(11, "0")}`,
        version: 1,
        state: "new",
        ...values,
      };
      state.extraRecords ??= {};
      (state.extraRecords[f.target_type] ??= []).push(rec);
      state.orders = [...(state.orders ?? []), { offer: order[2]!, answers }];
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
    // Dashboards (UI13): the list for the navigation, one with its widgets, saving, data.
    if (url.pathname === "/api/v1/records/core.dashboard" && method === "GET") {
      return json(200, { items: (state.dashboards ?? []).map((d) => d.dashboard) });
    }
    if (url.pathname === "/api/v1/records/core.dashboard" && method === "POST") {
      state.dashboards ??= [];
      const dashboard: RecordData = {
        ...(JSON.parse(bodyText) as RecordData),
        id: `01a15a3c-8545-7691-b780-d${String(state.dashboards.length + 1).padStart(11, "0")}`,
        version: 1,
      };
      state.dashboards.push({ dashboard, widgets: [], editable: true });
      return json(201, dashboard);
    }
    // Copying a dashboard (UI14a): a new one of one's own with copies of the widgets.
    const copy = url.pathname.match(/^\/api\/v1\/dashboards\/([^/]+)\/copy$/);
    if (copy && method === "POST") {
      const d = (state.dashboards ?? []).find((x) => x.dashboard["id"] === copy[1]);
      if (!d) {
        return problem(404, "not_found");
      }
      if (state.copyForbidden) {
        return problem(403, "forbidden");
      }
      const n = state.dashboards!.length + 1;
      const made = {
        dashboard: {
          ...(JSON.parse(bodyText) as RecordData),
          id: `01a15a3c-8545-7691-b780-d${String(n).padStart(11, "0")}`,
          version: 1,
        },
        widgets: d.widgets.map((w, i) => ({
          ...w,
          id: `01a15a3c-8545-7691-b780-f${String(n * 100 + i).padStart(11, "0")}`,
        })),
        editable: true,
      };
      state.dashboards!.push(made);
      return json(201, made);
    }
    const board = url.pathname.match(
      /^\/api\/v1\/dashboards\/([^/]+)(?:\/widgets\/([^/]+)\/data)?$/,
    );
    if (board) {
      const d = (state.dashboards ?? []).find((x) => x.dashboard["id"] === board[1]);
      if (!d) {
        return problem(404, "not_found");
      }
      if (board[2]) {
        const w = d.widgets.find((x) => x["id"] === board[2]);
        return json(200, state.widgetData?.[board[2]] ?? { kind: String(w?.["kind"] ?? "text") });
      }
      if (method === "PUT") {
        if (state.dashboardInvalid) {
          return json(422, {
            type: "/problems/validation",
            title: "Eingabe ungültig",
            status: 422,
            errors: state.dashboardInvalid,
          });
        }
        const body = JSON.parse(bodyText) as { fields?: RecordData; widgets: RecordData[] };
        Object.assign(d.dashboard, body.fields ?? {}, {
          version: Number(d.dashboard["version"]) + 1,
        });
        d.widgets = body.widgets.map((w, i) => ({
          ...w,
          id: w["id"] ?? `01a15a3c-8545-7691-b780-e${String(i + 1).padStart(11, "0")}`,
        }));
      }
      return json(200, d);
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
    const timeline = url.pathname.match(
      /^\/api\/v1\/records\/[a-z_]+\.[a-z_]+\/([^/]+)\/timeline$/,
    );
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
    const suggest = url.pathname.match(/^\/api\/v1\/records\/([a-z_]+\.[a-z_]+):suggest$/);
    if (suggest && method === "POST") {
      const body = JSON.parse(bodyText) as {
        values?: Record<string, unknown>;
        text?: string;
        id?: string;
      };
      const asked =
        !!body.text || Object.values(body.values ?? {}).some((v) => String(v ?? "").trim() !== "");
      return json(
        200,
        (state.suggest ?? []).map((g) => ({
          ...g,
          hits: asked
            ? g.hits.map((h) => {
                const rel = (state.relations ?? []).find(
                  (r) => r.kind === g.relation && r.from.id === body.id && r.to.id === h.id,
                );
                return { ...h, ...(rel ? { linked: rel.id } : {}) };
              })
            : [],
        })),
      );
    }
    const draftsOf = url.pathname.match(
      /^\/api\/v1\/records\/[a-z_]+\.[a-z_]+\/([^/]+)\/approval-templates$/,
    );
    if (draftsOf) {
      return json(200, state.approvalTemplates?.[draftsOf[1]!] ?? []);
    }
    const approvalsOf = url.pathname.match(
      /^\/api\/v1\/records\/[a-z_]+\.[a-z_]+\/([^/]+)\/approvals(?:\/([^/:]+):(decide|withdraw))?$/,
    );
    if (approvalsOf) {
      state.approvals ??= {};
      const list = (state.approvals[approvalsOf[1]!] ??= []);
      if (!approvalsOf[2]) {
        if (method === "POST") {
          const body = JSON.parse(bodyText) as ApprovalRequest;
          const now = new Date().toISOString();
          const created: Approval = {
            id: `0190a000-0000-7000-8000-${String(list.length + 1).padStart(12, "0")}`,
            state: "open",
            stage: 1,
            message: body.message ?? "",
            template: body.template,
            requester: approverPerson,
            requested_at: now,
            stages: body.stages.map((st, i) => ({
              position: i + 1,
              name: st.name ?? "",
              need: st.need ?? 0,
              state: i === 0 ? "open" : "waiting",
              steps: st.approvers.map((a, j) => ({
                id: `0190a000-0000-7000-9000-${String(i * 100 + j + 1).padStart(12, "0")}`,
                approver: a,
                state: i === 0 ? "open" : "waiting",
                mine: false,
              })),
            })),
          };
          list.unshift(created);
          return json(201, created);
        }
        return json(200, list);
      }
      const found = list.find((a) => a.id === approvalsOf[2]);
      if (!found) {
        return json(404, { title: "Not Found", status: 404 });
      }
      if (approvalsOf[3] === "withdraw") {
        found.state = "withdrawn";
        return json(200, found);
      }
      const body = JSON.parse(bodyText) as { decision: string; comment?: string; step?: string };
      for (const st of found.stages) {
        for (const step of st.steps) {
          if (step.id === body.step) {
            step.state = body.decision === "approve" ? "approved" : "rejected";
            step.comment = body.comment;
            step.decided_by = approverPerson;
            step.mine = false;
          }
        }
      }
      return json(200, found);
    }
    if (url.pathname === "/api/v1/me/approvals") {
      const which = url.searchParams.get("state") === "done" ? "done" : "open";
      return json(200, state.myApprovals?.[which] ?? []);
    }
    const substitutesOf = url.pathname.match(
      /^\/api\/v1\/persons\/([^/]+)\/substitutes(?:\/([^/]+))?$/,
    );
    if (substitutesOf) {
      state.substitutes ??= [];
      const person = substitutesOf[1]!;
      if (method === "DELETE") {
        state.substitutes = state.substitutes.filter((x) => x.id !== substitutesOf[2]);
        return new Response(null, { status: 204 });
      }
      if (method === "POST") {
        const body = JSON.parse(bodyText) as {
          substitute: string;
          valid_from?: string;
          valid_until?: string;
        };
        const created: Substitute = {
          id: `0190b000-0000-7000-8000-${String(state.substitutes.length + 1).padStart(12, "0")}`,
          person,
          substitute: body.substitute,
          valid_from: body.valid_from ?? new Date().toISOString(),
          ...(body.valid_until ? { valid_until: body.valid_until } : {}),
          active: true,
        };
        state.substitutes.push(created);
        return json(201, created);
      }
      return json(
        200,
        state.substitutes.filter((x) => x.person === person),
      );
    }
    const rating = url.pathname.match(/^\/api\/v1\/records\/[a-z_]+\.[a-z_]+\/([^/]+)\/rating$/);
    if (rating) {
      state.ratings ??= {};
      const sum = (state.ratings[rating[1]!] ??= { yes: 0, no: 0, mine: null });
      if (method === "PUT") {
        const helpful = (JSON.parse(bodyText) as { helpful: boolean | null }).helpful;
        if (sum.mine === true) sum.yes--;
        if (sum.mine === false) sum.no--;
        if (helpful === true) sum.yes++;
        if (helpful === false) sum.no++;
        sum.mine = helpful;
      }
      return json(200, sum);
    }
    if (url.pathname === "/api/v1/relations/kinds") {
      return json(200, state.relationKinds ?? defaultKinds);
    }
    const relationsOf = url.pathname.match(/^\/api\/v1\/records\/([^/]+)\/([^/]+)\/relations$/);
    if (relationsOf) {
      const at = (e: { type: string; id: string }) =>
        e.type === relationsOf[1] && e.id === relationsOf[2];
      return json(
        200,
        (state.relations ?? []).filter((r) => at(r.from) || at(r.to)),
      );
    }
    if (url.pathname === "/api/v1/relations" && method === "POST") {
      const body = JSON.parse(bodyText) as Pick<Relation, "kind" | "from" | "to">;
      const rel: Relation = {
        ...body,
        id: `rel-${(state.relations ?? []).length + 1}`,
        created_at: new Date().toISOString(),
      };
      (state.relations ??= []).push(rel);
      return json(201, rel);
    }
    const relation = url.pathname.match(/^\/api\/v1\/relations\/([^/]+)$/);
    if (relation && method === "DELETE") {
      state.relations = (state.relations ?? []).filter((r) => r.id !== relation[1]);
      return new Response(null, { status: 204 });
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
      return json(200, [
        ...probeType.transitions!.map((t) => ({
          name: t.name,
          label: t.label,
          ...applies(t.name, rec),
        })),
        ...(state.roleTransitions ?? []).map((t) => {
          const from = t.from.includes(String(rec["state"]));
          return {
            name: t.name,
            label: t.label,
            to: t.to,
            by: t.by,
            comment: t.comment,
            allowed: from && t.holder,
            ...(from ? (t.holder ? {} : { reason: "role" }) : { reason: "state" }),
          };
        }),
      ]);
    }
    if (transition && method === "POST") {
      const rec = state.records.find((r) => r["id"] === transition[1]);
      if (!rec) {
        return problem(404, "not_found");
      }
      const body = JSON.parse(bodyText) as { transition: string; comment?: string };
      const byRole = state.roleTransitions?.find((t) => t.name === body.transition);
      if (byRole) {
        if (byRole.comment === "required" && !body.comment) {
          return problem(422, "validation", {
            errors: [{ field: "comment", code: "required", message: "is required" }],
          });
        }
        if (body.comment) {
          (state.comments ??= []).push(body.comment);
        }
        Object.assign(rec, { state: byRole.to, version: Number(rec["version"]) + 1 });
        return json(200, rec, { ETag: `"${rec["version"]}"` });
      }
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

// settingDefinitions are a few declared settings of the core and the package incident (SD20).
const settingDefinitions = [
  {
    package: "core",
    key: "ui.time_zone",
    kind: "time_zone",
    group: { de: "Oberfläche", en: "Interface" },
    label: { de: "Zeitzone", en: "Time zone" },
    default: "Europe/Berlin",
  },
  {
    package: "core",
    key: "files.max_size",
    kind: "size",
    group: { de: "Dateien", en: "Files" },
    label: { de: "Höchstgröße einer Datei", en: "Largest file" },
    default: 104857600,
  },
  {
    package: "core",
    key: "files.allowed_extensions",
    kind: "list",
    group: { de: "Dateien", en: "Files" },
    label: { de: "Erlaubte Endungen", en: "Allowed extensions" },
    description: {
      de: "Leer: alles außer den gesperrten Endungen.",
      en: "Empty: all but the blocked.",
    },
  },
  {
    package: "core",
    key: "security.local_login",
    kind: "choice",
    group: { de: "Sicherheit", en: "Security" },
    label: { de: "Lokale Anmeldung", en: "Local sign-in" },
    options: [
      { value: "all", label: { de: "Für alle Konten", en: "For every account" } },
      { value: "none", label: { de: "Keine", en: "None" } },
    ],
    default: "all",
    root: true,
  },
  {
    package: "incident",
    key: "incident.priority_matrix",
    kind: "matrix",
    group: { de: "Störung", en: "Incident" },
    label: { de: "Prioritätsmatrix", en: "Priority matrix" },
    rows: {
      label: { de: "Auswirkung", en: "Impact" },
      options: [
        { value: "high", label: { de: "Hoch", en: "High" } },
        { value: "low", label: { de: "Niedrig", en: "Low" } },
      ],
    },
    columns: {
      label: { de: "Dringlichkeit", en: "Urgency" },
      options: [
        { value: "high", label: { de: "Hoch", en: "High" } },
        { value: "low", label: { de: "Niedrig", en: "Low" } },
      ],
    },
    options: [
      { value: 1, label: { de: "P1", en: "P1" } },
      { value: 2, label: { de: "P2", en: "P2" } },
      { value: 3, label: { de: "P3", en: "P3" } },
    ],
    default: { high: { high: 1, low: 2 }, low: { high: 2, low: 3 } },
  },
  {
    package: "incident",
    key: "incident.close_after",
    kind: "duration",
    group: { de: "Störung", en: "Incident" },
    label: {
      de: "Gelöste Störung automatisch schließen nach",
      en: "Close a resolved incident after",
    },
    min: 0,
    default: 3300,
  },
];
