// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The trash (UI63, UI-Plan E61): one table over all types with type, title, when and by whom,
// newest first, only what the person may read (Kern-Plan PL74). Above it the type and a search
// in the title; chosen rows go back ("Wiederherstellen", right delete) or for good ("Endgültig
// löschen", right purge, after a question with their number). A line says after how many days
// the trash empties itself (Kern-Plan PL76).

import { useInfiniteQuery, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  typesQuery,
  type TrashEntry,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { confirm } from "@/ui/confirm";
import { Input } from "@/ui/input";
import { PageHeader } from "@/ui/page-header";
import { Select } from "@/ui/select";
import { Table, Td, Th, THead, Tr } from "@/ui/table";
import { toast } from "@/ui/toast";

import { formatMoment } from "./fields";
import { useDensity } from "./TableTools";

const keepKey = "trash.keep_days";

// mayTrash tells whether the person may move records to the trash somewhere: then the page is
// in the navigation.
export function mayTrash(types: TypeDefinition[] | undefined): boolean {
  return (types ?? []).some((t) => t.may?.includes("delete"));
}

// useKeepDays is trash.keep_days of the organisation: set at the root, else the default of
// its declaration, else 30.
function useKeepDays(): number | undefined {
  const root = useQuery({
    queryKey: ["org", "settings", "root"],
    queryFn: () => api.orgSettings("root"),
  });
  const defs = useQuery({ queryKey: ["settings", "definitions"], queryFn: api.settingDefinitions });
  if (root.isPending || defs.isPending) {
    return undefined;
  }
  const set = root.data?.find((s) => s.key === keepKey)?.value;
  const def = defs.data?.find((d) => d.key === keepKey)?.default;
  const v = set ?? def ?? 30;
  return typeof v === "number" ? v : Number(v);
}

export function TrashPage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const density = useDensity();
  const types = useQuery(typesQuery);
  const [type, setType] = useState("");
  const [q, setQ] = useState("");
  const [chosen, setChosen] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const days = useKeepDays();
  const list = useInfiniteQuery({
    queryKey: ["trash", type, q],
    queryFn: ({ pageParam }) =>
      api.trashList({
        ...(type ? { type } : {}),
        ...(q.trim() ? { q: q.trim() } : {}),
        ...(pageParam ? { after: pageParam } : {}),
      }),
    initialPageParam: "",
    getNextPageParam: (last) => last.next ?? undefined,
  });
  const rows = list.data?.pages.flatMap((p) => p.items) ?? [];
  const byName = new Map((types.data ?? []).map((d) => [qualifiedName(d), d]));
  const typeLabel = (name: string) => {
    const d = byName.get(name);
    return d ? labelOf(d.label, i18n.language, name) : name;
  };
  const may = (e: TrashEntry, action: "delete" | "purge") =>
    byName.get(e.type)?.may?.includes(action) === true;
  const key = (e: TrashEntry) => `${e.type}/${e.id}`;
  const picked = rows.filter((e) => chosen.has(key(e)));
  const anyPurge = (types.data ?? []).some((d) => d.may?.includes("purge"));
  const toggle = (e: TrashEntry) =>
    setChosen((c) => {
      const n = new Set(c);
      if (n.has(key(e))) {
        n.delete(key(e));
      } else {
        n.add(key(e));
      }
      return n;
    });
  const allChosen = rows.length > 0 && picked.length === rows.length;

  const done = () => {
    setChosen(new Set());
    void queryClient.invalidateQueries({ queryKey: ["trash"] });
    for (const e of picked) {
      void queryClient.invalidateQueries({ queryKey: ["records", e.type] });
    }
  };

  // run does an action on each chosen row on its own; it returns those that failed.
  const run = async (action: (e: TrashEntry) => Promise<void>, rowsToDo: TrashEntry[]) => {
    setBusy(true);
    const results = await Promise.allSettled(rowsToDo.map(action));
    setBusy(false);
    return rowsToDo
      .map((e, i) => ({ e, r: results[i]! }))
      .filter((x) => x.r.status === "rejected")
      .map((x) => ({
        e: x.e,
        why: x.r.status === "rejected" && x.r.reason instanceof Error ? x.r.reason.message : "",
      }));
  };

  const failedList = (failed: { e: TrashEntry; why: string }[]) => (
    <ul className="list-disc pl-4">
      {failed.map(({ e, why }) => (
        <li key={key(e)}>
          {e.title}
          {why && ` – ${why}`}
        </li>
      ))}
    </ul>
  );

  const restore = async () => {
    const todo = picked.filter((e) => may(e, "delete"));
    const failed = await run((e) => api.restore(e.type, e.id), todo);
    const ok = todo.filter((e) => !failed.some((f) => f.e === e));
    done();
    if (failed.length > 0) {
      toast.error(t("trashPage.restoreFailed", { count: failed.length }), failedList(failed));
    }
    if (ok.length > 0) {
      const first = ok[0]!;
      toast.success(
        t("trashPage.restored", { count: ok.length }),
        ok.length === 1 ? first.title : undefined,
        ok.length === 1
          ? {
              action: {
                label: t("trashPage.open"),
                onClick: () =>
                  void navigate({
                    to: "/types/$typeName/$id",
                    params: { typeName: first.type, id: first.id },
                  }),
              },
            }
          : undefined,
      );
    }
  };

  const purge = async () => {
    const todo = picked.filter((e) => may(e, "purge"));
    if (todo.length === 0) {
      return;
    }
    const yes = await confirm({
      title: t("trashPage.purgeAsk", { count: todo.length }),
      text: t("trashPage.purgeText", { count: todo.length }),
      action: t("trashPage.purge"),
    });
    if (!yes) {
      return;
    }
    const failed = await run((e) => api.purge(e.type, e.id), todo);
    done();
    if (failed.length > 0) {
      toast.error(t("trashPage.purgeFailed", { count: failed.length }), failedList(failed));
    }
    const ok = todo.length - failed.length;
    if (ok > 0) {
      toast.success(t("trashPage.purged", { count: ok }));
    }
  };

  const typeOptions = [
    { value: "", label: t("trashPage.allTypes") },
    ...(types.data ?? [])
      .filter((d) => d.may?.includes("read") !== false)
      .map((d) => ({
        value: qualifiedName(d),
        label: labelOf(d.label, i18n.language, qualifiedName(d)),
      }))
      .sort((a, b) => a.label.localeCompare(b.label, i18n.language)),
  ];

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t("trashPage.title")}
        line={
          days === undefined
            ? undefined
            : days > 0
              ? t("trashPage.keep", { count: days })
              : t("trashPage.keepNever")
        }
      />
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <span id={`${id}-type`} className="text-sm text-foreground-2">
            {t("trashPage.type")}
          </span>
          <Select
            labelledBy={`${id}-type`}
            size="bar"
            value={type}
            onChange={(v) => {
              setType(v);
              setChosen(new Set());
            }}
            options={typeOptions}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor={`${id}-q`} className="text-sm text-foreground-2">
            {t("trashPage.search")}
          </label>
          <Input
            id={`${id}-q`}
            size="bar"
            type="search"
            value={q}
            onChange={(e) => {
              setQ(e.target.value);
              setChosen(new Set());
            }}
          />
        </div>
        {picked.length > 0 && (
          <div
            role="group"
            aria-label={t("trashPage.chosen", { count: picked.length })}
            className="flex items-center gap-2"
          >
            <span className="text-sm">{t("trashPage.chosen", { count: picked.length })}</span>
            <Button
              size="bar"
              variant="outline"
              disabled={busy || !picked.some((e) => may(e, "delete"))}
              onClick={() => void restore()}
            >
              {t("trashPage.restore")}
            </Button>
            {anyPurge && (
              <Button
                size="bar"
                variant="destructive"
                disabled={busy || !picked.some((e) => may(e, "purge"))}
                onClick={() => void purge()}
              >
                {t("trashPage.purge")}
              </Button>
            )}
          </div>
        )}
      </div>
      {list.isError && <Alert variant="destructive">{t("trashPage.error")}</Alert>}
      {list.isPending && <p>{t("list.loading")}</p>}
      {list.isSuccess && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("trashPage.empty")}</p>
      )}
      {rows.length > 0 && (
        <Table density={density}>
          <THead>
            <tr>
              <Th className="w-10">
                <input
                  type="checkbox"
                  aria-label={t("trashPage.chooseAll")}
                  checked={allChosen}
                  onChange={() => setChosen(allChosen ? new Set() : new Set(rows.map(key)))}
                />
              </Th>
              <Th>{t("trashPage.colType")}</Th>
              <Th>{t("trashPage.colTitle")}</Th>
              <Th>{t("trashPage.colAt")}</Th>
              <Th>{t("trashPage.colBy")}</Th>
            </tr>
          </THead>
          <tbody>
            {rows.map((e) => (
              <Tr key={key(e)}>
                <Td>
                  <input
                    type="checkbox"
                    aria-label={t("trashPage.choose", { title: e.title })}
                    checked={chosen.has(key(e))}
                    onChange={() => toggle(e)}
                  />
                </Td>
                <Td>{typeLabel(e.type)}</Td>
                <Td className="max-w-96 truncate">{e.title}</Td>
                <Td className="font-mono text-[12.5px]">
                  {formatMoment(e.deleted_at, i18n.language)}
                </Td>
                <Td>{e.deleted_by_name ?? "–"}</Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
      {list.hasNextPage && (
        <div>
          <Button
            variant="outline"
            disabled={list.isFetchingNextPage}
            onClick={() => void list.fetchNextPage()}
          >
            {t("trashPage.more")}
          </Button>
        </div>
      )}
    </div>
  );
}
