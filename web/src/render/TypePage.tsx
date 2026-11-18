// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The page of a type: its list, rendered from the definition (UI2), with filters and saved
// searches (UI4, Hauptplan E124). The saved searches of the type stand above the list, one's
// own apart from those shared with one; the chosen one is in the address (?search=<id>), so a
// link opens it. Saving keeps the conditions, the columns in their order and the sort; sharing
// is the role "shared with" on the page of the search (UI28).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { useCallback, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  meQuery,
  qualifiedName,
  recordQuery,
  typesQuery,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import { Filters, fromWhere, toWhere, type Condition } from "./Filters";
import { List, type ListView } from "./List";
import { Preview } from "./Preview";

const searchType = "core.saved_search";

const searchesQuery = (typeName: string) => ({
  queryKey: ["records", searchType, "list", "for", typeName],
  queryFn: () =>
    api.list(searchType, { limit: 100, sort: "id", filter: { target_type: typeName } }),
});

// parseWhere reads the condition of a saved search; an empty one is none.
function parseWhere(raw: unknown): unknown {
  if (typeof raw !== "string" || raw.trim() === "") {
    return undefined;
  }
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return undefined;
  }
}

function SearchBar({
  typeName,
  current,
  where,
  view,
}: {
  typeName: string;
  current?: RecordData;
  where?: Record<string, unknown>;
  view?: ListView;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const searches = useQuery(searchesQuery(typeName));
  const formId = useId();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const items = searches.data?.items ?? [];
  const own = items.filter((s) => s["created_by"] === me.data?.id);
  const shared = items.filter((s) => s["created_by"] !== me.data?.id);
  const open = (id?: string) =>
    void navigate({
      to: "/types/$typeName",
      params: { typeName },
      search: id ? { search: id } : {},
    });
  const body = () => ({
    query: where ? JSON.stringify(where) : "",
    columns: view?.columns.join(",") ?? "",
    sort: view?.sort ?? "",
  });
  const done = (saved: RecordData) => {
    void queryClient.invalidateQueries({ queryKey: ["records", searchType] });
    setNaming(false);
    setName("");
    open(String(saved["id"]));
  };
  const fail = (err: unknown) =>
    setFailure(
      err instanceof ApiError && err.status === 403
        ? t("search.forbidden")
        : t("search.error", { detail: err instanceof Error ? err.message : String(err) }),
    );
  const create = useMutation({
    mutationFn: () => api.create(searchType, { name, target_type: typeName, ...body() }),
    onMutate: () => setFailure(null),
    onSuccess: done,
    onError: fail,
  });
  const update = useMutation({
    mutationFn: () =>
      api.update(searchType, String(current!["id"]), Number(current!["version"]), body()),
    onMutate: () => setFailure(null),
    onSuccess: (saved) => {
      queryClient.setQueryData(recordQuery(searchType, String(saved["id"])).queryKey, saved);
      done(saved);
    },
    onError: fail,
  });
  const mine = current !== undefined && current["created_by"] === me.data?.id;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-end gap-2">
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${formId}-pick`}>{t("search.saved")}</Label>
          <select
            id={`${formId}-pick`}
            className="h-10 min-w-56 rounded-md border border-input bg-background px-3 text-sm"
            value={current ? String(current["id"]) : ""}
            onChange={(e) => open(e.target.value || undefined)}
          >
            <option value="">{t("search.all")}</option>
            {own.length > 0 && (
              <optgroup label={t("search.own")}>
                {own.map((s) => (
                  <option key={String(s["id"])} value={String(s["id"])}>
                    {String(s["name"])}
                  </option>
                ))}
              </optgroup>
            )}
            {shared.length > 0 && (
              <optgroup label={t("search.shared")}>
                {shared.map((s) => (
                  <option key={String(s["id"])} value={String(s["id"])}>
                    {String(s["name"])}
                  </option>
                ))}
              </optgroup>
            )}
          </select>
        </div>
        {mine && (
          <Button variant="outline" disabled={update.isPending} onClick={() => update.mutate()}>
            {t("search.update")}
          </Button>
        )}
        {!naming && (
          <Button variant="outline" onClick={() => setNaming(true)}>
            {t("search.saveAs")}
          </Button>
        )}
        {current && (
          <Link
            to="/types/$typeName/$id"
            params={{ typeName: searchType, id: String(current["id"]) }}
            className="self-center text-sm underline"
          >
            {mine ? t("search.share") : t("search.details")}
          </Link>
        )}
      </div>
      {naming && (
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) {
              create.mutate();
            }
          }}
        >
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${formId}-name`}>{t("search.name")}</Label>
            <Input
              id={`${formId}-name`}
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={!name.trim() || create.isPending}>
            {t("search.save")}
          </Button>
          <Button variant="outline" onClick={() => setNaming(false)}>
            {t("form.cancel")}
          </Button>
        </form>
      )}
      {failure && <Alert variant="destructive">{failure}</Alert>}
    </div>
  );
}

// TypeView is the list with its filters; another saved search makes a new one (key).
function TypeView({
  type,
  saved,
  initialWhere,
}: {
  type: TypeDefinition;
  saved?: RecordData;
  // initialWhere is a condition from the address, such as a click on a dashboard (UI13).
  initialWhere?: string;
}) {
  const { t } = useTranslation();
  const typeName = qualifiedName(type);
  const savedWhere = parseWhere(saved?.["query"] ?? initialWhere);
  const parsed = fromWhere(savedWhere);
  // A condition the filters cannot show (written over the API) applies as it is.
  const raw = parsed === null ? (savedWhere as Record<string, unknown>) : undefined;
  const [conditions, setConditions] = useState<Condition[]>(parsed ?? []);
  const [view, setView] = useState<ListView>();
  const onView = useCallback((v: ListView) => setView(v), []);
  // The record in the preview beside the list (UI30).
  const [picked, setPicked] = useState<string>();
  const where = raw ?? toWhere(type, conditions);
  const columns = String(saved?.["columns"] ?? "")
    .split(",")
    .map((c) => c.trim())
    .filter(Boolean);
  return (
    <div className="flex flex-col gap-4">
      <SearchBar typeName={typeName} current={saved} where={where} view={view} />
      {raw ? (
        <Alert>{t("search.raw", { where: JSON.stringify(raw) })}</Alert>
      ) : (
        <Filters type={type} conditions={conditions} onChange={setConditions} />
      )}
      {/* self-start: beside the open preview the list keeps the height of its rows instead of
          stretching to the height of the preview (UI46). */}
      <div className="flex flex-col gap-4 lg:flex-row">
        <div className="min-w-0 flex-1 lg:self-start">
          <List
            type={type}
            where={where}
            view={saved ? { columns, sort: String(saved["sort"] ?? "") || undefined } : undefined}
            onView={onView}
            picked={picked}
            onPick={setPicked}
          />
        </div>
        {picked && (
          <Preview key={picked} type={type} id={picked} onClose={() => setPicked(undefined)} />
        )}
      </div>
    </div>
  );
}

export function TypePage() {
  const { t, i18n } = useTranslation();
  const { typeName } = useParams({ from: "/shell/types/$typeName" });
  const { search, where } = useSearch({ from: "/shell/types/$typeName" });
  const types = useQuery(typesQuery);
  const me = useQuery(meQuery);
  const saved = useQuery({ ...recordQuery(searchType, search ?? ""), enabled: Boolean(search) });
  const type = types.data?.find((candidate) => qualifiedName(candidate) === typeName);
  if (types.isPending || (search && saved.isPending)) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  return (
    <>
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">
          {labelOf(type.label, i18n.language, typeName)}
          {saved.data && (
            <span className="text-muted-foreground"> · {String(saved.data["name"])}</span>
          )}
        </h1>
        {/* The editor of the type (UI25) for those who may change types. */}
        {me.data?.customize === true && (
          <div className="flex gap-4">
            <Link
              to="/types/$typeName/customize"
              params={{ typeName }}
              className="text-sm underline"
            >
              {t("customize.open")}
            </Link>
            <Link
              to="/types/$typeName/workflow"
              params={{ typeName }}
              className="text-sm underline"
            >
              {t("workflow.open")}
            </Link>
          </div>
        )}
      </div>
      {search && saved.isError && <Alert variant="destructive">{t("search.notFound")}</Alert>}
      <TypeView
        key={`${typeName}-${search ?? ""}-${String(saved.data?.["version"] ?? "")}-${where ?? ""}`}
        type={type}
        saved={saved.data}
        initialWhere={where}
      />
    </>
  );
}
