// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The command line (UI31, UI-Plan E36): Strg K opens a dialog in the middle of the page. It finds
// records over every type one may read (the full-text search of PL26; the running number with
// PL49), offers commands (a new record of a type, light or dark, the row height) and leads to the
// pages of the navigation. Empty, it lists the records opened last. The keyboard stays in the
// input, as the combobox pattern wants: the arrows choose, Enter runs, Esc closes and gives the
// focus back to where it was.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  meQuery,
  mySettingsQuery,
  packagesQuery,
  qualifiedName,
  savedSearchesQuery,
  typesQuery,
  type SettingsChange,
} from "@/api/client";
import { recentRecords } from "@/app/recent";
import { workplaceFormsQuery } from "@/render/FormPage";
import { useDebounced } from "@/render/RefPicker";
import { colorSchemes } from "@/ui/colorScheme";

type Group = "recent" | "records" | "commands" | "goto";
type Item = { key: string; group: Group; label: string; hint?: string; run: () => void };

const groups: Group[] = ["recent", "records", "commands", "goto"];

// matches tells whether every word of the text is in the label.
function matches(label: string, words: string[]): boolean {
  const l = label.toLowerCase();
  return words.every((w) => l.includes(w));
}

export function CommandPalette({ onClose }: { onClose: () => void }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const id = useId();
  const input = useRef<HTMLInputElement>(null);
  // back is what had the focus before; Esc gives it back.
  const back = useRef(document.activeElement as HTMLElement | null);
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const q = useDebounced(text.trim(), 200);
  const me = useQuery(meQuery);
  const types = useQuery(typesQuery);
  const packages = useQuery({ ...packagesQuery, enabled: me.data !== undefined });
  const saved = useQuery({ ...savedSearchesQuery, enabled: me.data !== undefined });
  const forms = useQuery({ ...workplaceFormsQuery, enabled: me.data !== undefined });
  const hits = useQuery({
    queryKey: ["search", "command", q],
    queryFn: () => api.search(q, "", 8),
    enabled: q.length >= 2,
    retry: false,
  });
  // Light, dark and the row height are kept with the person, as in the settings (UI9); the
  // frame and the lists follow the settings.
  const allowed = useQuery(mySettingsQuery).data?.allowed;
  const settings = useMutation({
    mutationFn: (change: SettingsChange) => api.setMySettings(change),
    onSuccess: (s) => queryClient.setQueryData(mySettingsQuery.queryKey, s),
  });
  useEffect(() => input.current?.focus(), []);

  const lang = i18n.language;
  const typeLabel = (name: string) =>
    labelOf(types.data?.find((x) => qualifiedName(x) === name)?.label, lang, name);
  const open = (typeName: string, recordId: string) => () =>
    void navigate({ to: "/types/$typeName/$id", params: { typeName, id: recordId } });
  const to = (path: "/settings" | "/settings/global" | "/roles" | "/customize" | "/portal") => () =>
    void navigate({ to: path });

  const words = text.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const found: Item[] = [];
  if (words.length === 0 && me.data) {
    for (const r of recentRecords(me.data.id)) {
      found.push({
        key: `recent:${r.type}:${r.id}`,
        group: "recent",
        label: r.title,
        hint: typeLabel(r.type),
        run: open(r.type, r.id),
      });
    }
  }
  if (words.length > 0 && q.length >= 2) {
    for (const h of hits.data ?? []) {
      found.push({
        key: `record:${h.type}:${h.id}`,
        group: "records",
        label: h.title ?? h.id,
        hint: typeLabel(h.type),
        run: open(h.type, h.id),
      });
    }
  }
  const offered: Item[] = [
    // The forms of the workplace by name, such as the phone note (SD9).
    ...(forms.data ?? []).map((f) => ({
      key: `form:${f.id}`,
      group: "commands" as const,
      label: t("command.form", { name: labelOf(f.definition.title, lang, f.name) }),
      run: () => void navigate({ to: "/forms/$formId", params: { formId: f.id } }),
    })),
    ...(types.data ?? []).map((ty) => {
      const name = qualifiedName(ty);
      return {
        key: `new:${name}`,
        group: "commands" as const,
        label: t("command.new", { type: labelOf(ty.label, lang, name) }),
        run: () => void navigate({ to: "/types/$typeName/new", params: { typeName: name } }),
      };
    }),
    // What the organisation lets the person set herself (UI39).
    ...(allowed?.color_scheme === false ? [] : colorSchemes).map((c) => ({
      key: `scheme:${c}`,
      group: "commands" as const,
      label: t("command.colorScheme", { value: t(`colorScheme.${c}`) }),
      run: () => settings.mutate({ color_scheme: c }),
    })),
    ...(allowed?.density === false ? [] : (["compact", "normal"] as const)).map((d) => ({
      key: `density:${d}`,
      group: "commands" as const,
      label: t("command.density", { value: t(`list.densities.${d}`) }),
      run: () => settings.mutate({ density: d }),
    })),
    ...(types.data ?? []).map((ty) => {
      const name = qualifiedName(ty);
      const pkg = ty.package ?? name.split(".")[0] ?? "";
      return {
        key: `type:${name}`,
        group: "goto" as const,
        label: labelOf(ty.label, lang, name),
        // The area is named by its package (UI36, E48).
        hint: labelOf(packages.data?.find((p) => p.name === pkg)?.label, lang, pkg),
        run: () => void navigate({ to: "/types/$typeName", params: { typeName: name } }),
      };
    }),
    ...(saved.data?.items ?? []).map((s) => ({
      key: `saved:${String(s["id"])}`,
      group: "goto" as const,
      label: String(s["name"]),
      hint: t("nav.saved"),
      run: () =>
        void navigate({
          to: "/types/$typeName",
          params: { typeName: String(s["target_type"]) },
          search: { search: String(s["id"]) },
        }),
    })),
    { key: "settings", group: "goto", label: t("settings.title"), run: to("/settings") },
    { key: "global", group: "goto", label: t("settings.global"), run: to("/settings/global") },
    ...(me.data?.manage_rights === true
      ? [{ key: "roles", group: "goto" as const, label: t("nav.roles"), run: to("/roles") }]
      : []),
    ...(me.data?.customize === true
      ? [
          {
            key: "customize",
            group: "goto" as const,
            label: t("nav.customize"),
            run: to("/customize"),
          },
        ]
      : []),
    { key: "portal", group: "goto", label: t("portal.toPortal"), run: to("/portal") },
  ];
  found.push(...offered.filter((o) => matches(o.label, words)));
  // With text, the last option leads to every hit on the search page (UI30).
  if (words.length > 0) {
    found.push({
      key: "search",
      group: "goto",
      label: t("command.searchAll", { q: text.trim() }),
      run: () => void navigate({ to: "/search", search: { q: text.trim() } }),
    });
  }
  const items = groups.flatMap((g) => found.filter((f) => f.group === g));
  const current = items.length === 0 ? -1 : Math.min(active, items.length - 1);
  const optionId = (i: number) => `${id}-option-${i}`;
  useEffect(() => {
    if (current >= 0) {
      document.getElementById(`${id}-option-${current}`)?.scrollIntoView?.({ block: "nearest" });
    }
  }, [id, current]);

  const close = (restore: boolean) => {
    onClose();
    if (restore) {
      back.current?.focus();
    }
  };
  const run = (item: Item) => {
    close(false);
    item.run();
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const n = items.length;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        if (n > 0) {
          setActive((current + 1) % n);
        }
        break;
      case "ArrowUp":
        e.preventDefault();
        if (n > 0) {
          setActive((current - 1 + n) % n);
        }
        break;
      case "Enter":
        e.preventDefault();
        if (current >= 0) {
          run(items[current]!);
        }
        break;
      case "Escape":
        e.preventDefault();
        close(true);
        break;
      case "Tab":
        // The focus stays in the dialog: its only stop is the input.
        e.preventDefault();
        break;
    }
  };

  const listId = `${id}-list`;
  const pending = words.length > 0 && (q !== text.trim() || hits.isFetching);
  return (
    <div className="fixed inset-0 z-50">
      {/* A click beside the dialog closes it; the keyboard has Esc. */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
      <div className="absolute inset-0 bg-foreground/40" onMouseDown={() => close(true)} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        className="relative mx-auto mt-24 flex max-h-[70vh] w-[640px] max-w-[calc(100%-2rem)] flex-col overflow-hidden rounded-lg border bg-card text-foreground shadow-xl"
      >
        <h2 id={`${id}-title`} className="sr-only">
          {t("command.title")}
        </h2>
        <div className="flex h-12 flex-none items-center gap-2 border-b px-3.5">
          <input
            ref={input}
            role="combobox"
            autoComplete="off"
            aria-label={t("command.input")}
            aria-autocomplete="list"
            aria-expanded={items.length > 0}
            aria-controls={listId}
            aria-activedescendant={current >= 0 ? optionId(current) : undefined}
            placeholder={t("command.placeholder")}
            className="h-full min-w-0 flex-1 bg-transparent text-[15px] outline-none"
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setActive(0);
            }}
            onKeyDown={onKeyDown}
          />
          <kbd className="rounded border px-1.5 font-mono text-xs text-foreground-2">Esc</kbd>
        </div>
        <div
          id={listId}
          role="listbox"
          aria-label={t("command.options")}
          className="min-h-0 flex-1 overflow-y-auto py-1"
        >
          {groups.map((g) => {
            const list = items.map((item, i) => ({ item, i })).filter((x) => x.item.group === g);
            if (list.length === 0) {
              return null;
            }
            return (
              <div key={g} role="group" aria-labelledby={`${id}-${g}`}>
                <div
                  id={`${id}-${g}`}
                  role="presentation"
                  className="px-3.5 pb-1 pt-2.5 text-xs font-medium uppercase tracking-wider text-foreground-2"
                >
                  {t(`command.groups.${g}`)}
                </div>
                {list.map(({ item, i }) => (
                  // The option never takes the focus; the click runs it.
                  // eslint-disable-next-line jsx-a11y/click-events-have-key-events
                  <div
                    key={item.key}
                    id={optionId(i)}
                    role="option"
                    tabIndex={-1}
                    aria-selected={i === current}
                    className={
                      i === current
                        ? "flex h-10 cursor-pointer items-center gap-2.5 bg-accent-soft px-3.5 shadow-[inset_2px_0_0_var(--accent)]"
                        : "flex h-10 cursor-pointer items-center gap-2.5 px-3.5"
                    }
                    onMouseDown={(e) => e.preventDefault()}
                    onMouseMove={() => setActive(i)}
                    onClick={() => run(item)}
                  >
                    <span className="min-w-0 flex-1 truncate">{item.label}</span>
                    {item.hint && <span className="text-xs text-foreground-2">{item.hint}</span>}
                  </div>
                ))}
              </div>
            );
          })}
        </div>
        {items.length === 0 && (
          <p className="px-3.5 py-3 text-sm text-foreground-2">
            {pending ? t("list.loading") : t("command.none")}
          </p>
        )}
        <p className="flex flex-none flex-wrap gap-4 border-t bg-surface-subtle px-3.5 py-2 text-xs text-foreground-2">
          <span>{t("command.keys.choose")}</span>
          <span>{t("command.keys.run")}</span>
          <span>{t("command.keys.close")}</span>
        </p>
      </div>
    </div>
  );
}
