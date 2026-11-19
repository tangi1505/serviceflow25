// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The list of a type, rendered from its definition (architektur.md §10): TanStack Table for
// the columns, TanStack Virtual for the rows, pages by key through TanStack Query's infinite
// query (never by offset, E12). Column choice is kept per type in localStorage. With a
// condition (UI4) the pages come from the JSON query; a saved search brings its columns,
// their order and its sort, and then the browser keeps nothing. Rows can be chosen one by
// one or all loaded at once for a mass action (UI6). Live (UI8): rows that someone else
// changes come again, gathered every 2 seconds; new records only show as a note, so that the
// list does not jump while one reads it.
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  type ColumnDef,
  type ColumnOrderState,
  flexRender,
  getCoreRowModel,
  type RowSelectionState,
  useReactTable,
  type VisibilityState,
} from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Columns3, Plus } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  meQuery,
  mySettingsQuery,
  qualifiedName,
  type Page,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { useLive } from "@/app/live";
import { usePageShortcuts } from "@/app/shortcuts";
import { Button } from "@/ui/button";
import { PriorityMark, StatePill } from "@/ui/marks";
import { Menu, MenuCheck, MenuLabel, MenuLink, primaryBarButton } from "@/ui/menu";
import { cn } from "@/ui/utils";

import { BatchBar } from "./Batch";
import { DeadlineCell } from "./DeadlineCell";
import { ExportButton } from "./Export";
import { workplaceFormsQuery } from "./FormPage";
import { fieldLabel, formatValue, sortableColumns, stateCategory, systemColumns } from "./fields";

const pageSize = 100;
// Row heights of the kit (UI30, §2.1): compact 36 px, normal 52 px, per person.
const rowHeights = { compact: 36, normal: 52 } as const;

type Sort = { column: string; desc: boolean };

// ListView is what a saved search keeps of a list: the shown columns in order, and the sort.
export type ListView = { columns: string[]; sort: string };

function parseSort(sort: string | undefined): Sort {
  if (!sort) {
    return { column: "updated_at", desc: true };
  }
  return sort.startsWith("-")
    ? { column: sort.slice(1), desc: true }
    : { column: sort, desc: false };
}

function storageKey(typeName: string) {
  return `itsm.list.${typeName}.columns`;
}

function loadVisibility(typeName: string, type: TypeDefinition): VisibilityState {
  const initial: VisibilityState = {};
  for (const c of systemColumns) {
    // The deadline shows where clocks run (UI34).
    initial[c.name] = c.name === "updated_at" || (c.name === "sla_due_at" && type.timed === true);
  }
  for (const f of type.fields) {
    initial[f.name] = true;
  }
  try {
    const stored = localStorage.getItem(storageKey(typeName));
    if (!stored) {
      return initial;
    }
    // Only columns the type still has: a field the customer layer took back (UI25) would
    // otherwise stay in the request and the API would refuse the list (422).
    const kept = Object.entries(JSON.parse(stored) as VisibilityState).filter(
      ([name]) => name in initial,
    );
    return { ...initial, ...Object.fromEntries(kept) };
  } catch {
    return initial;
  }
}

// NewMenu is the button "Neu" in the head of the list (UI47, UI-Plan E54); with forms of the
// workplace for the type (SD9, mockup A) it opens a menu: the form with all fields first, then
// the forms such as the phone note.
export function NewMenu({ type }: { type: TypeDefinition }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const typeName = qualifiedName(type);
  const forms = useQuery(workplaceFormsQuery);
  const mine = (forms.data ?? []).filter((f) => f.target_type === typeName);
  const plus = <Plus aria-hidden="true" className="size-4" />;
  if (mine.length === 0) {
    // An action, so a button, though it leads to the page of the new record (E56).
    return (
      <button
        type="button"
        className={primaryBarButton}
        onClick={() => void navigate({ to: "/types/$typeName/new", params: { typeName } })}
      >
        {plus}
        {t("list.new")}
      </button>
    );
  }
  return (
    <Menu
      label={t("list.newMenu")}
      primary
      trigger={
        <>
          {plus}
          {t("list.new")}
        </>
      }
      width="w-72"
    >
      <MenuLink to="/types/$typeName/new" params={{ typeName }}>
        {t("list.newFull", { type: labelOf(type.label, i18n.language, typeName) })}
      </MenuLink>
      <MenuLabel>{t("list.forms")}</MenuLabel>
      {mine.map((f) => (
        <MenuLink key={f.id} to="/forms/$formId" params={{ formId: f.id }}>
          {labelOf(f.definition.title, i18n.language, f.name)}
        </MenuLink>
      ))}
    </Menu>
  );
}

export function List({
  type,
  where,
  view,
  onView,
  picked,
  onPick,
  onCount,
  toolbarStart,
}: {
  type: TypeDefinition;
  // where is the condition of the JSON query; without one the plain list.
  where?: Record<string, unknown>;
  // view is the columns and sort of a saved search; without one they come from the browser.
  view?: Partial<ListView>;
  onView?: (view: ListView) => void;
  // picked is the record in the preview; onPick shows one there (UI30). Without onPick, or
  // on a narrow screen, a click opens the record page.
  picked?: string;
  onPick?: (id: string) => void;
  // onCount gets the line "120 Datensätze" for the head of the page (UI47).
  onCount?: (line: string) => void;
  // toolbarStart stands left in the bar above the table: the saved searches and the filter
  // row (UI48, UI-Plan E54); "Ansicht" and "Exportieren" stand right.
  toolbarStart?: ReactNode;
}) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const typeName = qualifiedName(type);
  const [sort, setSort] = useState<Sort>(() => parseSort(view?.sort));
  const [visibility, setVisibility] = useState<VisibilityState>(() => {
    const base = loadVisibility(typeName, type);
    const chosen = view?.columns;
    if (!chosen?.length) {
      return base;
    }
    return Object.fromEntries(Object.keys(base).map((k) => [k, chosen.includes(k)]));
  });
  const [order] = useState<ColumnOrderState>(() => ["select", ...(view?.columns ?? [])]);
  const [selection, setSelection] = useState<RowSelectionState>({});
  const ownView = (view?.columns?.length ?? 0) > 0;
  const settings = useQuery(mySettingsQuery);
  const density = settings.data?.density?.value === "compact" ? "compact" : "normal";
  const rowHeight = rowHeights[density];
  const settingsClient = useQueryClient();
  const setDensity = useMutation({
    mutationFn: (d: "compact" | "normal") => api.setMySettings({ density: d }),
    onSuccess: (saved) => settingsClient.setQueryData(mySettingsQuery.queryKey, saved),
  });
  const parentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (ownView) {
      // A saved search owns its columns; the browser's choice stays as it was.
      return;
    }
    try {
      localStorage.setItem(storageKey(typeName), JSON.stringify(visibility));
    } catch {
      // storage blocked: the choice lives for this page only
    }
  }, [typeName, visibility, ownView]);

  const fields = useMemo(() => {
    const sortParam = (sort.desc ? "-" : "") + sort.column;
    const shown = Object.entries(visibility)
      .filter(([, on]) => on)
      .map(([name]) => name);
    // The deadline mark needs the three-quarter point and the pause too (UI34).
    const columns = shown.includes("sla_due_at") ? [...shown, "sla_warn_at", "sla_paused"] : shown;
    return { sortParam, columns };
  }, [sort, visibility]);

  const whereKey = where ? JSON.stringify(where) : "";
  const listKey = [
    "records",
    typeName,
    "list",
    fields.sortParam,
    fields.columns.join(","),
    whereKey,
  ];
  const query = useInfiniteQuery({
    queryKey: listKey,
    queryFn: ({ pageParam }) =>
      where
        ? api.query(typeName, {
            where,
            limit: pageSize,
            sort: [fields.sortParam],
            fields: fields.columns,
            ...(pageParam ? { after: pageParam } : {}),
          })
        : api.list(typeName, {
            limit: pageSize,
            sort: fields.sortParam,
            fields: fields.columns.join(","),
            ...(pageParam ? { after: pageParam } : {}),
          }),
    initialPageParam: "",
    getNextPageParam: (last) => last.next ?? undefined,
  });

  const rows = useMemo(() => query.data?.pages.flatMap((p) => p.items) ?? [], [query.data]);

  // Live (UI8): changes by others to loaded rows, gathered for 2 seconds and read again with
  // the condition of the list (a row that no longer matches leaves); new records as a note.
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const [fresh, setFresh] = useState(0);
  const pending = useRef(new Set<string>());
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const refresh = async () => {
    timer.current = null;
    const ids = [...pending.current];
    pending.current.clear();
    if (ids.length === 0) {
      return;
    }
    const byId: Record<string, unknown> = { field: "id", in: ids };
    const page = await api.query(typeName, {
      where: where ? { and: [where, byId] } : byId,
      fields: fields.columns,
      limit: ids.length,
    });
    queryClient.setQueryData<InfiniteData<Page>>(listKey, (data) =>
      data
        ? {
            ...data,
            pages: data.pages.map((p) => ({
              ...p,
              items: p.items.flatMap((it) => {
                if (!ids.includes(String(it["id"]))) {
                  return [it];
                }
                const now = page.items.find((x) => x["id"] === it["id"]);
                return now ? [now] : [];
              }),
            })),
          }
        : data,
    );
  };
  useLive((e) => {
    if (e.type_name !== typeName || e.actor === me.data?.id) {
      return;
    }
    if (e.kind === "create") {
      setFresh((n) => n + 1);
      return;
    }
    if (!rows.some((r) => r["id"] === e.record_id)) {
      return;
    }
    pending.current.add(e.record_id);
    timer.current ??= setTimeout(() => void refresh().catch(() => undefined), 2000);
  });
  useEffect(
    () => () => {
      if (timer.current) {
        clearTimeout(timer.current);
      }
    },
    [],
  );

  const columns = useMemo<ColumnDef<RecordData>[]>(() => {
    const system = systemColumns.map<ColumnDef<RecordData>>((c) => ({
      id: c.name,
      accessorFn: (row) => row[c.name],
      header: t(`column.${c.name}`),
      cell: (info) => {
        if (c.name === "sla_due_at") {
          return <DeadlineCell row={info.row.original} />;
        }
        // The state as a pill with the dot of its kind (UI29).
        if (c.name === "state" && info.getValue()) {
          const s = type.states?.find((x) => x.name === info.getValue());
          return (
            <StatePill
              label={s ? labelOf(s.label, i18n.language, s.name) : String(info.getValue())}
              category={stateCategory(s)}
            />
          );
        }
        return c.name.endsWith("_at")
          ? formatValue({ name: c.name, type: "datetime" }, info.getValue(), i18n.language)
          : String(info.getValue() ?? "");
      },
    }));
    // Fields the customer layer hid (PL43) are no column.
    const own = type.fields
      .filter((f) => !f.hidden)
      .map<ColumnDef<RecordData>>((f) => ({
        id: f.name,
        accessorFn: (row) => row[f.name],
        header: fieldLabel(f, i18n.language),
        cell: (info) =>
          f.format === "priority" && typeof info.getValue() === "number" ? (
            <PriorityMark value={info.getValue() as number} />
          ) : (
            formatValue(f, info.getValue(), i18n.language)
          ),
      }));
    // The choice for mass actions (UI6): a box per row and one for all loaded rows.
    const titleField = type.title_field ?? type.fields[0]?.name ?? "id";
    const choose: ColumnDef<RecordData> = {
      id: "select",
      enableHiding: false,
      header: ({ table }) => (
        <input
          type="checkbox"
          className="h-4 w-4"
          aria-label={t("list.selectAll")}
          checked={table.getIsAllRowsSelected()}
          ref={(el) => {
            if (el) {
              el.indeterminate = table.getIsSomeRowsSelected();
            }
          }}
          onChange={table.getToggleAllRowsSelectedHandler()}
        />
      ),
      cell: ({ row }) => (
        <input
          type="checkbox"
          className="h-4 w-4"
          aria-label={t("list.select", { title: String(row.original[titleField] ?? row.id) })}
          checked={row.getIsSelected()}
          onClick={(e) => e.stopPropagation()}
          onKeyDown={(e) => e.stopPropagation()}
          onChange={row.getToggleSelectedHandler()}
        />
      ),
    };
    return [choose, ...own, ...system];
  }, [type, t, i18n.language]);

  const table = useReactTable({
    data: rows,
    columns,
    state: { columnVisibility: visibility, columnOrder: order, rowSelection: selection },
    onColumnVisibilityChange: setVisibility,
    onRowSelectionChange: setSelection,
    enableRowSelection: true,
    getCoreRowModel: getCoreRowModel(),
    getRowId: (row) => String(row["id"]),
  });

  // The view as it is now, in the order shown, for saving it as a search (UI4).
  const shownColumns = table
    .getVisibleLeafColumns()
    .map((c) => c.id)
    .filter((c) => c !== "select")
    .join(",");
  const chosen = rows.filter((r) => selection[String(r["id"])]);
  const countLine = query.isPending
    ? t("list.loading")
    : t("list.loaded", { count: rows.length, more: query.hasNextPage ? "+" : "" });
  useEffect(() => {
    onCount?.(countLine);
  }, [onCount, countLine]);
  useEffect(() => {
    onView?.({ columns: shownColumns.split(","), sort: fields.sortParam });
  }, [onView, shownColumns, fields.sortParam]);

  const tableRows = table.getRowModel().rows;
  const virtualizer = useVirtualizer({
    count: tableRows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowHeight,
    overscan: 10,
  });
  // Before the first measurement (and in jsdom, which has no layout) the virtualizer reports
  // nothing; then the first rows are shown plainly so that the list is never empty.
  const measured = virtualizer.getVirtualItems();
  const virtualRows =
    measured.length > 0 || tableRows.length === 0
      ? measured
      : tableRows.slice(0, 30).map((_, index) => ({
          index,
          key: index,
          start: index * rowHeight,
          size: rowHeight,
          end: (index + 1) * rowHeight,
          lane: 0,
        }));

  // Another row height measures the rows anew.
  useEffect(() => {
    virtualizer.measure();
  }, [rowHeight, virtualizer]);

  // Load the next page when the last rendered row comes into view.
  const lastIndex = virtualRows[virtualRows.length - 1]?.index ?? -1;
  useEffect(() => {
    if (lastIndex >= tableRows.length - 20 && query.hasNextPage && !query.isFetchingNextPage) {
      void query.fetchNextPage();
    }
  }, [lastIndex, tableRows.length, query]);

  const toggleSort = (column: string) => {
    if (!sortableColumns.includes(column)) {
      return;
    }
    setSort((s) => (s.column === column ? { column, desc: !s.desc } : { column, desc: false }));
  };

  // goTo moves the focus to a row; one outside the window of rows comes into view first.
  const goTo = (index: number) => {
    if (index < 0 || index >= tableRows.length) {
      return;
    }
    const find = () =>
      parentRef.current?.querySelector<HTMLTableRowElement>(`tbody tr[data-index="${index}"]`);
    const now = find();
    if (now) {
      now.focus();
      return;
    }
    virtualizer.scrollToIndex(index);
    requestAnimationFrame(() => find()?.focus());
  };
  // What the keys of the list do, for the dialog "Tastenkürzel" (UI52, UI53).
  usePageShortcuts([
    { keys: ["↑", "↓"], label: t("keys.rows") },
    { keys: ["Enter"], label: t("keys.preview") },
    { keys: [t("keys.shift"), "Enter"], label: t("keys.open") },
    { keys: [t("keys.space")], label: t("keys.choose") },
  ]);

  const open = (row: RecordData) =>
    void navigate({ to: "/types/$typeName/$id", params: { typeName, id: String(row["id"]) } });
  // pick shows the record in the preview where there is room for it (UI30, from 1100 px).
  const pick = (row: RecordData) => {
    if (onPick && window.innerWidth >= 1100) {
      onPick(String(row["id"]));
    } else {
      open(row);
    }
  };

  return (
    <div className="flex flex-col gap-3">
      {/* One bar (UI48, UI-Plan E54): left the searches and the filters, right "Ansicht" and
          "Exportieren"; it wraps on a narrow screen. */}
      <div className="flex flex-wrap items-center gap-2">
        {toolbarStart}
        {!onCount && (
          <span className="text-sm text-foreground-2" aria-live="polite">
            {countLine}
          </span>
        )}
        {query.isError && <span className="text-sm text-destructive">{t("list.error")}</span>}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          <Menu
            label={t("list.view")}
            title={t("list.viewHint")}
            trigger={
              <>
                <Columns3 aria-hidden="true" className="size-4 text-foreground-2" />
                {t("list.view")}
              </>
            }
          >
            {/* The row height, where the organisation lets the person choose it (UI39). */}
            {settings.data?.allowed?.density !== false && (
              <>
                <MenuLabel>{t("list.density")}</MenuLabel>
                <div
                  role="group"
                  aria-label={t("list.density")}
                  className="mx-1 mb-1 flex rounded-md bg-muted p-0.5"
                >
                  {(["compact", "normal"] as const).map((d) => (
                    <button
                      key={d}
                      type="button"
                      role="menuitemradio"
                      aria-checked={density === d}
                      disabled={setDensity.isPending}
                      onClick={() => setDensity.mutate(d)}
                      className={cn(
                        "h-7 flex-1 rounded-[6px] px-3 text-[13px] font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        density === d
                          ? "bg-segment-active text-foreground shadow-sm"
                          : "text-foreground-2",
                      )}
                    >
                      {t(`list.densities.${d}`)}
                    </button>
                  ))}
                </div>
              </>
            )}
            <MenuLabel>{t("list.columns")}</MenuLabel>
            <div
              role="group"
              aria-label={t("list.columns")}
              className="flex max-h-72 flex-col overflow-y-auto"
            >
              {table
                .getAllLeafColumns()
                .filter((column) => column.getCanHide())
                .map((column) => (
                  <MenuCheck
                    key={column.id}
                    kind="checkbox"
                    keepOpen
                    checked={column.getIsVisible()}
                    onSelect={() => column.toggleVisibility()}
                  >
                    {typeof column.columnDef.header === "string"
                      ? column.columnDef.header
                      : column.id}
                  </MenuCheck>
                ))}
            </div>
          </Menu>
          <ExportButton
            typeName={typeName}
            where={where}
            columns={shownColumns.split(",")}
            sort={fields.sortParam}
          />
        </div>
      </div>
      {fresh > 0 && (
        <div role="status" className="flex items-center gap-2 text-sm">
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setFresh(0);
              void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
            }}
          >
            {t("live.fresh", { count: fresh })}
          </Button>
        </div>
      )}
      {chosen.length > 0 && <BatchBar type={type} rows={chosen} />}
      <div
        ref={parentRef}
        className="relative overflow-auto rounded-md border"
        // As high as its rows, at most what the window leaves: a box of its own height never
        // shows a vertical scroll bar for rows that fit, also not when the horizontal one takes
        // its room (UI46).
        style={{ maxHeight: "calc(100vh - 12rem)" }}
      >
        <table className="w-full border-collapse text-sm" role="grid" aria-rowcount={rows.length}>
          <thead className="sticky top-0 z-[1] block bg-muted">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="flex w-full">
                {hg.headers.map((header) => {
                  const sortable = sortableColumns.includes(header.column.id);
                  const active = sort.column === header.column.id;
                  return (
                    <th
                      key={header.id}
                      scope="col"
                      className={cn(
                        "truncate px-3 py-2 text-left font-medium",
                        header.column.id === "select" ? "w-10 flex-none" : "flex-1",
                      )}
                      aria-sort={active ? (sort.desc ? "descending" : "ascending") : undefined}
                    >
                      {sortable ? (
                        <button
                          type="button"
                          className="inline-flex items-center gap-1"
                          onClick={() => toggleSort(header.column.id)}
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <span aria-hidden="true">{active ? (sort.desc ? "▼" : "▲") : ""}</span>
                        </button>
                      ) : (
                        flexRender(header.column.columnDef.header, header.getContext())
                      )}
                    </th>
                  );
                })}
              </tr>
            ))}
          </thead>
          <tbody style={{ height: virtualizer.getTotalSize() }} className="relative block">
            {virtualRows.map((virtualRow) => {
              const row = tableRows[virtualRow.index];
              if (!row) {
                return null;
              }
              return (
                <tr
                  key={row.id}
                  data-index={virtualRow.index}
                  className={cn(
                    // The border lies inside the height of the row, the cells have no padding of
                    // their own above and below and are cut at the row: nothing reaches past the
                    // last row, so a scaled screen shows no vertical bar for rows that fit (UI46).
                    "absolute left-0 flex w-full cursor-pointer items-center overflow-hidden border-t hover:bg-surface-active focus-visible:bg-surface-active",
                    picked === row.id && "bg-accent-soft hover:bg-accent-soft",
                  )}
                  style={{ transform: `translateY(${virtualRow.start}px)`, height: rowHeight }}
                  tabIndex={0}
                  aria-selected={onPick ? picked === row.id : undefined}
                  onClick={() => pick(row.original)}
                  onDoubleClick={() => open(row.original)}
                  onKeyDown={(e) => {
                    // The keyboard of the list (UI53, UI-Plan E55): arrows change the row, Enter
                    // shows the preview, Umschalt Enter opens the record, Space chooses it.
                    if (e.target !== e.currentTarget) {
                      return;
                    }
                    if (e.key === "Enter" && e.shiftKey) {
                      e.preventDefault();
                      open(row.original);
                    } else if (e.key === "Enter") {
                      e.preventDefault();
                      pick(row.original);
                    } else if (e.key === " ") {
                      e.preventDefault();
                      row.toggleSelected();
                    } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                      e.preventDefault();
                      goTo(virtualRow.index + (e.key === "ArrowDown" ? 1 : -1));
                    } else if (e.key === "Home" || e.key === "End") {
                      e.preventDefault();
                      goTo(e.key === "Home" ? 0 : tableRows.length - 1);
                    }
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td
                      key={cell.id}
                      className={cn(
                        "truncate px-3",
                        cell.column.id === "select" ? "w-10 flex-none" : "flex-1",
                      )}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
