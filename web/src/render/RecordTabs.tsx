// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The tabs under the form of a record page (UI-Plan E38, from SD4): relations first, the
// approvals of a type with the trait approvals (WF6), then one
// tab for each type suggested here (SD18, "Wissen"), the versions of a type with history
// (SD16b), the timeline last; files will come between. Tabs after the WAI-ARIA pattern: arrow keys, Home
// and End move between them, only the chosen one is in the tab order.
import { useQuery } from "@tanstack/react-query";
import { useId, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  labelOf,
  qualifiedName,
  typesQuery,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";

import { Approvals } from "./Approvals";
import { Relations } from "./Relations";
import { Substitutes } from "./Substitutes";
import { SuggestionTab, suggesters } from "./Suggestions";
import { Timeline } from "./Timeline";
import { Versions } from "./Versions";

type Tab = { key: string; label: string };

export function RecordTabs({
  type,
  id,
  timeline,
  version,
  record,
  onApply,
  show,
}: {
  type: TypeDefinition;
  id: string;
  timeline: boolean;
  // version is the record's current version: restoring sends it as If-Match (SD16b).
  version: number;
  // record and onApply serve the tabs of suggestions (SD18): their search starts from the
  // record's fields, using a hit puts values into the form.
  record: RecordData;
  onApply: (values: Record<string, unknown>) => void;
  // show opens a tab from outside, each time its n changes (SD17: the change that waits).
  show?: { n: number; key: string };
}) {
  const { t, i18n } = useTranslation();
  const base = useId();
  const types = useQuery(typesQuery);
  const suggested = suggesters(types.data ?? [], type);
  const withEntries = type.traits?.includes("timeline") === true;
  const tabs: Tab[] = [
    { key: "relations", label: t("relations.title") },
    ...(type.traits?.includes("approvals")
      ? [{ key: "approvals", label: t("approvals.title") }]
      : []),
    // The substitutes of a person at approvals, for the administration (WF6).
    ...(qualifiedName(type) === "core.person"
      ? [{ key: "substitutes", label: t("substitutes.tab") }]
      : []),
    ...suggested.map((s) => ({
      key: `suggest:${qualifiedName(s)}`,
      label: labelOf(s.suggest?.label, i18n.language, qualifiedName(s)),
    })),
    ...(type.traits?.includes("history") ? [{ key: "versions", label: t("versions.title") }] : []),
    ...(timeline
      ? [{ key: "timeline", label: withEntries ? t("timeline.title") : t("timeline.history") }]
      : []),
  ];
  const [current, setCurrent] = useState(tabs[0]!.key);
  const [shown, setShown] = useState(0);
  if (show && show.n !== shown) {
    setShown(show.n);
    setCurrent(show.key);
  }
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  const at = Math.max(
    0,
    tabs.findIndex((tab) => tab.key === current),
  );
  const go = (i: number) => {
    const next = (i + tabs.length) % tabs.length;
    setCurrent(tabs[next]!.key);
    refs.current[next]?.focus();
  };
  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
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
    <div className="mt-8 flex flex-col gap-4">
      <div
        role="tablist"
        aria-label={t("relations.tabs")}
        className="flex flex-wrap gap-1 border-b"
      >
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
            aria-controls={`${base}-${tab.key}-panel`}
            tabIndex={i === at ? 0 : -1}
            onClick={() => setCurrent(tab.key)}
            onKeyDown={onKeyDown}
            className={
              "-mb-px border-b-2 px-3 py-2 text-sm font-medium " +
              (i === at
                ? "border-foreground text-foreground"
                : "border-transparent text-foreground-2")
            }
          >
            {tab.label}
          </button>
        ))}
      </div>
      <div
        role="tabpanel"
        id={`${base}-${tabs[at]!.key}-panel`}
        aria-labelledby={`${base}-${tabs[at]!.key}`}
      >
        {tabs[at]!.key === "relations" ? (
          <Relations type={type} id={id} />
        ) : tabs[at]!.key === "approvals" ? (
          <Approvals type={type} id={id} record={record} />
        ) : tabs[at]!.key === "substitutes" ? (
          <Substitutes personId={id} />
        ) : tabs[at]!.key.startsWith("suggest:") ? (
          <SuggestionTab
            type={type}
            record={record}
            from={suggested.find((s) => `suggest:${qualifiedName(s)}` === tabs[at]!.key)!}
            onApply={onApply}
          />
        ) : tabs[at]!.key === "versions" ? (
          <Versions key={version} type={type} id={id} version={version} />
        ) : (
          <Timeline type={type} id={id} inTab />
        )}
      </div>
    </div>
  );
}
