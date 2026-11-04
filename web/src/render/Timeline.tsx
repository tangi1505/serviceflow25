// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The timeline of a record (UI3, on PL42): a new entry on top, then entries and changes,
// newest first. The changes of one version by one person form one block; a switch shows
// everything or the entries only. A type with history but without the trait timeline shows
// its history the same way, without the input.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  qualifiedName,
  timelineQuery,
  type ActorRef,
  type AuditEntry,
  type TimelineEntry,
  type TimelineItem,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";

import { fieldLabel, formatMoment, formatValue } from "./fields";
import { RefName } from "./RefName";
import { RichEditor, RichText } from "./RichEditor";

// A block of the timeline: one entry, or the changes of one version by one account.
export type Block =
  | { kind: "entry"; key: string; at: string; actorName?: string; entry: TimelineEntry }
  | {
      kind: "changes";
      key: string;
      at: string;
      actorName?: string;
      version: number;
      changes: AuditEntry[];
    };

// blocks groups the items (oldest first, as the API sends them) and turns them round.
export function blocks(items: TimelineItem[]): Block[] {
  const out: Block[] = [];
  for (const it of items) {
    if (it.kind === "entry" && it.entry) {
      out.push({
        kind: "entry",
        key: it.entry.id,
        at: it.at,
        actorName: it.actor_name,
        entry: it.entry,
      });
      continue;
    }
    const c = it.change;
    if (!c) {
      continue;
    }
    const last = out[out.length - 1];
    if (
      last?.kind === "changes" &&
      last.version === c.version &&
      last.changes[0]?.actor === c.actor
    ) {
      last.changes.push(c);
      continue;
    }
    out.push({
      kind: "changes",
      key: c.id,
      at: it.at,
      actorName: it.actor_name,
      version: c.version,
      changes: [c],
    });
  }
  return out.reverse();
}

function isRefList(value: unknown): value is ActorRef[] {
  return Array.isArray(value) && value.every((v) => typeof v === "object" && v && "id" in v);
}

// ChangeValue shows an old or new value: roles by name, states and choices by label.
export function ChangeValue({
  type,
  field,
  value,
}: {
  type: TypeDefinition;
  field: string;
  value: unknown;
}) {
  const { i18n } = useTranslation();
  if (
    value === null ||
    value === undefined ||
    value === "" ||
    (isRefList(value) && value.length === 0)
  ) {
    return <span className="text-muted-foreground">–</span>;
  }
  if (isRefList(value)) {
    return (
      <>
        {value.map((r, i) => (
          <span key={r.id}>
            {i > 0 && ", "}
            <RefName refTo={r} />
          </span>
        ))}
      </>
    );
  }
  if (field === "state") {
    const s = type.states?.find((x) => x.name === value);
    return <>{labelOf(s?.label, i18n.language, String(value))}</>;
  }
  const [name] = field.split(".");
  const f = type.fields.find((x) => x.name === name);
  if (f?.type === "reference" && f.to && typeof value === "string") {
    // A reference by the name of its record, as a role (the finding of the SD16 click test).
    return <RefName refTo={{ type: f.to, id: value }} />;
  }
  return <>{formatValue(f, value, i18n.language) || String(value)}</>;
}

// changeField is the label of the field a change names: a translation (title.en) with its
// language, a role (actors.assigned) by the label of the role.
export function changeField(
  type: TypeDefinition,
  field: string,
  language: string,
  stateLabel: string,
): string {
  if (field === "state") {
    return stateLabel;
  }
  const [name, sub] = field.split(".");
  if (name === "actors" && sub) {
    const role = type.actors?.find((r) => r.name === sub);
    return labelOf(role?.label, language, sub);
  }
  const f = type.fields.find((x) => x.name === name);
  const label = f ? fieldLabel(f, language) : (name ?? field);
  return sub ? `${label} (${sub})` : label;
}

function Changes({ type, changes }: { type: TypeDefinition; changes: AuditEntry[] }) {
  const { t, i18n } = useTranslation();
  const lines: ReactNode[] = [];
  const seen = new Set<string>();
  for (const c of changes) {
    const fieldChange = c.field && (c.action === "update" || c.action === "transition");
    if (!fieldChange) {
      // One line per action without a field: created, breach, escalation, notify, …
      const key = `${c.action}-${c.rule ?? ""}`;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      lines.push(
        <li key={c.id}>
          {t(`timeline.action.${c.action}`, { defaultValue: c.action })}
          {c.rule && (
            <span className="text-muted-foreground"> ({t("timeline.rule", { rule: c.rule })})</span>
          )}
        </li>,
      );
      continue;
    }
    lines.push(
      <li key={c.id}>
        <span className="font-medium">
          {changeField(type, c.field!, i18n.language, t("state.title"))}
        </span>
        {": "}
        <ChangeValue type={type} field={c.field!} value={c.old} />
        {" → "}
        <ChangeValue type={type} field={c.field!} value={c.new} />
        {c.rule && (
          <span className="text-muted-foreground"> ({t("timeline.rule", { rule: c.rule })})</span>
        )}
      </li>,
    );
  }
  return <ul className="mt-1 flex flex-col gap-0.5 text-sm">{lines}</ul>;
}

function NewEntry({ typeName, id, portal }: { typeName: string; id: string; portal: boolean }) {
  const { t } = useTranslation();
  const formId = useId();
  const queryClient = useQueryClient();
  const [body, setBody] = useState("");
  const [isPrivate, setPrivate] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const add = useMutation({
    mutationFn: () => api.addEntry(typeName, id, body, isPrivate),
    onMutate: () => setFailure(null),
    onSuccess: () => {
      setBody("");
      setPrivate(false);
      void queryClient.invalidateQueries({ queryKey: timelineQuery(typeName, id).queryKey });
    },
    onError: (err) =>
      setFailure(
        err instanceof ApiError && err.status === 403
          ? t(isPrivate ? "timeline.forbiddenPrivate" : "timeline.forbidden")
          : t("timeline.error", { detail: err instanceof Error ? err.message : String(err) }),
      ),
  });
  return (
    <form
      className="flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        if (body.trim()) {
          add.mutate();
        }
      }}
    >
      <Label htmlFor={`${formId}-body`}>{t("timeline.new")}</Label>
      <RichEditor
        id={`${formId}-body`}
        label={t("timeline.new")}
        value={body}
        onChange={setBody}
        // The requester in the portal writes without text modules (SD8).
        snippets={portal ? undefined : { use: "answer", typeName, recordId: id }}
      />
      <div className="flex flex-wrap items-center gap-4">
        {!portal && (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="h-4 w-4"
              checked={isPrivate}
              onChange={(e) => setPrivate(e.target.checked)}
            />
            {t("timeline.private")}
          </label>
        )}
        <Button type="submit" size="sm" disabled={!body.trim() || add.isPending}>
          {t("timeline.add")}
        </Button>
      </div>
      {failure && <Alert variant="destructive">{failure}</Alert>}
    </form>
  );
}

// portal (UI10): no private entries to write; what a user reads the server decides. inTab: in
// the tabs of the record page (UI-Plan E38), whose tab names it; the heading stays for readers.
export function Timeline({
  type,
  id,
  portal = false,
  inTab = false,
}: {
  type: TypeDefinition;
  id: string;
  portal?: boolean;
  inTab?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const typeName = qualifiedName(type);
  const withEntries = type.traits?.includes("timeline") === true;
  const items = useQuery(timelineQuery(typeName, id, withEntries));
  const [onlyEntries, setOnlyEntries] = useState(false);
  const shown = blocks(items.data ?? []).filter((b) => !onlyEntries || b.kind === "entry");
  const when = (at: string) => formatMoment(at, i18n.language);
  return (
    <section
      aria-labelledby="timeline-heading"
      className={inTab ? "flex max-w-2xl flex-col gap-4" : "mt-8 flex max-w-2xl flex-col gap-4"}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="timeline-heading" className={inTab ? "sr-only" : "text-lg font-semibold"}>
          {withEntries ? t("timeline.title") : t("timeline.history")}
        </h2>
        {withEntries && (
          <div role="group" aria-label={t("timeline.filter")} className="flex gap-1">
            <Button
              size="sm"
              variant={onlyEntries ? "outline" : "default"}
              aria-pressed={!onlyEntries}
              onClick={() => setOnlyEntries(false)}
            >
              {t("timeline.all")}
            </Button>
            <Button
              size="sm"
              variant={onlyEntries ? "default" : "outline"}
              aria-pressed={onlyEntries}
              onClick={() => setOnlyEntries(true)}
            >
              {t("timeline.entries")}
            </Button>
          </div>
        )}
      </div>
      {withEntries && <NewEntry typeName={typeName} id={id} portal={portal} />}
      {items.isPending && <p>{t("list.loading")}</p>}
      {items.isError && <Alert variant="destructive">{t("timeline.loadError")}</Alert>}
      {items.isSuccess && shown.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("timeline.empty")}</p>
      )}
      <ol className="flex flex-col gap-3">
        {shown.map((b) => (
          <li
            key={b.key}
            className={
              b.kind === "entry" ? "rounded-md border bg-card p-3" : "border-l-2 border-border pl-3"
            }
          >
            <p className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">
                {b.actorName ?? t("timeline.system")}
              </span>
              {" · "}
              <time dateTime={b.at}>{when(b.at)}</time>
              {b.kind === "changes" && <> · {t("form.version", { version: b.version })}</>}
              {b.kind === "entry" && b.entry.private && (
                <span className="ml-2 rounded-sm border border-warning px-1 text-foreground">
                  {t("timeline.privateBadge")}
                </span>
              )}
            </p>
            {b.kind === "entry" ? (
              b.entry.format === "html" ? (
                <RichText html={b.entry.body} className="mt-1" />
              ) : (
                <p className="mt-1 whitespace-pre-wrap text-sm">{b.entry.body}</p>
              )
            ) : (
              <Changes type={type} changes={b.changes} />
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
