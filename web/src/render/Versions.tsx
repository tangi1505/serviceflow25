// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The versions of a record (SD16b, variant A of the mockup "Fassungen vergleichen"): a tab on
// the record page of every type with history. The list names when, who and what; two chosen
// versions show the fields that differ side by side, the words of a text struck out and
// underlined (wordDiff, no library); restoring writes the older one as a new version after a
// question in the page. What a version holds and who may restore, the server decides (PL19).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, api, qualifiedName, type TypeDefinition } from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";

import { fieldLabel, formatMoment } from "./fields";
import { ChangeValue, changeField } from "./Timeline";
import { wordDiff, type Segment } from "./wordDiff";

// plain is the text of a value of a text field; formatted text without its tags.
export function plain(value: unknown, rich: boolean): string {
  if (value === null || value === undefined) {
    return "";
  }
  const s = String(value);
  return rich ? (new DOMParser().parseFromString(s, "text/html").body.textContent ?? "") : s;
}

export function Words({ segments }: { segments: Segment[] }) {
  if (segments.length === 0) {
    return <span className="text-muted-foreground">–</span>;
  }
  return (
    <>
      {segments.map((s, i) =>
        s.kind === "del" ? (
          <del key={i} className="text-foreground-2">
            {s.text}
          </del>
        ) : s.kind === "ins" ? (
          <ins key={i} className="bg-surface-active underline decoration-2">
            {s.text}
          </ins>
        ) : (
          <span key={i}>{s.text}</span>
        ),
      )}
    </>
  );
}

export function Versions({
  type,
  id,
  version,
}: {
  type: TypeDefinition;
  id: string;
  version: number;
}) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const headingId = useId();
  const versions = useQuery({
    queryKey: ["records", typeName, id, "versions", version],
    queryFn: () => api.versions(typeName, id),
  });
  const list = versions.data ?? [];
  const last = list[list.length - 1]?.version ?? 0;
  // The two newest by default; a click swaps the older of the two for the one clicked.
  const [chosen, setChosen] = useState<[number, number] | null>(null);
  const [from, to] = chosen ?? [Math.max(1, last - 1), last];
  const [asking, setAsking] = useState(false);
  const confirmRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (asking) {
      confirmRef.current?.focus();
    }
  }, [asking]);

  const comparison = useQuery({
    queryKey: ["records", typeName, id, "compare", from, to],
    queryFn: () => api.compare(typeName, id, from, to),
    enabled: last > 1 && from < to,
  });
  const restore = useMutation({
    mutationFn: () => api.restoreVersion(typeName, id, from, version),
    onSuccess: () => {
      setAsking(false);
      setChosen(null);
      void queryClient.invalidateQueries({ queryKey: ["records", typeName, id] });
      void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
    },
  });

  if (versions.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (versions.isError) {
    return <Alert variant="destructive">{t("list.error")}</Alert>;
  }
  if (last < 2) {
    return <p className="text-sm text-muted-foreground">{t("versions.one")}</p>;
  }

  const pick = (n: number) => {
    if (n === from || n === to) {
      return;
    }
    const next = [to, n].sort((a, b) => a - b) as [number, number];
    setChosen(next);
    setAsking(false);
    restore.reset();
  };
  const what = (v: (typeof list)[number]) => {
    if (v.actions.includes("create")) {
      return t("timeline.action.create");
    }
    const names = v.changed.map((f) => changeField(type, f, i18n.language, t("state.title")));
    return names.length > 0
      ? names.join(", ")
      : v.actions.map((a) => t(`timeline.action.${a}`, { defaultValue: a })).join(", ");
  };
  const failure =
    restore.error instanceof ApiError && restore.error.status === 409
      ? t("versions.conflict")
      : restore.error
        ? t("versions.failed", { message: restore.error.message })
        : null;

  return (
    <div className="flex flex-wrap items-start gap-4">
      <ul aria-label={t("versions.title")} className="flex flex-[0_1_20rem] flex-col gap-1.5">
        {[...list].reverse().map((v) => {
          const on = v.version === from || v.version === to;
          return (
            <li key={v.version}>
              <button
                type="button"
                aria-pressed={on}
                onClick={() => pick(v.version)}
                className={
                  "flex w-full flex-col gap-0.5 rounded-md border px-3 py-2 text-left text-sm " +
                  (on ? "border-foreground bg-surface-active" : "bg-card")
                }
              >
                <span className="font-semibold">
                  {t("versions.item", {
                    n: v.version,
                    time: formatMoment(v.at, i18n.language),
                  })}
                </span>
                <span className="text-foreground-2">
                  {v.actor_name ?? t("timeline.system")} · {what(v)}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      <section
        aria-labelledby={headingId}
        className="flex min-w-0 flex-[999_1_32rem] flex-col gap-3 rounded-md border bg-card p-4"
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 id={headingId} className="font-semibold">
            {t("versions.compare", { from, to })}
          </h3>
          <Button variant="outline" size="sm" onClick={() => setAsking(true)}>
            {t("versions.restore", { n: from })}
          </Button>
        </div>
        {asking && (
          <div
            role="group"
            aria-labelledby={`${headingId}-ask`}
            className="flex flex-col gap-2 rounded-md border border-foreground p-3 text-sm"
          >
            <p id={`${headingId}-ask`} className="font-semibold">
              {t("versions.ask", { n: from })}
            </p>
            <p>{t("versions.askText", { n: from, next: version + 1 })}</p>
            <div className="flex gap-2">
              <Button
                ref={confirmRef}
                size="sm"
                disabled={restore.isPending}
                onClick={() => restore.mutate()}
              >
                {t("versions.confirm")}
              </Button>
              <Button variant="outline" size="sm" onClick={() => setAsking(false)}>
                {t("versions.cancel")}
              </Button>
            </div>
          </div>
        )}
        {failure && <Alert variant="destructive">{failure}</Alert>}
        <p className="text-sm text-foreground-2">{t("versions.hint", { from, to })}</p>
        {comparison.data && comparison.data.changes.length === 0 && (
          <p className="text-sm">{t("versions.same")}</p>
        )}
        {comparison.data && comparison.data.changes.length > 0 && (
          <table className="w-full table-fixed text-sm">
            <thead>
              <tr className="text-left text-xs text-foreground-2">
                <th className="w-36 pb-1 font-semibold">{t("versions.field")}</th>
                <th className="pb-1 font-semibold">{t("versions.side", { n: from })}</th>
                <th className="pb-1 font-semibold">{t("versions.side", { n: to })}</th>
              </tr>
            </thead>
            <tbody>
              {comparison.data.changes.map((c) => {
                const f = type.fields.find((x) => x.name === c.field.split(".")[0]);
                const label = f
                  ? fieldLabel(f, i18n.language)
                  : changeField(type, c.field, i18n.language, t("state.title"));
                const text = f?.type === "text";
                const words = text
                  ? wordDiff(plain(c.from, f.format === "rich"), plain(c.to, f.format === "rich"))
                  : null;
                return (
                  <tr key={c.field} className="border-t align-top">
                    <th scope="row" className="py-2 pr-3 text-left font-medium">
                      {label}
                    </th>
                    <td className="break-words py-2 pr-3">
                      {words ? (
                        <Words segments={words.old} />
                      ) : (
                        <del className="text-foreground-2">
                          <ChangeValue type={type} field={c.field} value={c.from} />
                        </del>
                      )}
                    </td>
                    <td className="break-words py-2">
                      {words ? (
                        <Words segments={words.new} />
                      ) : (
                        <ins className="bg-surface-active underline decoration-2">
                          <ChangeValue type={type} field={c.field} value={c.to} />
                        </ins>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
