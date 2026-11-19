// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Tabs (UI67): a row of tabs over one panel, the WAI-ARIA pattern "tabs" — the arrows, Home and
// End move between the tabs and choose at once; the chosen tab alone is reached with Tab.

import { useId, useRef, type KeyboardEvent, type ReactNode } from "react";

import { cn } from "@/ui/utils";

export function Tabs({
  label,
  tabs,
  current,
  onChange,
  children,
}: {
  label: string;
  tabs: { key: string; label: ReactNode }[];
  current: string;
  onChange: (key: string) => void;
  children: ReactNode;
}) {
  const base = useId();
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const at = Math.max(
    0,
    tabs.findIndex((t) => t.key === current),
  );
  const go = (i: number) => {
    const n = (i + tabs.length) % tabs.length;
    onChange(tabs[n]!.key);
    refs.current[n]?.focus();
  };
  const onKeyDown = (e: KeyboardEvent) => {
    const moves: Record<string, number> = {
      ArrowRight: at + 1,
      ArrowLeft: at - 1,
      Home: 0,
      End: tabs.length - 1,
    };
    if (e.key in moves) {
      e.preventDefault();
      go(moves[e.key]!);
    }
  };
  return (
    <div className="flex flex-col gap-4">
      <div role="tablist" aria-label={label} className="flex flex-wrap gap-1 border-b">
        {tabs.map((tab, i) => (
          <button
            key={tab.key}
            ref={(el) => {
              refs.current[i] = el;
            }}
            type="button"
            role="tab"
            id={`${base}-${tab.key}`}
            aria-selected={i === at}
            aria-controls={`${base}-panel`}
            tabIndex={i === at ? 0 : -1}
            onClick={() => onChange(tab.key)}
            onKeyDown={onKeyDown}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm font-medium",
              i === at
                ? "border-foreground text-foreground"
                : "border-transparent text-foreground-2",
            )}
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`${base}-panel`} aria-labelledby={`${base}-${tabs[at]!.key}`}>
        {children}
      </div>
    </div>
  );
}
