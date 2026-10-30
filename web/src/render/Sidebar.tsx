// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The sidebar of a record page (UI3): the roles at the record with names (WF3, read only
// until the picker with search comes), the SLA clocks with a traffic light and the time left
// (WF9), and the records that point here with their sums (tasks, WF13). A section without
// content is left out.
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import {
  childrenQuery,
  clocksQuery,
  labelOf,
  qualifiedName,
  typesQuery,
  type ActorRef,
  type Clock,
  type Field,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { cn } from "@/ui/utils";

import { fieldLabel, formatMoment, formatValue } from "./fields";
import { RefName, titleOf } from "./RefName";

function Box({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return (
    <section aria-labelledby={id} className="rounded-md border bg-card p-4">
      <h2 id={id} className="text-sm font-semibold uppercase text-muted-foreground">
        {title}
      </h2>
      <div className="mt-2 text-sm">{children}</div>
    </section>
  );
}

function Roles({ type, record }: { type: TypeDefinition; record: RecordData }) {
  const { t, i18n } = useTranslation();
  const roles = type.actors ?? [];
  if (roles.length === 0) {
    return null;
  }
  const holders = (record["actors"] ?? {}) as Record<string, ActorRef[] | null>;
  return (
    <Box id="roles-heading" title={t("sidebar.roles")}>
      <dl className="flex flex-col gap-2">
        {roles.map((r) => {
          const list = holders[r.name] ?? [];
          return (
            <div key={r.name}>
              <dt className="text-muted-foreground">{labelOf(r.label, i18n.language, r.name)}</dt>
              <dd>
                {list.length === 0
                  ? "–"
                  : list.map((h, i) => (
                      <span key={h.id}>
                        {i > 0 && ", "}
                        <RefName refTo={h} />
                      </span>
                    ))}
              </dd>
            </div>
          );
        })}
      </dl>
    </Box>
  );
}

// light is the traffic light of a clock: breached, close (under a fifth left), in time; a
// clock that no longer runs says why.
export function light(c: Clock): "breached" | "close" | "inTime" | "met" | "paused" | "cancelled" {
  if (c.breached) {
    return "breached";
  }
  switch (c.state) {
    case "stopped":
      return "met";
    case "paused":
      return "paused";
    case "cancelled":
      return "cancelled";
  }
  return c.target_minutes > 0 && c.remaining_minutes < c.target_minutes / 5 ? "close" : "inTime";
}

const lightColour: Record<ReturnType<typeof light>, string> = {
  breached: "bg-destructive",
  close: "bg-warning",
  inTime: "bg-success",
  met: "bg-success",
  paused: "bg-muted-foreground",
  cancelled: "bg-muted-foreground",
};

// duration writes minutes as hours and minutes, always positive.
export function duration(minutes: number): string {
  const m = Math.round(Math.abs(minutes));
  const h = Math.floor(m / 60);
  return h > 0 ? `${h} h ${m % 60} min` : `${m} min`;
}

function Clocks({ typeName, id }: { typeName: string; id: string }) {
  const { t, i18n } = useTranslation();
  const clocks = useQuery(clocksQuery(typeName, id));
  // No SLA configured (503) or none holds: no section.
  if (!clocks.data || clocks.data.length === 0) {
    return null;
  }
  return (
    <Box id="clocks-heading" title={t("sidebar.clocks")}>
      <ul className="flex flex-col gap-3">
        {clocks.data.map((c) => {
          const l = light(c);
          return (
            <li key={c.id}>
              <p className="flex items-center gap-2 font-medium">
                <span aria-hidden="true" className={cn("h-3 w-3 rounded-full", lightColour[l])} />
                {c.sla_name}
              </p>
              <p>{t(`sidebar.light.${l}`)}</p>
              {(c.state === "running" || c.state === "paused") && (
                <p className="text-muted-foreground">
                  {c.remaining_minutes >= 0
                    ? t("sidebar.remaining", { time: duration(c.remaining_minutes) })
                    : t("sidebar.overdue", { time: duration(c.remaining_minutes) })}
                  {c.due_at && (
                    <>
                      {" · "}
                      {t("sidebar.due", { at: formatMoment(c.due_at, i18n.language) })}
                    </>
                  )}
                </p>
              )}
            </li>
          );
        })}
      </ul>
    </Box>
  );
}

// Children lists the records of one type that point here through one field, with the sums
// the type keeps over them.
function Children({
  record,
  from,
  via,
  sums,
}: {
  record: RecordData;
  from: string;
  via: string;
  sums: Field[];
}) {
  const { t, i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const id = String(record["id"]);
  const children = useQuery(childrenQuery(from, via, id));
  const fromType = types.data?.find((x) => qualifiedName(x) === from);
  const headingId = `children-${from.replace(".", "-")}-${via}`;
  return (
    <Box id={headingId} title={labelOf(fromType?.label, i18n.language, from)}>
      <dl className="mb-2 flex flex-col gap-1">
        {sums.map((f) => (
          <div key={f.name} className="flex justify-between gap-2">
            <dt className="text-muted-foreground">{fieldLabel(f, i18n.language)}</dt>
            <dd>{formatValue(f, record[f.name], i18n.language) || "0"}</dd>
          </div>
        ))}
      </dl>
      {children.data && children.data.items.length === 0 && (
        <p className="text-muted-foreground">{t("sidebar.none")}</p>
      )}
      <ul className="flex flex-col gap-1">
        {(children.data?.items ?? []).map((c) => {
          const state = fromType?.states?.find((s) => s.name === c["state"]);
          return (
            <li key={String(c["id"])} className="flex justify-between gap-2">
              <Link
                to="/types/$typeName/$id"
                params={{ typeName: from, id: String(c["id"]) }}
                className="underline"
              >
                {titleOf(fromType, c)}
              </Link>
              {state && (
                <span className="text-muted-foreground">
                  {labelOf(state.label, i18n.language, state.name)}
                </span>
              )}
            </li>
          );
        })}
      </ul>
      {children.data?.next && <p className="mt-1 text-muted-foreground">{t("sidebar.more")}</p>}
    </Box>
  );
}

export function Sidebar({ type, record }: { type: TypeDefinition; record: RecordData }) {
  const typeName = qualifiedName(type);
  const groups = new Map<string, { from: string; via: string; sums: Field[] }>();
  for (const f of type.fields) {
    if (f.rollup) {
      const key = `${f.rollup.from}|${f.rollup.via}`;
      const g = groups.get(key) ?? { from: f.rollup.from, via: f.rollup.via, sums: [] };
      g.sums.push(f);
      groups.set(key, g);
    }
  }
  return (
    <aside className="flex flex-col gap-4">
      <Roles type={type} record={record} />
      <Clocks typeName={typeName} id={String(record["id"])} />
      {[...groups.values()].map((g) => (
        <Children key={`${g.from}|${g.via}`} record={record} {...g} />
      ))}
    </aside>
  );
}
