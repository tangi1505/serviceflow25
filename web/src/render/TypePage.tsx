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
import { useNavigate, useParams, useSearch } from "@tanstack/react-router";
import { Bookmark, SlidersHorizontal, Workflow } from "lucide-react";
import { useCallback, useEffect, useId, useRef, useState } from "react";
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
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import {
  barButton,
  Menu,
  MenuCheck,
  MenuItem,
  MenuLabel,
  MenuLink,
  MenuSeparator,
  primaryBarButton,
} from "@/ui/menu";
import { PageHeader } from "@/ui/page-header";
import { isTyping, usePageShortcuts } from "@/app/shortcuts";

import { fromWhere, toWhere, type Condition } from "./Filters";
import { FilterRow } from "./FilterRow";
import { List, NewMenu, type ListView } from "./List";
import { Preview } from "./Preview";
import { mayDelete, useTrash } from "./trash";

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

// SearchMenu is the button of the saved searches in front of the filters (UI49, UI-Plan E54):
// it shows the search shown now and a dot when filters, columns or sort differ from it. Its
// menu lists one's own and the shared searches, then saves or discards the changes (saving
// only one's own), saves as a new search, and leads to sharing. It replaces the list and the
// buttons of UI4.
function SearchMenu({
  typeName,
  current,
  where,
  view,
  dirty,
  onReset,
}: {
  typeName: string;
  current?: RecordData;
  where?: Record<string, unknown>;
  view?: ListView;
  dirty: boolean;
  onReset: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const searches = useQuery(searchesQuery(typeName));
  // Deleting the shown search (UI62) needs the right delete on saved searches.
  const searchDef = useQuery(typesQuery).data?.find((c) => qualifiedName(c) === searchType);
  const trash = useTrash();
  const formId = useId();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState("");
  // "Als neue Suche speichern" puts the focus into the name.
  const nameField = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (naming) {
      nameField.current?.focus();
    }
  }, [naming]);
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
  const currentId = current ? String(current["id"]) : "";
  const pick = (s: RecordData) => (
    <MenuCheck
      key={String(s["id"])}
      checked={String(s["id"]) === currentId}
      onSelect={() => open(String(s["id"]))}
    >
      {String(s["name"])}
    </MenuCheck>
  );

  return (
    <>
      <Menu
        label={t("search.saved")}
        buttonLabel={t(dirty ? "search.buttonChanged" : "search.button", {
          name: current ? String(current["name"]) : t("search.allShort"),
        })}
        align="start"
        width="w-72"
        title={t("search.saved")}
        trigger={
          <>
            <Bookmark aria-hidden="true" className="size-4 text-foreground-2" />
            <span className="max-w-48 truncate">
              {current ? String(current["name"]) : t("search.allShort")}
            </span>
            {dirty && (
              <span aria-hidden="true" className="size-1.5 flex-none rounded-full bg-accent" />
            )}
          </>
        }
      >
        <MenuCheck checked={currentId === ""} onSelect={() => open(undefined)}>
          {t("search.allShort")}
        </MenuCheck>
        {own.length > 0 && <MenuLabel>{t("search.own")}</MenuLabel>}
        {own.map(pick)}
        {shared.length > 0 && <MenuLabel>{t("search.shared")}</MenuLabel>}
        {shared.map(pick)}
        <MenuSeparator />
        {dirty && mine && (
          <MenuItem className="font-medium" onSelect={() => update.mutate()}>
            {t("search.update")}
          </MenuItem>
        )}
        {dirty && <MenuItem onSelect={onReset}>{t("search.discard")}</MenuItem>}
        <MenuItem onSelect={() => setNaming(true)}>{t("search.saveAsMore")}</MenuItem>
        {current && (
          <MenuLink to="/types/$typeName/$id" params={{ typeName: searchType, id: currentId }}>
            {mine ? t("search.shareMore") : t("search.detailsMore")}
          </MenuLink>
        )}
        {current && searchDef && mayDelete(searchDef) && (
          <MenuItem
            className="text-destructive"
            onSelect={() =>
              void trash(searchType, currentId, String(current["name"]), () => open(undefined))
            }
          >
            {t("search.delete")}
          </MenuItem>
        )}
      </Menu>
      {naming && (
        <form
          className="flex flex-wrap items-center gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) {
              create.mutate();
            }
          }}
        >
          <Label htmlFor={`${formId}-name`} className="sr-only">
            {t("search.name")}
          </Label>
          <Input
            id={`${formId}-name`}
            className="h-8 w-56"
            placeholder={t("search.name")}
            maxLength={100}
            ref={nameField}
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setNaming(false);
              }
            }}
          />
          <button
            type="submit"
            className={primaryBarButton}
            disabled={!name.trim() || create.isPending}
          >
            {t("search.save")}
          </button>
          <button type="button" className={barButton} onClick={() => setNaming(false)}>
            {t("form.cancel")}
          </button>
        </form>
      )}
      {failure && (
        <Alert variant="destructive" className="basis-full">
          {failure}
        </Alert>
      )}
    </>
  );
}

// sameView says whether the list shows what the saved search keeps: the same condition, and
// the columns and sort it names (a search without columns leaves them to the browser).
function sameView(
  saved: RecordData | undefined,
  base: unknown,
  where: Record<string, unknown> | undefined,
  view: ListView | undefined,
) {
  if (JSON.stringify(base ?? null) !== JSON.stringify(where ?? null)) {
    return false;
  }
  if (!saved || !view) {
    return true;
  }
  const columns = String(saved["columns"] ?? "");
  const sort = String(saved["sort"] ?? "");
  return (
    (columns === "" || columns === view.columns.join(",")) && (sort === "" || sort === view.sort)
  );
}

// TypeView is the list with its filters; another saved search makes a new one (key).
function TypeView({
  type,
  saved,
  initialWhere,
  onCount,
  onReset,
}: {
  type: TypeDefinition;
  saved?: RecordData;
  // initialWhere is a condition from the address, such as a click on a dashboard (UI13).
  initialWhere?: string;
  onCount: (line: string) => void;
  // onReset shows the saved search again as it is stored (UI49, "Änderungen verwerfen").
  onReset: () => void;
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
  // Esc closes the preview (UI52), when no menu or dialog took it before.
  useEffect(() => {
    if (!picked) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !e.defaultPrevented && !isTyping(e.target)) {
        setPicked(undefined);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [picked]);
  const where = raw ?? toWhere(type, conditions);
  // Changed against the saved search (UI49): the condition as the filters write it, so that
  // the order of keys in the stored text does not count.
  const base = raw ?? (parsed ? toWhere(type, parsed) : undefined);
  const dirty = saved !== undefined && !sameView(saved, base, where, view);
  const columns = String(saved?.["columns"] ?? "")
    .split(",")
    .map((c) => c.trim())
    .filter(Boolean);
  return (
    <div className="flex flex-col gap-4">
      {raw && <Alert>{t("search.raw", { where: JSON.stringify(raw) })}</Alert>}
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
            onCount={onCount}
            toolbarStart={
              <>
                <SearchMenu
                  typeName={typeName}
                  current={saved}
                  where={where}
                  view={view}
                  dirty={dirty}
                  onReset={onReset}
                />
                {/* The filter row (UI50); a condition it cannot show stays a note above. */}
                {!raw && <FilterRow type={type} conditions={conditions} onChange={setConditions} />}
              </>
            }
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
  // The number of records stands under the title (UI47); the list reports it.
  const [count, setCount] = useState("");
  // A new round shows the saved search again as stored (UI49).
  const [round, setRound] = useState(0);
  const navigate = useNavigate();
  // N creates a new record, like the button "Neu" (UI52).
  usePageShortcuts([
    {
      keys: ["N"],
      key: "n",
      label: t("keys.new"),
      run: () => void navigate({ to: "/types/$typeName/new", params: { typeName } }),
    },
  ]);
  if (types.isPending || (search && saved.isPending)) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  return (
    <>
      <PageHeader
        title={
          <>
            {labelOf(type.label, i18n.language, typeName)}
            {saved.data && (
              <span className="text-muted-foreground"> · {String(saved.data["name"])}</span>
            )}
          </>
        }
        line={count}
        actions={
          <>
            {/* The editors of the type (UI25, UI26) for those who may change types (UI47). */}
            {me.data?.customize === true && (
              <Menu
                label={t("list.customize")}
                trigger={
                  <>
                    <SlidersHorizontal aria-hidden="true" className="size-4 text-foreground-2" />
                    {t("list.customize")}
                  </>
                }
                width="w-72"
              >
                <MenuLink
                  to="/types/$typeName/customize"
                  params={{ typeName }}
                  description={t("list.customizeType")}
                  icon={
                    <SlidersHorizontal
                      aria-hidden="true"
                      className="mt-0.5 size-4 flex-none text-foreground-2"
                    />
                  }
                >
                  {t("customize.open")}
                </MenuLink>
                <MenuLink
                  to="/types/$typeName/workflow"
                  params={{ typeName }}
                  description={t("list.customizeWorkflow")}
                  icon={
                    <Workflow
                      aria-hidden="true"
                      className="mt-0.5 size-4 flex-none text-foreground-2"
                    />
                  }
                >
                  {t("workflow.open")}
                </MenuLink>
              </Menu>
            )}
            <NewMenu type={type} />
          </>
        }
      />
      {search && saved.isError && <Alert variant="destructive">{t("search.notFound")}</Alert>}
      <TypeView
        key={`${typeName}-${search ?? ""}-${String(saved.data?.["version"] ?? "")}-${where ?? ""}-${round}`}
        type={type}
        saved={saved.data}
        initialWhere={where}
        onCount={setCount}
        onReset={() => setRound((r) => r + 1)}
      />
    </>
  );
}
