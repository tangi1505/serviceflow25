// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Chart draws a pie, bars or a line with ECharts (UI13, Hauptplan E41), SVG only and with the
// colours of the theme. The chart is a picture for the eye; beside it a legend of buttons (pie,
// bars) or a hidden table (line) carries the same numbers for the keyboard and screen readers,
// and a point opens the list behind it.
import { BarChart, LineChart, PieChart } from "echarts/charts";
import { GridComponent, TooltipComponent } from "echarts/components";
import * as echarts from "echarts/core";
import { SVGRenderer } from "echarts/renderers";
import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";

echarts.use([PieChart, BarChart, LineChart, GridComponent, TooltipComponent, SVGRenderer]);

export type ChartPoint = { label: string; value: number };

// palette are the colours of the series, the accent first; they differ in lightness too.
function palette(): string[] {
  const css = getComputedStyle(document.documentElement);
  const accent = css.getPropertyValue("--accent").trim() || "#0b6e80";
  return [accent, "#4a4f57", "#c77a00", "#8a5200", "#2e7d4f", "#9aa0a8", "#15171a"];
}

function color(name: string, fallback: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback;
}

export function Chart({
  kind,
  points,
  title,
  format,
  onPick,
}: {
  kind: "pie" | "bar" | "line";
  points: ChartPoint[];
  title: string;
  format: (n: number) => string;
  onPick?: (index: number) => void;
}) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const pick = useRef(onPick);
  pick.current = onPick;
  const key = JSON.stringify(points);
  useEffect(() => {
    const el = ref.current;
    if (!el) {
      return;
    }
    // A page without layout (tests) cannot draw: the legend carries the numbers then.
    if (el.clientWidth === 0 && el.clientHeight === 0 && typeof ResizeObserver === "undefined") {
      return;
    }
    let chart: echarts.ECharts;
    try {
      chart = echarts.init(el, undefined, { renderer: "svg" });
    } catch {
      return;
    }
    const colors = palette();
    const fg = color("--foreground-2", "#4a4f57");
    const line = color("--border", "#e4e4df");
    const labels = points.map((p) => p.label);
    const values = points.map((p) => p.value);
    const axis = {
      axisLabel: { color: fg, fontSize: 11 },
      axisLine: { lineStyle: { color: line } },
      splitLine: { lineStyle: { color: line } },
    };
    chart.setOption(
      kind === "pie"
        ? {
            color: colors,
            tooltip: { trigger: "item", valueFormatter: (v: unknown) => format(Number(v)) },
            series: [
              {
                type: "pie",
                radius: ["45%", "80%"],
                label: { show: false },
                data: points.map((p) => ({ name: p.label, value: p.value })),
              },
            ],
          }
        : {
            color: colors,
            tooltip: { trigger: "axis", valueFormatter: (v: unknown) => format(Number(v)) },
            grid: { left: 8, right: 8, top: 8, bottom: 4, containLabel: true },
            xAxis: { type: "category", data: labels, ...axis },
            yAxis: { type: "value", minInterval: 1, ...axis },
            series: [
              kind === "bar"
                ? { type: "bar", data: values, barMaxWidth: 32 }
                : { type: "line", data: values, smooth: false, showSymbol: values.length <= 31 },
            ],
          },
    );
    chart.on("click", (p) => pick.current?.(p.dataIndex));
    const resize =
      typeof ResizeObserver === "undefined" ? undefined : new ResizeObserver(() => chart.resize());
    resize?.observe(el);
    return () => {
      resize?.disconnect();
      chart.dispose();
    };
    // key stands for the points; kind and format are fixed for a widget.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [kind, key]);

  const total = points.reduce((a, p) => a + p.value, 0);
  const summary = t("dashboard.chartSummary", {
    title,
    points: points.map((p) => `${p.label} ${format(p.value)}`).join(", "),
  });
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2 sm:flex-row sm:items-center">
      <div
        ref={ref}
        role="img"
        aria-label={summary}
        className="h-40 min-h-40 w-full min-w-0 flex-1"
      />
      {kind === "line" ? (
        <table className="sr-only">
          <caption>{title}</caption>
          <tbody>
            {points.map((p, i) => (
              <tr key={i}>
                <th scope="row">{p.label}</th>
                <td>{format(p.value)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <ul
          className="flex shrink-0 flex-col gap-1 text-sm"
          aria-label={t("dashboard.legend", { title })}
        >
          {points.map((p, i) => (
            <li key={i}>
              <button
                type="button"
                className="flex items-center gap-2 rounded px-1 text-left hover:bg-surface-active disabled:cursor-default"
                disabled={!onPick}
                onClick={() => onPick?.(i)}
              >
                <span
                  aria-hidden="true"
                  className="inline-block h-2.5 w-2.5 rounded-sm"
                  style={{ background: palette()[kind === "pie" ? i % 7 : 0] }}
                />
                <span>
                  {p.label} · {format(p.value)}
                  {kind === "pie" && total > 0 && (
                    <span className="text-muted-foreground">
                      {" "}
                      ({Math.round((p.value / total) * 100)} %)
                    </span>
                  )}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
