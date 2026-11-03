// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The deadline of a record in its list (UI34, UI-Plan E37): the next deadline of its running
// SLA clocks with the mark of the design kit (UI29), red with text when breached, yellow from
// three quarters of the time on, grey before; a record whose clocks only wait says so. The
// server keeps the deadline and the three-quarter point at the record; the bar estimates the
// share from the two, exact for clocks round the clock.
import { useTranslation } from "react-i18next";

import type { RecordData } from "@/api/client";
import { Deadline, type DeadlineState } from "@/ui/marks";

import { formatMoment } from "./fields";
import { duration } from "./Sidebar";

const minute = 60_000;

export type DeadlineView = { state: DeadlineState; share: number; left: number; due?: string };

// deadlineOf reads the deadline of a row at a moment; null without one.
export function deadlineOf(row: RecordData, now: number): DeadlineView | null {
  const due = typeof row["sla_due_at"] === "string" ? row["sla_due_at"] : undefined;
  if (!due) {
    return row["sla_paused"] === true ? { state: "paused", share: 0, left: 0 } : null;
  }
  const end = Date.parse(due);
  const warn = typeof row["sla_warn_at"] === "string" ? Date.parse(row["sla_warn_at"]) : Number.NaN;
  const left = (end - now) / minute;
  // A quarter of the time lies between the three-quarter point and the deadline.
  const quarter = (end - warn) / minute;
  const share = quarter > 0 ? 1 - left / (4 * quarter) : left > 0 ? 0 : 1;
  const state: DeadlineState = left <= 0 ? "breached" : now >= warn ? "close" : "running";
  return { state, share, left, due };
}

export function DeadlineCell({ row }: { row: RecordData }) {
  const { t, i18n } = useTranslation();
  const d = deadlineOf(row, Date.now());
  if (!d) {
    return <span className="text-foreground-2">–</span>;
  }
  let text: string;
  if (d.state === "paused") {
    text = t("sidebar.light.paused");
  } else if (d.state === "breached") {
    text = t("sidebar.overdue", { time: duration(d.left) });
  } else if (d.left < 24 * 60) {
    text = t("sidebar.remaining", { time: duration(d.left) });
  } else {
    text = t("sidebar.due", { at: formatMoment(d.due, i18n.language) });
  }
  return <Deadline text={text} share={d.share} state={d.state} />;
}
