// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The marks of the design kit "Klartext" (UI29, UI plan §2.1, Bausteine): the state as a round
// pill with a dot, the priority as a mark without traffic light, and a deadline as the time
// left in mono over a thin bar. Colour never carries the meaning alone: the text says it.
import { cn } from "@/ui/utils";

export type StateCategory = "open" | "active" | "waiting" | "done";

const dot: Record<StateCategory, string> = {
  // New is an empty ring; in progress the accent; waiting yellow; solved green.
  open: "border-[1.5px] border-muted-foreground bg-transparent",
  active: "bg-accent",
  waiting: "bg-warning",
  done: "bg-success",
};

export function StatePill({
  label,
  category,
  className,
}: {
  label: string;
  category: StateCategory;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border bg-card py-0.5 pr-2.5 pl-2 text-[13px] leading-5 whitespace-nowrap",
        className,
      )}
    >
      <span aria-hidden="true" className={cn("inline-block h-2 w-2 rounded-full", dot[category])} />
      {label}
    </span>
  );
}

const priority: Record<number, string> = {
  1: "bg-primary text-primary-foreground border-primary",
  2: "border-foreground text-foreground",
  3: "border-input text-foreground-2",
};

// PriorityMark shows P1 … Pn: filled for 1, a strong border for 2, quieter below (§2.1).
export function PriorityMark({ value, label }: { value: number; label?: string }) {
  return (
    <span
      title={label}
      className={cn(
        "inline-flex h-5 min-w-[26px] items-center justify-center rounded-[4px] border px-1 font-mono text-[11.5px] font-medium",
        priority[value] ?? "border-border text-muted-foreground",
      )}
    >
      P{value}
    </span>
  );
}

export type DeadlineState = "running" | "close" | "breached" | "paused" | "done";

const bar: Record<DeadlineState, string> = {
  running: "bg-clock-running",
  close: "bg-warning",
  breached: "bg-destructive",
  paused: "bg-clock-paused",
  done: "bg-success",
};

const text: Record<DeadlineState, string> = {
  running: "text-foreground",
  close: "text-warning-foreground",
  breached: "text-destructive",
  paused: "text-muted-foreground",
  done: "text-foreground-2",
};

// Deadline is the time left over a 4 px bar on its track; share is the part of the time used,
// 0 to 1. The bar only adds to the text (it does not reach 3:1, §2.1).
export function Deadline({
  text: label,
  share,
  state,
}: {
  text: string;
  share: number;
  state: DeadlineState;
}) {
  const width = `${Math.round(Math.min(1, Math.max(0, share)) * 100)}%`;
  return (
    <span className="inline-flex min-w-24 flex-col gap-1">
      <span className={cn("font-mono text-[12.5px] font-medium", text[state])}>{label}</span>
      <span aria-hidden="true" className="block h-1 w-full rounded-full bg-clock-track">
        <span className={cn("block h-1 rounded-full", bar[state])} style={{ width }} />
      </span>
    </span>
  );
}
