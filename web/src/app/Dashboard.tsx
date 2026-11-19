// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Dashboards (UI13, UI-Plan E39; your decisions 2026-11-13 after the mockup "Dashboards"): the
// filters period and unit on top, the widgets in a grid of four columns, each computed with the
// rights of the person who looks. A point of a chart opens the list behind it. The numbers come
// again when the records of their types change, at most every 30 seconds. Editing moves the
// widgets by dragging or with the keyboard, sets each widget below the grid and saves all at
// once in the bar at the top.
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Plus } from "lucide-react";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  mySettingsQuery,
  qualifiedName,
  typesQuery,
  type DashboardFilters,
  type RecordData,
  type TypeDefinition,
  type WidgetData,
} from "@/api/client";
import { useLive } from "@/app/live";
import { Chart } from "@/render/Chart";
import { Filters, fromWhere, toWhere, type Condition } from "@/render/Filters";
import { RichEditor } from "@/render/RichEditor";
import { RichText } from "@/render/RichText";
import { fieldLabel, formatValue } from "@/render/fields";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { Select } from "@/ui/select";
import { Input } from "@/ui/input";

const kinds = ["number", "pie", "bar", "line", "list", "text"] as const;
const measures = ["count", "sum", "avg", "min", "max"] as const;
const periods = ["all", "today", "days_7", "days_30", "year", "own"] as const;
type Period = (typeof periods)[number];
// isPeriod tells a period a dashboard may open with (PL65): every one but "own".
const isPeriod = (v: unknown): v is Exclude<Period, "own"> =>
  typeof v === "string" && v !== "own" && (periods as readonly string[]).includes(v);
// live says whether changes load the numbers again by themselves (PL65, Hauptplan E195): only
// for periods up to 7 days, so that a year is not counted again for each change.
export function live(p: Period, own: { from: string; to: string }): boolean {
  if (p === "today" || p === "days_7") return true;
  if (p !== "own" || !own.from || !own.to) return false;
  const days = (Date.parse(own.to) - Date.parse(own.from)) / 86_400_000 + 1;
  return days > 0 && days <= 7;
}
// refreshEvery is the least time between two reloads by changes (your decision 2026-11-13).
const refreshEvery = 30_000;

// range is the period as [from, to) for the server; own takes two days, both included.
export function range(
  p: Period,
  own: { from: string; to: string },
  now = new Date(),
): DashboardFilters {
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate());
  switch (p) {
    case "today":
      return { from: day(now).toISOString() };
    case "days_7":
      return { from: new Date(now.getTime() - 7 * 86_400_000).toISOString() };
    case "days_30":
      return { from: new Date(now.getTime() - 30 * 86_400_000).toISOString() };
    case "year":
      return { from: new Date(now.getFullYear(), 0, 1).toISOString() };
    case "own": {
      const out: DashboardFilters = {};
      if (own.from) out.from = new Date(own.from + "T00:00:00").toISOString();
      if (own.to)
        out.to = new Date(new Date(own.to + "T00:00:00").getTime() + 86_400_000).toISOString();
      return out;
    }
  }
  return {};
}

// pack places the widgets in reading order on a grid of four columns, each at the first free
// place where it fits.
export function pack(ws: { w: number; h: number }[]): { x: number; y: number }[] {
  const taken = new Set<string>();
  const free = (x: number, y: number, w: number, h: number) => {
    for (let i = x; i < x + w; i++)
      for (let j = y; j < y + h; j++) if (taken.has(`${i}:${j}`)) return false;
    return true;
  };
  return ws.map(({ w, h }) => {
    const width = Math.min(Math.max(w, 1), 4);
    for (let y = 0; ; y++) {
      for (let x = 0; x + width <= 4; x++) {
        if (free(x, y, width, h)) {
          for (let i = x; i < x + width; i++)
            for (let j = y; j < y + h; j++) taken.add(`${i}:${j}`);
          return { x, y };
        }
      }
    }
  });
}

const num = (r: RecordData, f: string, fallback: number) =>
  typeof r[f] === "number" ? (r[f] as number) : fallback;

export function DashboardPage() {
  const { id } = useParams({ from: "/shell/dashboards/$id" });
  return <DashboardScreen key={id} id={id} />;
}

// DashboardScreen shows a dashboard; embedded (the start page) without its title.
export function DashboardScreen({ id, embedded = false }: { id: string; embedded?: boolean }) {
  const { t, i18n } = useTranslation();
  const formId = useId();
  const queryClient = useQueryClient();
  const view = useQuery({ queryKey: ["dashboard", id], queryFn: () => api.dashboard(id) });
  const nodes = useQuery({ queryKey: ["org", "nodes"], queryFn: api.orgNodes });
  const settings = useQuery(mySettingsQuery);
  // The period opens as the person chose it last, else as the dashboard says, else "Gesamt"
  // (PL65); a choice here wins and is kept in the person's settings, "own" only here.
  const [chosen, setChosen] = useState<Period | null>(null);
  const remembered = settings.data?.dashboard_periods?.[id];
  const start = view.data?.dashboard["period"];
  const period: Period =
    chosen ?? (isPeriod(remembered) ? remembered : isPeriod(start) ? start : "all");
  const keep = useMutation({
    mutationFn: (p: Exclude<Period, "own">) =>
      api.setMySettings({ dashboard_periods: { [id]: p } }),
    onSuccess: (saved) => queryClient.setQueryData(mySettingsQuery.queryKey, saved),
  });
  const setPeriod = (p: Period) => {
    setChosen(p);
    if (p !== "own" && p !== period) keep.mutate(p);
  };
  const [own, setOwn] = useState({ from: "", to: "" });
  const [unit, setUnit] = useState("");
  const [editing, setEditing] = useState(false);
  const [loadedAt, setLoadedAt] = useState(() => new Date());
  const filters = useMemo(
    () => ({ ...range(period, own), ...(unit ? { unit } : {}) }),
    [period, own, unit],
  );
  // The types the widgets count: a change of one of their records loads the numbers again,
  // at most every 30 seconds, and only for periods up to 7 days (PL65).
  const typesSeen = useRef(new Set<string>());
  const liveNow = useRef(false);
  liveNow.current = live(period, own);
  const last = useRef(0);
  const pending = useRef<ReturnType<typeof setTimeout>>(undefined);
  // fresh asks the next load to compute again instead of showing the stored state (PL68).
  const fresh = useRef(false);
  const reload = (again = false) => {
    last.current = Date.now();
    fresh.current = again;
    setLoadedAt(new Date());
    void queryClient.invalidateQueries({ queryKey: ["dashboard", id, "data"] });
  };
  useLive((e) => {
    if (!liveNow.current || !typesSeen.current.has(e.type_name) || pending.current) return;
    const wait = Math.max(0, last.current + refreshEvery - Date.now());
    pending.current = setTimeout(() => {
      pending.current = undefined;
      reload();
    }, wait);
  });
  useEffect(() => () => clearTimeout(pending.current), []);

  // All widgets come in one request (PL67); each line lands in the query of its widget, so
  // that a widget shows as soon as it is ready. The old period's stream stops with its key.
  // Its result is the oldest moment of the numbers: a long period shows a stored state (PL68),
  // and "Stand" tells how old it is; while the widgets compute again, the old one stays.
  const counted = (view.data?.widgets ?? []).some((w) => w["kind"] !== "text");
  const stream = useQuery({
    queryKey: ["dashboard", id, "data", "all", filters],
    queryFn: async ({ signal }) => {
      const again = fresh.current;
      fresh.current = false;
      // Set in the callback: TypeScript would narrow a plain null to never.
      let oldest = null as string | null;
      await api.dashboardData(
        id,
        again ? { ...filters, fresh: true } : filters,
        (line) => {
          const at = line.data?.computed_at;
          if (at && (oldest === null || at < oldest)) oldest = at;
          queryClient.setQueryData<Shown>(
            ["dashboard", id, "data", line.widget, filters],
            // A line with a problem marks the widget as failed; its kind stays the widget's.
            line.data ?? {
              kind: String(
                view.data?.widgets.find((w) => w["id"] === line.widget)?.["kind"] ?? "number",
              ) as WidgetData["kind"],
              failed: true,
            },
          );
        },
        signal,
      );
      return oldest;
    },
    enabled: counted && !settings.isPending,
  });

  // The settings say the period to open with; without them the widgets would count twice.
  if (view.isPending || settings.isPending) return <p>{t("list.loading")}</p>;
  if (view.isError) {
    return (
      <Alert variant="destructive">
        {view.error instanceof ApiError && view.error.status === 404
          ? t("dashboard.notFound")
          : t("dashboard.error")}
      </Alert>
    );
  }
  const d = view.data.dashboard;
  if (editing) {
    return <DashboardEditor view={view.data} onDone={() => setEditing(false)} />;
  }
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          {embedded ? (
            <h2 className="text-xl font-semibold">{String(d["name"])}</h2>
          ) : (
            <h1 className="text-2xl font-semibold">{String(d["name"])}</h1>
          )}
          {typeof d["description"] === "string" && d["description"] !== "" && (
            <p className="text-sm text-muted-foreground">{d["description"]}</p>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          {!view.data.editable && <CopyButton id={id} name={String(d["name"])} />}
          {view.data.editable && (
            <>
              <Link
                to="/types/$typeName/$id"
                params={{ typeName: "core.dashboard", id }}
                className="inline-flex h-10 items-center rounded-md border border-input px-4 text-sm"
              >
                {t("dashboard.share")}
              </Link>
              <Button variant="outline" className="h-10" onClick={() => setEditing(true)}>
                {t("dashboard.edit")}
              </Button>
            </>
          )}
        </div>
      </div>
      <div
        role="group"
        aria-label={t("dashboard.filters")}
        className="flex flex-wrap items-end gap-4"
      >
        <div className="flex flex-col gap-1">
          <span id={`${formId}-period`} className="text-sm text-foreground-2">
            {t("dashboard.period")}
          </span>
          <div
            role="radiogroup"
            aria-labelledby={`${formId}-period`}
            className="flex flex-wrap gap-1"
          >
            {periods.map((p) => (
              <button
                key={p}
                type="button"
                role="radio"
                aria-checked={period === p}
                onClick={() => setPeriod(p)}
                className={
                  period === p
                    ? "h-9 rounded-md bg-primary px-3 text-sm text-primary-foreground"
                    : "h-9 rounded-md px-3 text-sm hover:bg-surface-active"
                }
              >
                {t(`dashboard.periods.${p}`)}
              </button>
            ))}
          </div>
        </div>
        {period === "own" && (
          <>
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${formId}-from`}>{t("dashboard.from")}</Label>
              <Input
                id={`${formId}-from`}
                type="date"
                value={own.from}
                onChange={(e) => setOwn({ ...own, from: e.target.value })}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${formId}-to`}>{t("dashboard.to")}</Label>
              <Input
                id={`${formId}-to`}
                type="date"
                value={own.to}
                onChange={(e) => setOwn({ ...own, to: e.target.value })}
              />
            </div>
          </>
        )}
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${formId}-unit`}>{t("dashboard.unit")}</Label>
          <Select
            id={`${formId}-unit`}
            value={unit}
            onChange={setUnit}
            options={[
              { value: "", label: t("dashboard.allUnits") },
              ...(nodes.data ?? []).map((n) => ({
                value: n.path,
                label: t("dashboard.unitBelow", { name: labelOf(n.label, i18n.language, n.path) }),
              })),
            ]}
          />
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <span>
            {t("dashboard.loadedAt", {
              time: (stream.data ? new Date(stream.data) : loadedAt).toLocaleTimeString(
                i18n.language,
                { hour: "2-digit", minute: "2-digit" },
              ),
            })}
          </span>
          <Button size="sm" variant="outline" onClick={() => reload(true)}>
            {t("dashboard.refresh")}
          </Button>
        </div>
      </div>
      {view.data.widgets.length === 0 ? (
        <p className="text-muted-foreground">{t("dashboard.empty")}</p>
      ) : (
        <div className="grid auto-rows-[minmax(11rem,auto)] grid-cols-1 gap-3 md:grid-cols-4">
          {view.data.widgets.map((w) => (
            <div
              key={String(w["id"])}
              className="min-w-0 md:[grid-column:var(--c)] md:[grid-row:var(--r)]"
              style={
                {
                  "--c": `${num(w, "x", 0) + 1} / span ${num(w, "w", 1)}`,
                  "--r": `${num(w, "y", 0) + 1} / span ${num(w, "h", 1)}`,
                } as CSSProperties
              }
            >
              <WidgetCard
                widget={w}
                queryKey={["dashboard", id, "data", String(w["id"]), filters]}
                load={(signal) => api.widgetData(id, String(w["id"]), filters, signal)}
                onType={(type) => typesSeen.current.add(type)}
                fed={stream.isError ? "failed" : stream.isFetching ? "loading" : "done"}
              />
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// Shown is what a widget shows; failed marks one whose line in the stream of its dashboard
// carried a problem (PL67).
type Shown = WidgetData & { failed?: boolean };

// WidgetCard computes and shows one widget: on its dashboard, or in the sidebar of a record
// (UI13b); load says how. On its dashboard the stream of all widgets feeds it (fed, PL67) and
// load is not called.
export function WidgetCard({
  widget,
  queryKey,
  load,
  onType,
  compact = false,
  fed,
}: {
  widget: RecordData;
  queryKey: unknown[];
  // load computes the widget; signal ends it when the widget is no longer wanted (PL65).
  load: (signal?: AbortSignal) => Promise<WidgetData>;
  onType?: (type: string) => void;
  compact?: boolean;
  // fed is the state of the stream that feeds the widget; without it the widget loads itself.
  fed?: "loading" | "done" | "failed";
}) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const wid = String(widget["id"]);
  const kind = String(widget["kind"]);
  const title = String(widget["title"]);
  // A query of an old period is stopped when its key is no longer shown (PL65).
  const data = useQuery<Shown>({
    queryKey,
    queryFn: ({ signal }) => load(signal),
    enabled: kind !== "text" && fed === undefined,
  });
  // Fed: no answer yet while the stream runs; none after it ended is an error.
  const failed =
    data.isError ||
    data.data?.failed === true ||
    (fed !== undefined && fed !== "loading" && !data.data);
  const types = useQuery(typesQuery);
  useEffect(() => {
    if (data.data?.type) onType?.(data.data.type);
  }, [data.data?.type, onType]);
  const type = types.data?.find((ty) => qualifiedName(ty) === data.data?.type);
  const nf = new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 });
  const format = (n: number) => nf.format(n);
  // open shows the list of the type with the widget's condition and one more.
  const open = (d: WidgetData, extra?: Record<string, unknown>) => {
    if (!d.type) return;
    const parts = [d.where, extra].filter(Boolean);
    const where = parts.length === 0 ? undefined : parts.length === 1 ? parts[0] : { and: parts };
    void navigate({
      to: "/types/$typeName",
      params: { typeName: d.type },
      search: where ? { where: JSON.stringify(where) } : {},
    });
  };
  const pointLabel = (d: WidgetData, p: NonNullable<WidgetData["points"]>[number]) => {
    if (p.key === null || p.key === undefined || p.key === "") return t("dashboard.none");
    if (p.label) return labelOf(p.label, i18n.language, String(p.key));
    if (d.group?.includes(":"))
      return new Date(String(p.key) + "T00:00:00").toLocaleDateString(i18n.language);
    const field = type?.fields.find((f) => f.name === d.group);
    return field ? formatValue(field, p.key, i18n.language) : String(p.key);
  };
  return (
    <section
      aria-labelledby={`w-${wid}`}
      className={`flex h-full min-w-0 flex-col gap-2 rounded-lg border bg-card ${compact ? "p-3" : "p-4"}`}
    >
      <h2 id={`w-${wid}`} className="truncate text-sm font-semibold" title={title}>
        {title}
      </h2>
      {kind === "text" ? (
        <RichText html={String(widget["body"] ?? "")} />
      ) : failed ? (
        <p className="text-sm text-destructive">{t("dashboard.widgetError")}</p>
      ) : !data.data ? (
        <p className="text-sm text-muted-foreground">{t("list.loading")}</p>
      ) : data.data.sidebar_only ? (
        <p className="text-sm text-muted-foreground">
          {t("dashboard.sidebarOnly", { type: String(widget["sidebar_type"] ?? "") })}
        </p>
      ) : kind === "number" ? (
        <button
          type="button"
          className="self-start text-4xl font-semibold tracking-tight hover:underline"
          onClick={() => open(data.data)}
          aria-label={t("dashboard.openNumber", {
            title,
            value: format(Number(data.data.value ?? 0)),
          })}
        >
          {format(Number(data.data.value ?? 0))}
        </button>
      ) : kind === "list" ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr>
                {(data.data.columns ?? []).map((c) => {
                  const f = type?.fields.find((x) => x.name === c);
                  return (
                    <th
                      key={c}
                      scope="col"
                      className="py-1 pr-2 text-left text-xs font-medium uppercase text-muted-foreground"
                    >
                      {f ? fieldLabel(f, i18n.language) : t(`column.${c}`, { defaultValue: c })}
                    </th>
                  );
                })}
              </tr>
            </thead>
            <tbody>
              {(data.data.items ?? []).map((item) => (
                <tr key={String(item["id"])} className="border-t">
                  {(data.data.columns ?? []).map((c, i) => {
                    const f = type?.fields.find((x) => x.name === c);
                    const text = formatValue(f, item[c], i18n.language);
                    return (
                      <td key={c} className="py-1 pr-2">
                        {i === 0 && data.data.type ? (
                          <Link
                            to="/types/$typeName/$id"
                            params={{ typeName: data.data.type, id: String(item["id"]) }}
                            className="underline"
                          >
                            {text || String(item["id"]).slice(0, 8)}
                          </Link>
                        ) : (
                          text
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
          {(data.data.items ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">{t("dashboard.noRecords")}</p>
          )}
        </div>
      ) : (data.data.points ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("dashboard.noRecords")}</p>
      ) : (
        <Chart
          kind={kind as "pie" | "bar" | "line"}
          title={title}
          format={format}
          points={(data.data.points ?? []).map((p) => ({
            label: pointLabel(data.data, p),
            value: Number(p.value ?? 0),
          }))}
          onPick={
            data.data.group && !data.data.group.includes(":")
              ? (i) => {
                  const p = data.data.points![i]!;
                  open(
                    data.data,
                    p.key === null || p.key === undefined
                      ? { field: data.data.group!, null: true }
                      : { field: data.data.group!, eq: p.key },
                  );
                }
              : undefined
          }
        />
      )}
    </section>
  );
}

// --- editing ---

type Draft = Record<string, unknown> & { key: string };

let nextKey = 0;
const draftOf = (w: RecordData): Draft => ({ ...w, key: String(w["id"] ?? `new-${nextKey++}`) });

// fieldsFor are the fields of the widget the server keeps.
const widgetFields = [
  "id",
  "title",
  "kind",
  "saved_search",
  "target_type",
  "query",
  "group_by",
  "measure",
  "measure_field",
  "date_field",
  "use_period",
  "use_unit",
  "columns",
  "sort",
  "size",
  "body",
  "sidebar_type",
  "w",
  "h",
];

function DashboardEditor({
  view,
  onDone,
}: {
  view: { dashboard: RecordData; widgets: RecordData[] };
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const formId = useId();
  const queryClient = useQueryClient();
  const d = view.dashboard;
  const id = String(d["id"]);
  const start = useMemo(() => view.widgets.map(draftOf), [view.widgets]);
  const [widgets, setWidgets] = useState<Draft[]>(start);
  const [name, setName] = useState(String(d["name"] ?? ""));
  const [description, setDescription] = useState(String(d["description"] ?? ""));
  // The period the dashboard opens with (PL65); empty is "Gesamt" as before.
  const [openWith, setOpenWith] = useState(isPeriod(d["period"]) ? String(d["period"]) : "all");
  const [selected, setSelected] = useState<string | undefined>(start[0]?.key);
  const [errors, setErrors] = useState<Record<string, Record<string, string>>>({});
  const [message, setMessage] = useState<string | null>(null);
  const dirty =
    name !== String(d["name"] ?? "") ||
    description !== String(d["description"] ?? "") ||
    openWith !== (isPeriod(d["period"]) ? String(d["period"]) : "all") ||
    JSON.stringify(widgets) !== JSON.stringify(start);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return;
    const from = widgets.findIndex((w) => w.key === e.active.id);
    const to = widgets.findIndex((w) => w.key === e.over!.id);
    setWidgets(arrayMove(widgets, from, to));
  };
  const change = (key: string, patch: Record<string, unknown>) =>
    setWidgets((ws) => ws.map((w) => (w.key === key ? { ...w, ...patch } : w)));
  const add = () => {
    const w: Draft = {
      key: `new-${nextKey++}`,
      title: t("dashboard.newWidget"),
      kind: "number",
      measure: "count",
      date_field: "created_at",
      use_period: true,
      use_unit: true,
      w: 1,
      h: 1,
      size: 10,
    };
    setWidgets([...widgets, w]);
    setSelected(w.key);
  };
  const remove = (key: string) => {
    setWidgets(widgets.filter((w) => w.key !== key));
    setSelected(undefined);
  };
  const save = () => {
    const places = pack(widgets.map((w) => ({ w: num(w, "w", 1), h: num(w, "h", 1) })));
    const body = widgets.map((w, i) => {
      const out: Record<string, unknown> = {};
      for (const f of widgetFields) {
        if (w[f] !== undefined && w[f] !== "") out[f] = w[f];
      }
      out["x"] = places[i]!.x;
      out["y"] = places[i]!.y;
      return out;
    });
    void api
      .saveDashboard(id, {
        version: Number(d["version"]),
        fields: { name, description, period: openWith },
        widgets: body,
      })
      .then(
        (saved) => {
          queryClient.setQueryData(["dashboard", id], saved);
          void queryClient.invalidateQueries({ queryKey: ["dashboard", id, "data"] });
          void queryClient.invalidateQueries({ queryKey: ["records", "core.dashboard"] });
          onDone();
        },
        (err: unknown) => {
          if (
            err instanceof ApiError &&
            err.status === 422 &&
            err.problem &&
            "errors" in err.problem
          ) {
            const byWidget: Record<string, Record<string, string>> = {};
            for (const e of err.problem.errors) {
              const m = /^widgets\[(\d+)\]\.(.+)$/.exec(e.field);
              const key = m ? widgets[Number(m[1])]?.key : "dashboard";
              if (!key) continue;
              byWidget[key] = { ...(byWidget[key] ?? {}), [m ? m[2]! : e.field]: e.message };
            }
            setErrors(byWidget);
            const first = Object.keys(byWidget).find((k) => k !== "dashboard");
            if (first) setSelected(first);
            setMessage(t("dashboard.invalid"));
            return;
          }
          if (err instanceof ApiError && err.status === 409) {
            setMessage(t("dashboard.conflict"));
            return;
          }
          setMessage(
            t("dashboard.saveError", { detail: err instanceof Error ? err.message : String(err) }),
          );
        },
      );
  };
  const current = widgets.find((w) => w.key === selected);
  return (
    <div className="flex flex-col gap-4">
      <div
        role="region"
        aria-label={t("settings.bar")}
        className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-background p-3"
      >
        <h1 className="text-xl font-semibold">
          {t("dashboard.editTitle", { name: String(d["name"]) })}
        </h1>
        <div className="flex flex-wrap items-center gap-2">
          <span
            role="status"
            className={
              dirty
                ? "text-sm font-medium text-warning-foreground"
                : "text-sm text-muted-foreground"
            }
          >
            {dirty ? t("dashboard.unsaved") : t("settings.allSaved")}
          </span>
          <Button variant="outline" className="h-10" onClick={onDone}>
            {dirty ? t("settings.discard") : t("dashboard.close")}
          </Button>
          <Button className="h-10" disabled={!dirty} onClick={save}>
            {t("form.save")}
          </Button>
        </div>
      </div>
      {message && (
        <p role="alert" className="rounded-md border bg-muted p-3 text-sm">
          {message}
        </p>
      )}
      <p className="text-sm text-muted-foreground">{t("dashboard.dragHint")}</p>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={widgets.map((w) => w.key)} strategy={rectSortingStrategy}>
          <ul
            aria-label={t("dashboard.grid")}
            className="grid grid-cols-1 gap-3 rounded-lg border border-dashed border-input bg-surface-subtle p-3 md:grid-cols-4"
          >
            {widgets.map((w) => (
              <EditCard
                key={w.key}
                widget={w}
                selected={w.key === selected}
                invalid={Boolean(errors[w.key])}
                onSelect={() => setSelected(w.key)}
              />
            ))}
            <li className="flex min-h-24 items-center justify-center rounded-lg border border-dashed border-input">
              <Button variant="outline" onClick={add}>
                {t("dashboard.addWidget")}
              </Button>
            </li>
          </ul>
        </SortableContext>
      </DndContext>
      {current && (
        <WidgetForm
          key={current.key}
          widget={current}
          errors={errors[current.key] ?? {}}
          onChange={(patch) => change(current.key, patch)}
          onRemove={() => remove(current.key)}
        />
      )}
      <section
        aria-labelledby={`${formId}-dash`}
        className="flex max-w-3xl flex-col gap-3 rounded-lg border p-4"
      >
        <h2 id={`${formId}-dash`} className="text-base font-semibold">
          {t("dashboard.dashboard")}
        </h2>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${formId}-name`}>{t("dashboard.name")}</Label>
          <Input id={`${formId}-name`} value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${formId}-desc`}>{t("dashboard.description")}</Label>
          <Input
            id={`${formId}-desc`}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${formId}-open`}>{t("dashboard.openWith")}</Label>
          <Select
            id={`${formId}-open`}
            value={openWith}
            onChange={setOpenWith}
            options={periods
              .filter((p) => p !== "own")
              .map((p) => ({ value: p, label: t(`dashboard.periods.${p}`) }))}
          />
        </div>
        <p className="text-sm text-muted-foreground">{t("dashboard.shareHint")}</p>
      </section>
    </div>
  );
}

// EditCard is a widget in the grid while editing: drag it, or move it with the keyboard.
function EditCard({
  widget,
  selected,
  invalid,
  onSelect,
}: {
  widget: Draft;
  selected: boolean;
  invalid: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({
    id: widget.key,
  });
  const w = num(widget, "w", 1);
  const h = num(widget, "h", 1);
  const title = String(widget["title"] ?? "");
  return (
    <li
      ref={setNodeRef}
      style={
        {
          transform: CSS.Transform.toString(transform),
          transition,
          "--w": String(w),
        } as CSSProperties
      }
      className="min-w-0 md:[grid-column:span_var(--w)]"
    >
      <div
        className={`flex min-h-24 flex-col gap-1 rounded-lg border bg-card p-3 ${
          selected ? "border-accent ring-1 ring-accent" : invalid ? "border-destructive" : ""
        }`}
      >
        <div className="flex min-w-0 items-center gap-2">
          <button
            type="button"
            className="cursor-grab rounded px-1 font-mono text-muted-foreground"
            aria-label={t("dashboard.move", { title })}
            {...attributes}
            {...listeners}
          >
            ⠿
          </button>
          <button
            type="button"
            className="min-w-0 truncate text-left text-sm font-semibold"
            title={title}
            onClick={onSelect}
          >
            {title}
          </button>
        </div>
        <span className="text-xs text-muted-foreground">
          {t(`dashboard.kinds.${String(widget["kind"])}`)} · {w} × {h}
          {invalid && ` · ${t("dashboard.check")}`}
        </span>
      </div>
    </li>
  );
}

// WidgetForm sets one widget: kind, source, grouping, measure, filters, size.
function WidgetForm({
  widget,
  errors,
  onChange,
  onRemove,
}: {
  widget: Draft;
  errors: Record<string, string>;
  onChange: (patch: Record<string, unknown>) => void;
  onRemove: () => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const types = useQuery(typesQuery);
  const searches = useQuery({
    queryKey: ["records", "core.saved_search", "list", "dashboard"],
    queryFn: () => api.list("core.saved_search", { limit: 200, sort: "id" }),
  });
  const kind = String(widget["kind"] ?? "number");
  const fromSearch = Boolean(widget["saved_search"]);
  const [source, setSource] = useState<"search" | "type">(fromSearch ? "search" : "type");
  const search = searches.data?.items.find((s) => String(s["id"]) === widget["saved_search"]);
  const typeName =
    source === "search"
      ? String(search?.["target_type"] ?? "")
      : String(widget["target_type"] ?? "");
  const type: TypeDefinition | undefined = types.data?.find((ty) => qualifiedName(ty) === typeName);
  const where = (() => {
    try {
      return widget["query"] ? (JSON.parse(String(widget["query"])) as unknown) : undefined;
    } catch {
      return undefined;
    }
  })();
  const parsed = fromWhere(where);
  // A widget of a sidebar names the record shown as @record: its condition is written as JSON.
  const rawMode =
    Boolean(widget["sidebar_type"]) || String(widget["query"] ?? "").includes("@record");
  const [conditions, setConditions] = useState<Condition[]>(parsed ?? []);
  const err = (f: string) =>
    errors[f] ? (
      <p id={`${id}-${f}-err`} className="text-sm text-destructive">
        {errors[f]}
      </p>
    ) : null;
  const described = (f: string) => (errors[f] ? `${id}-${f}-err` : undefined);
  const fields = type?.fields ?? [];
  const dates = [
    { value: "created_at", label: t("column.created_at") },
    { value: "updated_at", label: t("column.updated_at") },
    ...fields
      .filter((f) => f.type === "date" || f.type === "datetime")
      .map((f) => ({ value: f.name, label: fieldLabel(f, i18n.language) })),
  ];
  const groups = [
    ...(type?.states?.length ? [{ value: "state", label: t("column.state") }] : []),
    // A role groups by its holders (UI14a); a record with two counts for both.
    ...(type?.actors ?? []).map((r) => ({
      value: `actors.${r.name}`,
      label: t("dashboard.role", { role: labelOf(r.label, i18n.language, r.name) }),
    })),
    ...fields
      .filter(
        (f) =>
          ["choice", "reference", "boolean", "text", "number"].includes(f.type) &&
          f.format !== "rich",
      )
      .map((f) => ({ value: f.name, label: fieldLabel(f, i18n.language) })),
    ...dates.flatMap((d) =>
      (["day", "week", "month", "year"] as const).map((u) => ({
        value: `${d.value}:${u}`,
        label: `${d.label} (${t(`dashboard.units.${u}`)})`,
      })),
    ),
  ];
  const numbers = fields.filter((f) => f.type === "number");
  const select = (f: string, options: { value: string; label: string }[], empty?: string) => (
    <Select
      id={`${id}-${f}`}
      value={String(widget[f] ?? "")}
      invalid={Boolean(errors[f])}
      describedBy={described(f)}
      onChange={(v) => onChange({ [f]: v })}
      options={empty !== undefined ? [{ value: "", label: empty }, ...options] : options}
    />
  );
  const field = (f: string, label: string, control: ReactNode) => (
    <div className="flex min-w-0 flex-col gap-1">
      <Label htmlFor={`${id}-${f}`}>{label}</Label>
      {control}
      {err(f)}
    </div>
  );
  return (
    <section aria-labelledby={`${id}-head`} className="flex flex-col gap-4 rounded-lg border p-4">
      <h2 id={`${id}-head`} className="text-base font-semibold">
        {t("dashboard.widgetTitle", { title: String(widget["title"] ?? "") })}
      </h2>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {field(
          "title",
          t("dashboard.title"),
          <Input
            id={`${id}-title`}
            value={String(widget["title"] ?? "")}
            onChange={(e) => onChange({ title: e.target.value })}
          />,
        )}
        {field(
          "kind",
          t("dashboard.kind"),
          select(
            "kind",
            kinds.map((k) => ({ value: k, label: t(`dashboard.kinds.${k}`) })),
          ),
        )}
        <div className="grid grid-cols-2 gap-2">
          {field(
            "w",
            t("dashboard.width"),
            <Select
              id={`${id}-w`}
              value={String(num(widget, "w", 1))}
              onChange={(v) => onChange({ w: Number(v) })}
              options={[1, 2, 3, 4].map((n) => ({
                value: String(n),
                label: t("dashboard.columns", { count: n }),
              }))}
            />,
          )}
          {field(
            "h",
            t("dashboard.height"),
            <Select
              id={`${id}-h`}
              value={String(num(widget, "h", 1))}
              onChange={(v) => onChange({ h: Number(v) })}
              options={[1, 2, 3].map((n) => ({
                value: String(n),
                label: t("dashboard.rows", { count: n }),
              }))}
            />,
          )}
        </div>
      </div>
      {kind === "text" ? (
        <RichEditor
          id={`${id}-body`}
          label={t("dashboard.body")}
          value={String(widget["body"] ?? "")}
          onChange={(html) => onChange({ body: html })}
        />
      ) : (
        <>
          <fieldset className="flex flex-col gap-3 rounded-md border p-3">
            <legend className="px-1 text-sm font-semibold">{t("dashboard.source")}</legend>
            <div className="flex flex-wrap gap-4 text-sm">
              <label className="flex items-center gap-2">
                <input
                  type="radio"
                  name={`${id}-src`}
                  checked={source === "search"}
                  onChange={() => {
                    setSource("search");
                    onChange({ target_type: "", query: "" });
                  }}
                />
                {t("dashboard.fromSearch")}
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="radio"
                  name={`${id}-src`}
                  checked={source === "type"}
                  onChange={() => {
                    setSource("type");
                    onChange({ saved_search: "" });
                  }}
                />
                {t("dashboard.fromType")}
              </label>
            </div>
            {source === "search" ? (
              field(
                "saved_search",
                t("dashboard.savedSearch"),
                select(
                  "saved_search",
                  (searches.data?.items ?? []).map((s) => ({
                    value: String(s["id"]),
                    label: String(s["name"]),
                  })),
                  t("dashboard.choose"),
                ),
              )
            ) : (
              <>
                {field(
                  "target_type",
                  t("dashboard.type"),
                  <Select
                    id={`${id}-target_type`}
                    value={typeName}
                    describedBy={described("target_type")}
                    onChange={(v) => {
                      setConditions([]);
                      onChange({
                        target_type: v,
                        query: "",
                        group_by: "",
                        measure_field: "",
                      });
                    }}
                    options={[
                      { value: "", label: t("dashboard.choose") },
                      ...(types.data ?? []).map((ty) => ({
                        value: qualifiedName(ty),
                        label: labelOf(ty.label, i18n.language, qualifiedName(ty)),
                      })),
                    ]}
                  />,
                )}
                {(rawMode || parsed === null) && (
                  <div className="flex flex-col gap-1">
                    <Label htmlFor={`${id}-query`}>{t("dashboard.queryJson")}</Label>
                    <textarea
                      id={`${id}-query`}
                      className="min-h-20 rounded-md border border-input bg-background p-2 font-mono text-sm"
                      value={String(widget["query"] ?? "")}
                      aria-describedby={`${id}-query-hint ${described("query") ?? ""}`.trim()}
                      onChange={(e) => onChange({ query: e.target.value })}
                    />
                    <p id={`${id}-query-hint`} className="text-sm text-muted-foreground">
                      {t("dashboard.queryHint")}
                    </p>
                  </div>
                )}
                {type && !rawMode && parsed !== null && (
                  <Filters
                    type={type}
                    conditions={conditions}
                    onChange={(c) => {
                      setConditions(c);
                      const w = toWhere(type, c);
                      onChange({ query: w ? JSON.stringify(w) : "" });
                    }}
                  />
                )}
                {err("query")}
              </>
            )}
          </fieldset>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {(kind === "pie" || kind === "bar" || kind === "line") &&
              field(
                "group_by",
                t("dashboard.groupBy"),
                select("group_by", groups, t("dashboard.choose")),
              )}
            {kind !== "list" &&
              field(
                "measure",
                t("dashboard.measure"),
                select(
                  "measure",
                  measures.map((m) => ({ value: m, label: t(`dashboard.measures.${m}`) })),
                ),
              )}
            {kind !== "list" &&
              widget["measure"] !== "count" &&
              widget["measure"] !== undefined &&
              field(
                "measure_field",
                t("dashboard.measureField"),
                select(
                  "measure_field",
                  numbers.map((f) => ({ value: f.name, label: fieldLabel(f, i18n.language) })),
                  t("dashboard.choose"),
                ),
              )}
            {field("date_field", t("dashboard.dateField"), select("date_field", dates))}
            {field(
              "sidebar_type",
              t("dashboard.sidebarType"),
              select(
                "sidebar_type",
                (types.data ?? []).map((ty) => ({
                  value: qualifiedName(ty),
                  label: labelOf(ty.label, i18n.language, qualifiedName(ty)),
                })),
                t("dashboard.noSidebar"),
              ),
            )}
            <fieldset className="flex flex-col gap-1 text-sm">
              <legend className="font-medium">{t("dashboard.takesFilters")}</legend>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={widget["use_period"] !== false}
                  onChange={(e) => onChange({ use_period: e.target.checked })}
                />
                {t("dashboard.period")}
              </label>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={widget["use_unit"] !== false}
                  onChange={(e) => onChange({ use_unit: e.target.checked })}
                />
                {t("dashboard.unit")}
              </label>
            </fieldset>
            {kind === "list" && (
              <>
                {field(
                  "columns",
                  t("dashboard.columnsList"),
                  <Input
                    id={`${id}-columns`}
                    value={String(widget["columns"] ?? "")}
                    aria-describedby={described("columns")}
                    onChange={(e) => onChange({ columns: e.target.value })}
                  />,
                )}
                {field(
                  "sort",
                  t("dashboard.sort"),
                  <Input
                    id={`${id}-sort`}
                    value={String(widget["sort"] ?? "")}
                    aria-describedby={described("sort")}
                    onChange={(e) => onChange({ sort: e.target.value })}
                  />,
                )}
                {field(
                  "size",
                  t("dashboard.size"),
                  <Input
                    id={`${id}-size`}
                    type="number"
                    min={1}
                    max={50}
                    value={num(widget, "size", 10)}
                    onChange={(e) => onChange({ size: Number(e.target.value) })}
                  />,
                )}
              </>
            )}
          </div>
        </>
      )}
      {Object.entries(errors)
        .filter(
          ([f]) =>
            ![
              "title",
              "kind",
              "saved_search",
              "target_type",
              "query",
              "group_by",
              "measure",
              "measure_field",
              "date_field",
              "sidebar_type",
              "columns",
              "sort",
              "size",
              "w",
            ].includes(f),
        )
        .map(([f, m]) => (
          <p key={f} className="text-sm text-destructive">
            {f}: {m}
          </p>
        ))}
      <div>
        <Button variant="outline" size="sm" onClick={onRemove}>
          {t("dashboard.removeWidget")}
        </Button>
      </div>
    </section>
  );
}

// CopyButton copies a dashboard one only reads into one of one's own and opens it (UI14a).
function CopyButton({ id, name }: { id: string; name: string }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [failed, setFailed] = useState(false);
  return (
    <div className="flex flex-col items-end gap-1">
      <Button
        variant="outline"
        className="h-10"
        onClick={() =>
          void api.copyDashboard(id, t("dashboard.copyName", { name })).then(
            (v) => {
              void queryClient.invalidateQueries({ queryKey: ["records", "core.dashboard"] });
              void navigate({ to: "/dashboards/$id", params: { id: String(v.dashboard["id"]) } });
            },
            () => setFailed(true),
          )
        }
      >
        {t("dashboard.copy")}
      </Button>
      {failed && (
        <p role="alert" className="text-sm text-destructive">
          {t("dashboard.copyFailed")}
        </p>
      )}
    </div>
  );
}

// NewDashboardButton creates a dashboard and opens it.
// NewDashboardButton creates a dashboard and opens it; small is the plus at the head of the area
// "Dashboards" in the navigation (UI36).
export function NewDashboardButton({ small = false }: { small?: boolean }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [failed, setFailed] = useState(false);
  return (
    <>
      <button
        type="button"
        aria-label={small ? t("dashboard.new") : undefined}
        title={small ? t("dashboard.new") : undefined}
        className={
          small
            ? "inline-grid size-7 flex-none place-items-center rounded-md text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            : "block w-full rounded-md px-2 py-1 text-left text-sm text-muted-foreground hover:bg-surface-active"
        }
        onClick={() =>
          void api.create("core.dashboard", { name: t("dashboard.newName") }).then(
            (d) => {
              void queryClient.invalidateQueries({ queryKey: ["records", "core.dashboard"] });
              void navigate({ to: "/dashboards/$id", params: { id: String(d["id"]) } });
            },
            () => setFailed(true),
          )
        }
      >
        {small ? <Plus aria-hidden="true" className="size-4" /> : t("dashboard.new")}
      </button>
      {failed && <p className="text-sm text-destructive">{t("dashboard.newFailed")}</p>}
    </>
  );
}
