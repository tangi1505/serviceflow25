// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Suggestions while a user reports something in the portal (SD18, SD-Plan E12, variant 1A of
// the mockup): beside the form, below it on a phone, up to three hits of every type suggested
// for this one, found with what the form holds so far. A hit opens in place; "Das hat
// geholfen" counts a yes and ends without creating anything, "Hilft nicht" counts a no.
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  readQuery,
  typesQuery,
  type Field,
  type SuggestHit,
  type TypeDefinition,
} from "@/api/client";
import { RichText } from "@/render/RichText";
import { marked, suggesters } from "@/render/Suggestions";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";

import { FieldValue } from "./Read";

// useSettled is the value once it has rested for a while: no search per key stroke.
function useSettled<T>(value: T, ms: number): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return settled;
}

// textOf is what the form holds in the fields a suggesting type searches with.
function textOf(from: TypeDefinition[], values: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const s of from) {
    for (const f of s.suggest?.from ?? []) {
      if (typeof values[f] === "string" && values[f].trim() !== "") {
        out[f] = values[f];
      }
    }
  }
  return out;
}

function Hit({
  from,
  hit,
  open,
  onToggle,
  onHelped,
  onNotHelped,
  busy,
}: {
  from: TypeDefinition;
  hit: SuggestHit;
  open: boolean;
  onToggle: () => void;
  onHelped: () => void;
  onNotHelped: () => void;
  busy: boolean;
}) {
  const { t } = useTranslation();
  const bodyId = useId();
  const typeName = qualifiedName(from);
  const record = useQuery({ ...readQuery(typeName, hit.id), enabled: open });
  const shown = (from.portal?.read?.fields ?? [])
    .filter((n) => n !== from.portal?.read?.group)
    .map((n) => from.fields.find((f) => f.name === n))
    .filter((f): f is Field => Boolean(f));
  const title = hit.title ?? hit.id;
  return (
    <li
      className={
        "flex flex-col gap-1 rounded-md border bg-card p-3" + (open ? " border-foreground" : "")
      }
    >
      <button
        type="button"
        aria-expanded={open}
        aria-controls={bodyId}
        className="self-start text-left font-medium text-accent underline"
        onClick={onToggle}
      >
        {title}
      </button>
      {!open && <span className="text-sm">{marked(hit.snippet)}</span>}
      {open && (
        <div id={bodyId} className="flex flex-col gap-2 border-t border-border-soft pt-2">
          {record.isPending && <p className="text-sm">{t("list.loading")}</p>}
          {record.data &&
            shown.map((f) =>
              f.format === "rich" ? (
                <RichText key={f.name} html={String(record.data[f.name] ?? "")} />
              ) : (
                <p key={f.name} className="text-sm">
                  <FieldValue field={f} value={record.data[f.name]} />
                </p>
              ),
            )}
          {from.portal?.read && (
            <Link
              to="/portal/read/$typeName/$id"
              params={{ typeName, id: hit.id }}
              className="text-sm underline"
            >
              {t("suggest.open", { title })}
            </Link>
          )}
          <div className="flex flex-wrap gap-2 pt-1">
            <Button size="sm" disabled={busy} onClick={onHelped}>
              {t("suggest.helped")}
            </Button>
            <Button size="sm" variant="outline" disabled={busy} onClick={onNotHelped}>
              {t("suggest.notHelped")}
            </Button>
          </div>
        </div>
      )}
    </li>
  );
}

export function PortalSuggestions({
  type,
  values,
  onHelped,
}: {
  type: TypeDefinition;
  values: Record<string, unknown>;
  // onHelped ends the report: a hit helped, nothing is created.
  onHelped: (title: string) => void;
}) {
  const { t, i18n } = useTranslation();
  const headId = useId();
  const typeName = qualifiedName(type);
  const types = useQuery(typesQuery);
  const from = suggesters(types.data ?? [], type);
  // Settled as a string: the same values give the same text, and nothing draws again.
  const settled = useSettled(JSON.stringify(textOf(from, values)), 400);
  const text = JSON.parse(settled) as Record<string, string>;
  const asked = Object.keys(text).length > 0;
  const found = useQuery({
    queryKey: ["suggest", typeName, settled],
    queryFn: () => api.suggest(typeName, { values: text, limit: 3 }),
    enabled: from.length > 0 && asked,
  });
  const [open, setOpen] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const rate = useMutation({
    mutationFn: async (v: { group: string; hit: SuggestHit; helpful: boolean }) => {
      const suggesting = from.find((s) => qualifiedName(s) === v.group);
      if (suggesting?.traits?.includes("rating")) {
        await api.rate(v.group, v.hit.id, v.helpful);
      }
      return v;
    },
    onMutate: () => setFailure(null),
    onSuccess: (v) => {
      if (v.helpful) {
        onHelped(v.hit.title ?? v.hit.id);
      } else {
        setOpen(null);
      }
    },
    onError: (err) =>
      setFailure(t("rating.error", { detail: err instanceof Error ? err.message : String(err) })),
  });
  if (from.length === 0) {
    return null;
  }
  const groups = (found.data ?? []).filter((g) => g.hits.length > 0);
  return (
    <aside aria-labelledby={headId} className="flex flex-col gap-2">
      <h2 id={headId} className="text-base font-semibold">
        {t("suggest.portalHeading")}
      </h2>
      <div aria-live="polite" className="flex flex-col gap-2">
        {!asked && <p className="text-sm text-muted-foreground">{t("suggest.portalEmpty")}</p>}
        {asked && found.isSuccess && groups.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("suggest.none")}</p>
        )}
        {groups.map((g) => (
          <section key={g.type} className="flex flex-col gap-2">
            <p className="text-sm text-muted-foreground">
              {t("suggest.portalFrom", { label: labelOf(g.label, i18n.language, g.type) })}
            </p>
            <ul className="flex flex-col gap-2">
              {g.hits.map((h) => (
                <Hit
                  key={h.id}
                  from={from.find((s) => qualifiedName(s) === g.type)!}
                  hit={h}
                  open={open === h.id}
                  onToggle={() => setOpen(open === h.id ? null : h.id)}
                  onHelped={() => rate.mutate({ group: g.type, hit: h, helpful: true })}
                  onNotHelped={() => rate.mutate({ group: g.type, hit: h, helpful: false })}
                  busy={rate.isPending}
                />
              ))}
            </ul>
          </section>
        ))}
        {failure && <Alert variant="destructive">{failure}</Alert>}
      </div>
    </aside>
  );
}
