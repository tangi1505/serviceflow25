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
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  meQuery,
  qualifiedName,
  type Page,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { useLive } from "@/app/live";
import { Button } from "@/ui/button";
import { cn } from "@/ui/utils";

import { BatchBar } from "./Batch";
import { ExportButton } from "./Export";
import { fieldLabel, formatValue, sortableColumns, systemColumns } from "./fields";

const pageSize = 100;
const rowHeight = 36;

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
    initial[c.name] = c.name === "updated_at";
  }
  for (const f of type.fields) {
    initial[f.name] = true;
  }
  try {
    const stored = localStorage.getItem(storageKey(typeName));
    return stored ? { ...initial, ...(JSON.parse(stored) as VisibilityState) } : initial;
  } catch {
    return initial;
  }
}

export function List({
  type,
  where,
  view,
  onView,
}: {
  type: TypeDefinition;
  // where is the condition of the JSON query; without one the plain list.
  where?: Record<string, unknown>;
  // view is the columns and sort of a saved search; without one they come from the browser.
  view?: Partial<ListView>;
  onView?: (view: ListView) => void;
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
  const chooserId = useId();
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
    const columns = Object.entries(visibility)
      .filter(([, shown]) => shown)
      .map(([name]) => name);
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
      cell: (info) =>
        c.name.endsWith("_at")
          ? formatValue({ name: c.name, type: "datetime" }, info.getValue(), i18n.language)
          : String(info.getValue() ?? ""),
    }));
    // Fields the customer layer hid (PL43) are no column.
    const own = type.fields
      .filter((f) => !f.hidden)
      .map<ColumnDef<RecordData>>((f) => ({
        id: f.name,
        accessorFn: (row) => row[f.name],
        header: fieldLabel(f, i18n.language),
        cell: (info) => formatValue(f, info.getValue(), i18n.language),
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

  const open = (row: RecordData) =>
    void navigate({ to: "/types/$typeName/$id", params: { typeName, id: String(row["id"]) } });

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => void navigate({ to: "/types/$typeName/new", params: { typeName } })}>
          {t("list.new")}
        </Button>
        <details className="relative">
          <summary className="cursor-pointer rounded-md border px-3 py-2 text-sm">
            {t("list.columns")}
          </summary>
          <fieldset
            id={chooserId}
            className="absolute z-10 mt-1 flex max-h-80 flex-col gap-1 overflow-auto rounded-md border bg-card p-3 shadow"
          >
            <legend className="sr-only">{t("list.columns")}</legend>
            {table
              .getAllLeafColumns()
              .filter((column) => column.getCanHide())
              .map((column) => (
                <label key={column.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={column.getIsVisible()}
                    onChange={column.getToggleVisibilityHandler()}
                  />
                  {typeof column.columnDef.header === "string"
                    ? column.columnDef.header
                    : column.id}
                </label>
              ))}
          </fieldset>
        </details>
        <span className="text-sm text-muted-foreground" aria-live="polite">
          {query.isPending
            ? t("list.loading")
            : t("list.loaded", { count: rows.length, more: query.hasNextPage ? "+" : "" })}
        </span>
        {query.isError && <span className="text-sm text-destructive">{t("list.error")}</span>}
        <ExportButton
          typeName={typeName}
          where={where}
          columns={shownColumns.split(",")}
          sort={fields.sortParam}
        />
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
        className="relative flex-1 overflow-auto rounded-md border"
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
                    "absolute left-0 flex w-full cursor-pointer border-t hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none",
                  )}
                  style={{ transform: `translateY(${virtualRow.start}px)`, height: rowHeight }}
                  tabIndex={0}
                  onClick={() => open(row.original)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      open(row.original);
                    }
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td
                      key={cell.id}
                      className={cn(
                        "truncate px-3 py-2",
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
