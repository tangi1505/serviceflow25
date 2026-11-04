// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The reading areas of the portal (SD16a, variant B of the mockup "Wissen im Portal"): a type
// whose package names portal.read gets an area in the head of the portal beside "Meine
// Vorgänge". There a person browses by the tree list the area's group field points to,
// searches the full text, and reads a record on a page of its own. What a person reads, the
// readers of the type and the roles decide on the server (SD-Plan N12); the portal only shows.
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  readQuery,
  typesQuery,
  type Field,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { fieldLabel, formatMoment, formatValue } from "@/render/fields";
import { RefName } from "@/render/RefName";
import { RichText } from "@/render/RichText";
import { Alert } from "@/ui/alert";

// creates tells a type users create in the portal (UI10), as against one they only read.
export function creates(type: TypeDefinition): boolean {
  return (type.portal?.fields?.length ?? 0) > 0;
}

// FieldValue shows a value in the portal: a reference by the name of its record, not its id
// (the finding of the SD16 click test).
export function FieldValue({ field, value }: { field: Field; value: unknown }) {
  const { i18n } = useTranslation();
  if (field.type === "reference" && field.to && typeof value === "string" && value) {
    return <RefName refTo={{ type: field.to, id: value }} />;
  }
  return <>{formatValue(field, value, i18n.language) || "–"}</>;
}

// AreaNav is "Meine Vorgänge" and one link per reading area in the head; nothing while there
// is no area.
export function AreaNav() {
  const { t, i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const areas = (types.data ?? []).filter((x) => x.portal?.read);
  if (areas.length === 0) {
    return null;
  }
  const link = "rounded-md px-2.5 py-1.5 text-sm";
  const active = { className: "bg-surface-active font-medium" };
  return (
    <nav aria-label={t("portal.areas")} className="flex flex-wrap gap-1">
      <Link to="/portal" activeOptions={{ exact: true }} className={link} activeProps={active}>
        {t("portal.mine")}
      </Link>
      {areas.map((x) => (
        <Link
          key={qualifiedName(x)}
          to="/portal/read/$typeName"
          params={{ typeName: qualifiedName(x) }}
          className={link}
          activeProps={active}
        >
          {labelOf(x.portal!.read!.label, i18n.language, qualifiedName(x))}
        </Link>
      ))}
    </nav>
  );
}

// plain is the text of HTML, cut to a teaser.
function plain(html: unknown, max = 160): string {
  if (typeof html !== "string" || html === "") {
    return "";
  }
  const text = (new DOMParser().parseFromString(html, "text/html").body.textContent ?? "")
    .replace(/\s+/g, " ")
    .trim();
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

// Entry is one entry of the tree list an area is browsed by.
type Entry = { id: string; name: string; parent: string | null };

// below lists an entry and every entry under it.
function below(entries: Entry[], id: string): string[] {
  const out = [id];
  for (let i = 0; i < out.length; i++) {
    for (const e of entries) {
      if (e.parent === out[i]) {
        out.push(e.id);
      }
    }
  }
  return out;
}

function useReadType(typeName: string) {
  const types = useQuery(typesQuery);
  return {
    type: types.data?.find((x) => qualifiedName(x) === typeName && x.portal?.read),
    pending: types.isPending,
  };
}

export function PortalReadPage() {
  const { t, i18n } = useTranslation();
  const { typeName } = useParams({ from: "/portal/read/$typeName" });
  const { group, q = "" } = useSearch({ from: "/portal/read/$typeName" });
  const { type, pending } = useReadType(typeName);
  if (pending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  const area = labelOf(type.portal!.read!.label, i18n.language, typeName);
  return <ReadArea key={`${group ?? ""}:${q}`} type={type} area={area} group={group} q={q} />;
}

function ReadArea({
  type,
  area,
  group,
  q,
}: {
  type: TypeDefinition;
  area: string;
  group?: string;
  q: string;
}) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate({ from: "/portal/read/$typeName" });
  const typeName = qualifiedName(type);
  const read = type.portal!.read!;
  const groupField = type.fields.find((f) => f.name === read.group && f.type === "reference");
  const title = type.title_field ?? type.fields[0]?.name ?? "id";
  const rich = read.fields.find((n) => type.fields.find((f) => f.name === n)?.format === "rich");
  const [text, setText] = useState(q);

  const entries = useQuery({
    queryKey: ["records", groupField?.to, "list", "portal-tree"],
    queryFn: async () =>
      (
        await api.query(groupField!.to!, {
          // The server sorts by one column (and the id); the name orders within a position.
          fields: ["name", "parent", "position"],
          sort: ["position"],
          limit: 500,
        })
      ).items
        .map((r): Entry & { position: number } => ({
          id: String(r["id"]),
          name: String(r["name"] ?? ""),
          parent: typeof r["parent"] === "string" ? r["parent"] : null,
          position: typeof r["position"] === "number" ? r["position"] : 0,
        }))
        .sort((a, b) => a.position - b.position || a.name.localeCompare(b.name)),
    enabled: Boolean(groupField?.to),
  });
  const tree = entries.data ?? [];
  const chosen = tree.find((e) => e.id === group);
  const searching = q.trim() !== "";

  const list = useQuery({
    queryKey: ["records", typeName, "list", "portal-read", group ?? ""],
    queryFn: () =>
      api.query(typeName, {
        ...(group && groupField
          ? { where: { field: groupField.name, in: below(tree, group) } }
          : {}),
        fields: [title, "updated_at", ...(rich ? [rich] : [])],
        sort: ["-updated_at"],
        limit: 50,
      }),
    enabled: !searching && (!group || entries.isSuccess),
  });
  const hits = useQuery({
    queryKey: ["search", typeName, q],
    queryFn: () => api.search(q, typeName, 50),
    enabled: searching,
  });

  const rows: { id: string; title: string; teaser: string }[] = searching
    ? (hits.data ?? []).map((h) => ({
        id: h.id,
        title: h.title ?? h.id,
        teaser: plain(h.snippet.replace(/<\/?mark>/g, "")),
      }))
    : (list.data?.items ?? []).map((r: RecordData) => ({
        id: String(r["id"]),
        title: String(r[title] ?? r["id"]),
        teaser: rich ? plain(r[rich]) : "",
      }));
  const loaded = searching ? hits.isSuccess : list.isSuccess;

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void navigate({
      search: { ...(group ? { group } : {}), ...(text.trim() ? { q: text.trim() } : {}) },
    });
  };

  // The tree as an indented list: the children of an entry follow it.
  const items: { entry: Entry; depth: number }[] = [];
  const walk = (parent: string | null, depth: number) => {
    for (const e of tree.filter((x) => x.parent === parent)) {
      items.push({ entry: e, depth });
      walk(e.id, depth + 1);
    }
  };
  walk(null, 0);

  return (
    <div className="flex flex-wrap gap-6">
      {groupField && (
        <nav
          aria-label={fieldLabel(groupField, i18n.language)}
          className="flex flex-[0_1_15rem] flex-col gap-0.5 text-sm"
        >
          {/* Exact, so that "Alle" is not current under an entry; the router marks the
              current one with aria-current. */}
          <Link
            to="/portal/read/$typeName"
            params={{ typeName }}
            className="rounded-md px-2 py-1"
            activeOptions={{ exact: true }}
            activeProps={{ className: "bg-surface-active font-medium" }}
          >
            {t("portal.readAll")}
          </Link>
          {items.map(({ entry, depth }) => (
            <Link
              key={entry.id}
              to="/portal/read/$typeName"
              params={{ typeName }}
              search={{ group: entry.id }}
              className="rounded-md py-1 pr-2"
              style={{ paddingLeft: `${0.5 + depth * 0.875}rem` }}
              activeOptions={{ exact: true }}
              activeProps={{ className: "bg-surface-active font-medium" }}
            >
              {entry.name}
            </Link>
          ))}
        </nav>
      )}
      <section
        aria-labelledby="read-heading"
        className="flex min-w-0 flex-[999_1_32rem] flex-col gap-3"
      >
        <h1 id="read-heading" className="text-2xl font-semibold">
          {searching ? t("portal.readHits", { q }) : chosen ? `${area}: ${chosen.name}` : area}
        </h1>
        <form role="search" onSubmit={submit} className="flex flex-col gap-1">
          <label htmlFor="read-q" className="text-sm text-muted-foreground">
            {t("portal.readSearch", { area })}
          </label>
          <input
            id="read-q"
            type="search"
            value={text}
            onChange={(e) => setText(e.target.value)}
            className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
          />
        </form>
        {loaded && rows.length === 0 && (
          <p className="text-sm text-muted-foreground">
            {searching ? t("portal.readNoHits") : t("portal.readNone")}
          </p>
        )}
        <ul className="flex flex-col divide-y">
          {rows.map((r) => (
            <li key={r.id} className="flex flex-col gap-0.5 py-2.5">
              <Link
                to="/portal/read/$typeName/$id"
                params={{ typeName, id: r.id }}
                className="font-medium underline"
              >
                {r.title}
              </Link>
              {r.teaser && <p className="text-sm text-muted-foreground">{r.teaser}</p>}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

export function PortalArticlePage() {
  const { t, i18n } = useTranslation();
  const { typeName, id } = useParams({ from: "/portal/read/$typeName/$id" });
  const { type, pending } = useReadType(typeName);
  const types = useQuery(typesQuery);
  const record = useQuery({ ...readQuery(typeName, id), retry: false });
  if (pending || record.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  if (!record.data) {
    return <Alert variant="destructive">{t("form.notFound")}</Alert>;
  }
  const rec = record.data;
  const read = type.portal!.read!;
  const area = labelOf(read.label, i18n.language, typeName);
  const groupField = type.fields.find((f) => f.name === read.group);
  const groupId = typeof rec[read.group ?? ""] === "string" ? String(rec[read.group!]) : "";
  const shown = read.fields
    .filter((n) => n !== read.group)
    .map((n) => type.fields.find((f) => f.name === n))
    .filter((f): f is Field => Boolean(f));
  const report = (types.data ?? []).filter(creates);
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-3">
      <nav aria-label="breadcrumb" className="flex gap-1.5 text-sm text-muted-foreground">
        <Link to="/portal" className="underline">
          {t("portal.title")}
        </Link>
        <span>/</span>
        <Link
          to="/portal/read/$typeName"
          params={{ typeName }}
          search={groupId ? { group: groupId } : {}}
          className="underline"
        >
          {area}
        </Link>
      </nav>
      <h1 className="text-2xl font-semibold">{String(rec[type.title_field ?? "id"] ?? id)}</h1>
      <p className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
        {groupField?.to && groupId && (
          <span className="rounded-full border px-2.5 py-0.5 text-foreground">
            <RefName refTo={{ type: groupField.to, id: groupId }} />
          </span>
        )}
        <span>
          {t("portal.changedOn", { time: formatMoment(rec["updated_at"], i18n.language) })}
        </span>
      </p>
      {shown.map((f) =>
        f.format === "rich" ? (
          <article key={f.name} className="rounded-md border bg-card p-4">
            <RichText html={String(rec[f.name] ?? "")} className="text-base" />
          </article>
        ) : (
          <p key={f.name} className="text-sm">
            <span className="text-muted-foreground">{fieldLabel(f, i18n.language)}: </span>
            <FieldValue field={f} value={rec[f.name]} />
          </p>
        ),
      )}
      {report.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-muted-foreground">{t("portal.readHelp")}</span>
          {report.map((x) => (
            <Link
              key={qualifiedName(x)}
              to="/portal/$typeName/new"
              params={{ typeName: qualifiedName(x) }}
              className="rounded-md border px-3 py-1.5"
            >
              {t("portal.newTitle", { type: labelOf(x.label, i18n.language, qualifiedName(x)) })}
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
