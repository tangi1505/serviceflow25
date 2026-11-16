// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// MyRecords is "Meine Vorgänge" in the portal (UI43, UI plan E43, E52): one list over every
// type the portal creates, the last changed first, with title, number, type, time, state and
// a hint when it is the person's turn. The number of the open ones stands at the heading;
// "Offen" and "Alle" switch between the open and every record. The browser asks each type and
// mixes the answers: 20 at first, 20 more with each "Weitere anzeigen".
import { keepPreviousData, useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  meQuery,
  qualifiedName,
  type Field,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { formatMoment, formatValue, stateCategory } from "@/render/fields";
import { StatePill } from "@/ui/marks";

const pageSize = 20;

// waitsPath reads at a record whether its reason to wait means the requester (UI43): a path
// through the reference, which the server answers whatever the person may read of the reasons.
const waitsPath = "wait_reason.awaits_requester";

// mine is the condition of one's own records: as requester, or created oneself.
function mine(type: TypeDefinition, account: string): Record<string, unknown> {
  const created = { field: "created_by", eq: account };
  return type.actors?.some((r) => r.name === "requester")
    ? { or: [{ field: "actors.requester", eq: "@me" }, created] }
    : created;
}

// turns are the transitions a role at the record makes, such as confirming a solution.
function turns(type: TypeDefinition) {
  return (type.transitions ?? []).filter((tr) => !tr.disabled && (tr.by?.length ?? 0) > 0);
}

// openOnly is the condition of the open records: every state but those of the category done,
// and of those the ones where a role of the person still has a transition to make, such as a
// solution to confirm. A type without done states has none to leave out.
function openOnly(type: TypeDefinition): Record<string, unknown> | null {
  const done = (type.states ?? []).filter((s) => stateCategory(s) === "done").map((s) => s.name);
  if (done.length === 0) {
    return null;
  }
  const waiting = new Map<string, Set<string>>();
  for (const tr of turns(type)) {
    for (const role of tr.by!) {
      const states = waiting.get(role) ?? new Set<string>();
      tr.from.filter((s) => done.includes(s)).forEach((s) => states.add(s));
      if (states.size > 0) {
        waiting.set(role, states);
      }
    }
  }
  const notDone = { field: "state", nin: done };
  if (waiting.size === 0) {
    return notDone;
  }
  return {
    or: [
      notDone,
      ...[...waiting].map(([role, states]) => ({
        and: [
          { field: "state", in: [...states] },
          { field: `actors.${role}`, eq: "@me" },
        ],
      })),
    ],
  };
}

function condition(type: TypeDefinition, account: string, open: boolean) {
  const only = open ? openOnly(type) : null;
  return only ? { and: [mine(type, account), only] } : mine(type, account);
}

function titleOf(type: TypeDefinition): string {
  return type.title_field ?? type.fields[0]?.name ?? "id";
}

// numberOf is the running number of the type, shown with its prefix (PL49): INC-24817.
function numberOf(type: TypeDefinition): Field | undefined {
  return type.fields.find((f) => f.type === "sequence");
}

function waits(type: TypeDefinition): boolean {
  return type.fields.some(
    (f) => f.name === "wait_reason" && f.type === "reference" && f.to === "core.wait_reason",
  );
}

type Row = { type: TypeDefinition; record: RecordData };

// holds tells whether the person holds the role at the record.
function holds(record: RecordData, role: string, person: string | undefined): boolean {
  if (!person) {
    return false;
  }
  const actors = record["actors"] as Record<string, { id?: string }[] | undefined> | undefined;
  return (actors?.[role] ?? []).some((a) => a.id === person);
}

// hint says what the person is to do (UI43): the first transition of the state that one of
// her roles at the record makes, such as "Lösung bestätigen"; else "Rückfrage beantworten"
// while the record waits for a reason that means the requester.
function hint(
  row: Row,
  person: string | undefined,
  language: string,
  answer: string,
): string | undefined {
  const { type, record } = row;
  const state = String(record["state"] ?? "");
  const turn = turns(type).find(
    (tr) => tr.from.includes(state) && tr.by!.some((role) => holds(record, role, person)),
  );
  if (turn) {
    return labelOf(turn.label, language, turn.name);
  }
  const s = type.states?.find((x) => x.name === state);
  if (record[waitsPath] === true && stateCategory(s) !== "done") {
    return answer;
  }
  return undefined;
}

export function MyRecords({
  types,
  heading: Heading,
}: {
  types: TypeDefinition[];
  heading: "h1" | "h2";
}) {
  const { t, i18n } = useTranslation();
  const me = useQuery(meQuery);
  const account = me.data?.id;
  const [open, setOpen] = useState(true);
  const [shown, setShown] = useState(pageSize);
  // The number at the heading counts the open ones, whichever is shown.
  const counts = useQueries({
    queries: types.map((type) => ({
      queryKey: ["records", qualifiedName(type), "list", "portal", "open", account],
      queryFn: () =>
        api.query(qualifiedName(type), {
          where: condition(type, account!, true),
          fields: ["id"],
          limit: 1,
          count: true,
        }),
      enabled: account !== undefined,
    })),
  });
  const lists = useQueries({
    queries: types.map((type) => {
      const fields = [titleOf(type), "state", "updated_at"];
      const number = numberOf(type);
      if (number) {
        fields.push(number.name);
      }
      if (type.actors && type.actors.length > 0) {
        fields.push("actors");
      }
      if (waits(type)) {
        fields.push(waitsPath);
      }
      return {
        queryKey: ["records", qualifiedName(type), "list", "portal", open, shown, account],
        queryFn: () =>
          api.query(qualifiedName(type), {
            where: condition(type, account!, open),
            fields,
            sort: ["-updated_at"],
            // One more than shown tells whether there are more.
            limit: shown + 1,
          }),
        enabled: account !== undefined,
        placeholderData: keepPreviousData,
      };
    }),
  });
  // A type whose question fails does not hold back the others: what came counts.
  const total = counts.reduce((n, c) => n + (c.data?.total ?? 0), 0);
  const counted = counts.every((c) => !c.isPending);
  const loaded = lists.every((l) => !l.isPending);
  const all: Row[] = lists
    .flatMap((l, i) => (l.data?.items ?? []).map((record) => ({ type: types[i]!, record })))
    .sort((a, b) =>
      String(b.record["updated_at"] ?? "").localeCompare(String(a.record["updated_at"] ?? "")),
    );
  const rows = all.slice(0, shown);
  const more = all.length > shown;
  const switchTo = (next: boolean) => {
    setOpen(next);
    setShown(pageSize);
  };
  const toggle =
    "inline-flex h-9 items-center rounded-[5px] px-3 text-sm text-foreground-2 aria-pressed:bg-card aria-pressed:font-medium aria-pressed:text-foreground aria-pressed:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
  return (
    <section aria-labelledby="portal-mine" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-baseline gap-3">
          <Heading
            id="portal-mine"
            className={Heading === "h1" ? "text-2xl font-semibold" : "text-xl font-semibold"}
          >
            {t("portal.mine")}
          </Heading>
          {counted && (
            <span className="text-sm text-muted-foreground">
              {t("portal.openCount", { count: total })}
            </span>
          )}
        </div>
        <div
          role="group"
          aria-label={t("portal.show")}
          className="inline-flex gap-0.5 rounded-md border bg-surface-subtle p-0.5"
        >
          <button
            type="button"
            aria-pressed={open}
            onClick={() => switchTo(true)}
            className={toggle}
          >
            {t("portal.open")}
          </button>
          <button
            type="button"
            aria-pressed={!open}
            onClick={() => switchTo(false)}
            className={toggle}
          >
            {t("portal.all")}
          </button>
        </div>
      </div>
      {types.length === 0 && <p>{t("portal.nothing")}</p>}
      {types.length > 0 && loaded && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {open ? t("portal.noneOpen") : t("portal.none")}
        </p>
      )}
      {rows.length > 0 && (
        <ul className="flex flex-col divide-y rounded-md border bg-card">
          {rows.map((row) => {
            const { type, record } = row;
            const typeName = qualifiedName(type);
            const state = type.states?.find((s) => s.name === record["state"]);
            const number = numberOf(type);
            const todo = hint(row, me.data?.person_id, i18n.language, t("portal.answer"));
            return (
              <li
                key={`${typeName}/${String(record["id"])}`}
                className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <Link
                    to="/portal/$typeName/$id"
                    params={{ typeName, id: String(record["id"]) }}
                    className="font-medium underline-offset-2 hover:underline focus-visible:underline"
                  >
                    {String(record[titleOf(type)] ?? record["id"])}
                  </Link>
                  <span className="text-sm text-muted-foreground">
                    {[
                      number ? formatValue(number, record[number.name], i18n.language) : null,
                      labelOf(type.label, i18n.language, typeName),
                      formatMoment(record["updated_at"], i18n.language),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </span>
                </div>
                <div className="flex flex-none flex-wrap items-center gap-2">
                  {todo && (
                    <span className="rounded-full bg-primary px-2.5 py-0.5 text-[13px] leading-5 font-medium text-primary-foreground">
                      {todo}
                    </span>
                  )}
                  {state && (
                    <StatePill
                      label={labelOf(state.label, i18n.language, state.name)}
                      category={stateCategory(state)}
                    />
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {more && (
        <div>
          <button
            type="button"
            onClick={() => setShown((n) => n + pageSize)}
            className="inline-flex h-9 items-center rounded-md border bg-card px-3 text-sm hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {t("portal.moreRecords")}
          </button>
        </div>
      )}
    </section>
  );
}
