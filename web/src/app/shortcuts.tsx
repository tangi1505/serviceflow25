// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Keyboard shortcuts of the frame (UI52, UI-Plan E55): one place where pages announce theirs,
// the dialog "Tastenkürzel" that shows them, and the single keys: "?" shows the dialog, "/"
// jumps into the filter row, a page adds its own such as "N" for new. Single keys do not count
// while the focus is in a field or the text editor, and a person can turn them off in Meine
// Einstellungen › Darstellung (WCAG 2.1.4: a voice input must not set them off). Keys with
// Ctrl (Strg K, Strg B, Strg S) are not single keys and always work; Esc closes what is open.
import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import { mySettingsQuery } from "@/api/client";

// Shortcut is one entry of the dialog; one with key and run also works: key is one character
// for a single key. keys is what the dialog shows, such as ["Umschalt", "Enter"].
export type Shortcut = { keys: string[]; label: string; key?: string; run?: () => void };

type Registry = {
  add: (list: Shortcut[]) => () => void;
  openHelp: () => void;
};

const Ctx = createContext<Registry>({ add: () => () => undefined, openHelp: () => undefined });

// isTyping says whether a key goes into a field: an input, a text area, a choice or the text
// editor. Single keys do not count there.
export function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  if (target.isContentEditable || target.closest('[contenteditable="true"]')) {
    return true;
  }
  const tag = target.tagName;
  if (tag === "TEXTAREA" || tag === "SELECT") {
    return true;
  }
  if (tag === "INPUT") {
    const type = (target as HTMLInputElement).type;
    return !["checkbox", "radio", "button", "submit", "reset"].includes(type);
  }
  return false;
}

// inEditor says whether the key goes into the text editor, where Strg B means bold.
export function inEditor(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable || target.closest('[contenteditable="true"], .ProseMirror') !== null)
  );
}

export function ShortcutsProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const settings = useQuery(mySettingsQuery);
  const singleOn = settings.data?.shortcuts?.value !== "off";
  const [pages, setPages] = useState<Shortcut[][]>([]);
  const [help, setHelp] = useState(false);
  const back = useRef<HTMLElement | null>(null);
  const add = useCallback((list: Shortcut[]) => {
    setPages((p) => [...p, list]);
    return () => setPages((p) => p.filter((x) => x !== list));
  }, []);
  const openHelp = useCallback(() => {
    back.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setHelp(true);
  }, []);
  const closeHelp = useCallback(() => {
    setHelp(false);
    // The focus goes back to where it was (UI52).
    requestAnimationFrame(() => back.current?.focus());
  }, []);
  const own = pages.flat();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey || !singleOn || help) {
        return;
      }
      if (isTyping(e.target)) {
        return;
      }
      if (e.key === "?") {
        e.preventDefault();
        openHelp();
        return;
      }
      if (e.key === "/") {
        const row = document.querySelector<HTMLInputElement>("[data-filter-input]");
        if (row) {
          e.preventDefault();
          row.focus();
        }
        return;
      }
      const hit = own.find((s) => s.key && s.run && s.key.toLowerCase() === e.key.toLowerCase());
      if (hit && !e.shiftKey) {
        e.preventDefault();
        hit.run!();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [own, singleOn, help, openHelp]);

  const value = useMemo(() => ({ add, openHelp }), [add, openHelp]);
  const always: Shortcut[] = [
    { keys: [t("keys.ctrl"), "K"], label: t("keys.command") },
    { keys: [t("keys.ctrl"), "B"], label: t("keys.nav") },
    { keys: ["Esc"], label: t("keys.esc") },
  ];
  const single: Shortcut[] = [
    { keys: ["?"], label: t("keys.help") },
    { keys: ["/"], label: t("keys.filter") },
  ];
  return (
    <Ctx.Provider value={value}>
      {children}
      {help && (
        <HelpDialog
          onClose={closeHelp}
          groups={[
            { title: t("keys.always"), list: always },
            { title: t("keys.single"), list: single, off: !singleOn },
            ...(own.length > 0 ? [{ title: t("keys.page"), list: own }] : []),
          ]}
        />
      )}
    </Ctx.Provider>
  );
}

// usePageShortcuts announces the shortcuts of a page while it is shown. The list is read on
// every render; the dialog shows what it holds now.
export function usePageShortcuts(list: Shortcut[]) {
  const { add } = useContext(Ctx);
  const key = list.map((s) => `${s.keys.join("+")}:${s.label}`).join("|");
  const latest = useRef(list);
  useEffect(() => {
    latest.current = list;
  });
  useEffect(() => {
    const wrapped = latest.current.map((s) => ({
      ...s,
      run: s.run && (() => latest.current.find((x) => x.label === s.label)?.run?.()),
    }));
    return add(wrapped);
    // The list changes its functions on every render; it counts as changed by its keys.
  }, [add, key]);
}

// useShortcutsHelp opens the dialog, for the button in the header.
export function useShortcutsHelp() {
  return useContext(Ctx).openHelp;
}

function HelpDialog({
  onClose,
  groups,
}: {
  onClose: () => void;
  groups: { title: string; list: Shortcut[]; off?: boolean }[];
}) {
  const { t } = useTranslation();
  const id = useId();
  const closer = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    closer.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div className="fixed inset-0 z-50">
      {/* A click beside the dialog closes it; the keyboard has Esc. */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
      <div className="absolute inset-0 bg-foreground/40" onMouseDown={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        className="relative mx-auto mt-24 flex max-h-[75vh] w-[560px] max-w-[calc(100%-2rem)] flex-col overflow-hidden rounded-lg border bg-card text-foreground shadow-xl"
      >
        <div className="flex h-12 flex-none items-center justify-between border-b px-4">
          <h2 id={`${id}-title`} className="text-[15px] font-semibold">
            {t("keys.title")}
          </h2>
          <button
            ref={closer}
            type="button"
            aria-label={t("keys.close")}
            onClick={onClose}
            className="inline-grid size-8 place-items-center rounded-md text-foreground-2 hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <X aria-hidden="true" className="size-4" />
          </button>
        </div>
        <div className="flex min-h-0 flex-col gap-4 overflow-y-auto p-4">
          {groups.map((g) => (
            <section key={g.title} aria-labelledby={`${id}-${g.title}`}>
              <h3
                id={`${id}-${g.title}`}
                className="mb-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase"
              >
                {g.title}
              </h3>
              {g.off && <p className="mb-1.5 text-sm text-foreground-2">{t("keys.off")}</p>}
              <dl className="flex flex-col gap-1">
                {g.list.map((s) => (
                  <div key={`${s.keys.join("+")}-${s.label}`} className="flex items-center gap-3">
                    <dt className="flex w-36 flex-none gap-1">
                      {s.keys.map((k) => (
                        <kbd
                          key={k}
                          className="rounded border border-b-2 border-input bg-card px-1.5 font-mono text-xs text-foreground-2"
                        >
                          {k}
                        </kbd>
                      ))}
                    </dt>
                    <dd className="text-sm">{s.label}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </div>
    </div>
  );
}
