// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The one question before something that cannot be undone (UI63, UI64, UI-Plan E61): what goes,
// by name, and that it cannot be undone; a red button to go on, "Abbrechen" and Esc to stop.
// confirm() asks and resolves to the answer; <Confirmer/> at the root of the app shows it. The
// focus starts on "Abbrechen", stays inside the dialog and goes back where it was.

import { useEffect, useId, useRef, useSyncExternalStore, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "./button";

export type Question = {
  // title says what happens, with the name of what goes ("Rolle „Leitstelle“ löschen?").
  title: string;
  // text says more; by default that it cannot be undone.
  text?: ReactNode;
  // action is the label of the red button; by default "Löschen".
  action?: string;
};

type Open = Question & { answer: (yes: boolean) => void };

let current: Open | null = null;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

// confirm asks the question; a question still open is answered "no" first.
export function confirm(q: Question): Promise<boolean> {
  current?.answer(false);
  return new Promise((resolve) => {
    const open: Open = {
      ...q,
      answer: (yes) => {
        if (current === open) {
          current = null;
          emit();
        }
        resolve(yes);
      },
    };
    current = open;
    emit();
  });
}

// clearConfirm answers an open question "no" (tests between cases).
export function clearConfirm() {
  current?.answer(false);
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function Confirmer() {
  const q = useSyncExternalStore(
    subscribe,
    () => current,
    () => null,
  );
  return q ? <Dialog key={q.title} q={q} /> : null;
}

function Dialog({ q }: { q: Open }) {
  const { t } = useTranslation();
  const id = useId();
  const box = useRef<HTMLDivElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    cancel.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        q.answer(false);
      }
      // Tab stays among the controls of the dialog.
      if (e.key === "Tab" && box.current) {
        const all = Array.from(box.current.querySelectorAll<HTMLElement>("button"));
        const first = all[0];
        const last = all[all.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last?.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first?.focus();
        }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      before?.focus();
    };
  }, [q]);
  return (
    <div className="fixed inset-0 z-[60]">
      {/* A click beside the dialog stops, like Abbrechen; the keyboard has Esc. */}
      {/* eslint-disable-next-line jsx-a11y/no-static-element-interactions */}
      <div className="absolute inset-0 bg-foreground/40" onMouseDown={() => q.answer(false)} />
      <div
        ref={box}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={`${id}-title`}
        aria-describedby={`${id}-text`}
        className="relative mx-auto mt-32 flex w-[440px] max-w-[calc(100%-2rem)] flex-col gap-3 rounded-lg border bg-card p-5 text-foreground shadow-xl"
      >
        <h2 id={`${id}-title`} className="text-[15px] font-semibold">
          {q.title}
        </h2>
        <div id={`${id}-text`} className="text-sm text-foreground-2">
          {q.text ?? t("confirm.final")}
        </div>
        <div className="mt-2 flex justify-end gap-2">
          <Button ref={cancel} variant="outline" onClick={() => q.answer(false)}>
            {t("confirm.cancel")}
          </Button>
          <Button variant="destructive" onClick={() => q.answer(true)}>
            {q.action ?? t("confirm.delete")}
          </Button>
        </div>
      </div>
    </div>
  );
}
