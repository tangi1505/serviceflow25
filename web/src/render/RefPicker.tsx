// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The picker for references and roles (UI28, Hauptplan E123): a combobox after the WAI-ARIA
// pattern. Types with searchable fields are searched over /api/v1/search (typos too); types
// without (small select types) show their first 100, filtered in the browser. The chosen
// records stand above the input, each with a button to remove it; single holds at most one.
import { useQuery } from "@tanstack/react-query";
import { useEffect, useId, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  typesQuery,
  type ActorRef,
  type TypeDefinition,
} from "@/api/client";
import { Input } from "@/ui/input";

import { RefName, titleOf } from "./RefName";

type Option = ActorRef & { title: string };

// searchable tells the types /api/v1/search can find: those with a searchable field.
export function searchable(t: TypeDefinition | undefined): boolean {
  return t?.fields.some((f) => f.searchable === true) === true;
}

// useDebounced gives the value once it has stood still for ms.
function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setV(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return v;
}

// useOptions finds the records of the types that match the text.
function useOptions(targets: string[], text: string, open: boolean) {
  const types = useQuery(typesQuery);
  const q = useDebounced(text.trim(), 200);
  const defs = targets.map((name) => types.data?.find((t) => qualifiedName(t) === name));
  const searched = targets.filter((_, i) => searchable(defs[i]));
  const listed = targets.filter((_, i) => types.data && !searchable(defs[i]));
  const hits = useQuery({
    queryKey: ["pick", "search", searched, q],
    queryFn: () => api.search(q, searched.join(","), 20),
    enabled: open && q !== "" && searched.length > 0,
    retry: false,
  });
  const lists = useQuery({
    queryKey: ["pick", "list", listed],
    queryFn: async () =>
      (
        await Promise.all(
          listed.map(async (name) => {
            const page = await api.list(name, { limit: 100 });
            const def = types.data?.find((t) => qualifiedName(t) === name);
            return page.items.map((r) => ({
              type: name,
              id: String(r["id"]),
              title: titleOf(def, r),
            }));
          }),
        )
      ).flat(),
    enabled: open && listed.length > 0,
    staleTime: 60_000,
  });
  const needle = q.toLowerCase();
  const options: Option[] = [
    ...(q === "" || !hits.data
      ? []
      : hits.data.map((h) => ({ type: h.type, id: h.id, title: h.title ?? h.id }))),
    ...(lists.data ?? []).filter((o) => needle === "" || o.title.toLowerCase().includes(needle)),
  ];
  return {
    options,
    pending: hits.isFetching || lists.isFetching,
    // A type to search needs some text first.
    needsText: q === "" && searched.length > 0 && listed.length === 0,
    typeLabel: (name: string, lang: string) =>
      labelOf(types.data?.find((t) => qualifiedName(t) === name)?.label, lang, name),
  };
}

export type RefPickerProps = {
  // id is the input's id, for the label of the field.
  id: string;
  targets: string[];
  value: ActorRef[];
  onChange: (value: ActorRef[]) => void;
  single?: boolean;
  required?: boolean;
  invalid?: boolean;
  describedBy?: string;
  disabled?: boolean;
};

export function RefPicker({
  id,
  targets,
  value,
  onChange,
  single = false,
  required,
  invalid,
  describedBy,
  disabled,
}: RefPickerProps) {
  const { t, i18n } = useTranslation();
  const listId = useId();
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const { options, pending, needsText, typeLabel } = useOptions(targets, text, open);
  const chosen = new Set(value.map((v) => `${v.type}:${v.id}`));
  const shown = options.filter((o) => !chosen.has(`${o.type}:${o.id}`));
  const current = Math.min(active, shown.length - 1);
  const optionId = (i: number) => `${listId}-${i}`;

  const pick = (o: Option) => {
    const ref = { type: o.type, id: o.id };
    onChange(single ? [ref] : [...value, ref]);
    setText("");
    setActive(0);
    setOpen(false);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setOpen(true);
        setActive(Math.min(current + 1, shown.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        setActive(Math.max(current - 1, 0));
        break;
      case "Enter":
        if (open && shown[current]) {
          e.preventDefault();
          pick(shown[current]);
        }
        break;
      case "Escape":
        if (open) {
          e.preventDefault();
          setOpen(false);
        }
        break;
    }
  };

  const expanded = open && !disabled;
  return (
    <div className="flex flex-col gap-1">
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1" aria-label={t("picker.chosen")}>
          {value.map((v) => (
            <li
              key={`${v.type}:${v.id}`}
              className="flex items-center gap-1 rounded-md border bg-secondary px-2 py-0.5 text-sm"
            >
              <RefName refTo={v} />
              {!disabled && (
                <button
                  type="button"
                  className="rounded-sm px-1 text-muted-foreground hover:text-foreground"
                  aria-label={t("picker.remove")}
                  onClick={() => onChange(value.filter((x) => x !== v))}
                >
                  ×
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
      {!disabled && (
        <div className="relative">
          <Input
            id={id}
            role="combobox"
            autoComplete="off"
            aria-autocomplete="list"
            aria-expanded={expanded}
            aria-controls={listId}
            aria-activedescendant={expanded && shown[current] ? optionId(current) : undefined}
            aria-required={required && value.length === 0 ? true : undefined}
            aria-invalid={invalid ? true : undefined}
            aria-describedby={describedBy}
            placeholder={t("picker.placeholder")}
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setActive(0);
              setOpen(true);
            }}
            onFocus={() => setOpen(true)}
            onBlur={() => setOpen(false)}
            onKeyDown={onKeyDown}
          />
          <ul
            id={listId}
            role="listbox"
            aria-label={t("picker.options")}
            hidden={!expanded}
            className="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-md border bg-card py-1 text-sm shadow-md"
          >
            {shown.map((o, i) => (
              // The keyboard works in the input (aria-activedescendant), as the combobox
              // pattern wants; the option itself never takes the focus.
              // eslint-disable-next-line jsx-a11y/click-events-have-key-events
              <li
                key={`${o.type}:${o.id}`}
                id={optionId(i)}
                role="option"
                aria-selected={i === current}
                className={
                  i === current
                    ? "cursor-pointer bg-secondary px-3 py-1"
                    : "cursor-pointer px-3 py-1"
                }
                // Keep the focus in the input; the click picks.
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => pick(o)}
              >
                {o.title}
                {targets.length > 1 && (
                  <span className="text-muted-foreground">
                    {" "}
                    · {typeLabel(o.type, i18n.language)}
                  </span>
                )}
              </li>
            ))}
            {shown.length === 0 && (
              <li
                role="option"
                aria-selected={false}
                aria-disabled="true"
                className="px-3 py-1 text-muted-foreground"
              >
                {pending ? t("list.loading") : needsText ? t("picker.type") : t("picker.none")}
              </li>
            )}
          </ul>
        </div>
      )}
    </div>
  );
}
