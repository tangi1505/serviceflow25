// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Messages at the bottom right (UI59, UI-Plan E60): what an action reports as its result. One
// place to call them: toast.success, .info, .warning and .error with a title and a text. Four
// kinds in green, the accent, yellow and red, each with a stripe on the left, a tinted ground, a
// symbol (not only the colour tells the kind, WCAG 1.4.1) and a close button. The newest stands
// at the bottom of the corner, at most three at once. Success and info go after 5 s, held while
// the mouse or the focus is on them; warning and error stay until closed (reading takes time,
// WCAG 2.2.1). A message can carry a button such as "Rückgängig" (E61); then it stays 10 s.
// Screen readers hear success and info as status, warning and error as alert.
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from "lucide-react";
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { cn } from "@/ui/utils";

export type ToastKind = "success" | "info" | "warning" | "error";

export type ToastAction = { label: string; onClick: () => void };

export type Toast = {
  id: number;
  kind: ToastKind;
  title: string;
  text?: ReactNode;
  action?: ToastAction;
};

// How long a message stays on its own; warning and error stay until closed.
export const toastTimes = { plain: 5000, withAction: 10000 };
// At most this many at once; a new one pushes out the oldest.
export const toastLimit = 3;

let next = 1;
let toasts: Toast[] = [];
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) {
    l();
  }
}

function add(kind: ToastKind, title: string, text?: ReactNode, action?: ToastAction) {
  const id = next++;
  toasts = [...toasts, { id, kind, title, text, action }].slice(-toastLimit);
  emit();
  return id;
}

type Options = { action?: ToastAction };

export const toast = {
  success: (title: string, text?: ReactNode, o?: Options) => add("success", title, text, o?.action),
  info: (title: string, text?: ReactNode, o?: Options) => add("info", title, text, o?.action),
  warning: (title: string, text?: ReactNode, o?: Options) => add("warning", title, text, o?.action),
  error: (title: string, text?: ReactNode, o?: Options) => add("error", title, text, o?.action),
  dismiss: (id: number) => {
    toasts = toasts.filter((t) => t.id !== id);
    emit();
  },
  // clear removes all messages (tests, signing out).
  clear: () => {
    toasts = [];
    emit();
  },
};

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

const look: Record<ToastKind, { box: string; icon: string; Symbol: typeof Info }> = {
  success: {
    box: "border-l-success bg-success-background",
    icon: "text-success",
    Symbol: CircleCheck,
  },
  info: { box: "border-l-accent bg-info-background", icon: "text-foreground", Symbol: Info },
  warning: {
    box: "border-l-warning bg-warning-background",
    icon: "text-warning-foreground",
    Symbol: TriangleAlert,
  },
  error: {
    box: "border-l-destructive bg-destructive-background",
    icon: "text-destructive",
    Symbol: CircleAlert,
  },
};

// Toaster shows the messages; it stands once in the app and once in the portal.
export function Toaster() {
  const { t } = useTranslation();
  const list = useSyncExternalStore(subscribe, () => toasts);
  return (
    <section
      aria-label={t("toast.region")}
      className="pointer-events-none fixed right-4 bottom-4 z-[60] flex w-[min(400px,calc(100vw-2rem))] flex-col gap-2"
    >
      {list.map((m) => (
        <Message key={m.id} toast={m} />
      ))}
    </section>
  );
}

function Message({ toast: m }: { toast: Toast }) {
  const { t } = useTranslation();
  const stays = m.kind === "warning" || m.kind === "error";
  const [held, setHeld] = useState(false);
  const left = useRef(m.action ? toastTimes.withAction : toastTimes.plain);
  useEffect(() => {
    if (stays || held) {
      return;
    }
    const started = Date.now();
    const timer = window.setTimeout(() => toast.dismiss(m.id), left.current);
    return () => {
      window.clearTimeout(timer);
      left.current = Math.max(left.current - (Date.now() - started), 1000);
    };
  }, [stays, held, m.id]);
  const { box, icon, Symbol } = look[m.kind];
  return (
    <div
      role={stays ? "alert" : "status"}
      onMouseEnter={() => setHeld(true)}
      onMouseLeave={() => setHeld(false)}
      onFocus={() => setHeld(true)}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
          setHeld(false);
        }
      }}
      className={cn(
        "pointer-events-auto flex items-start gap-2.5 rounded-md border border-l-[5px] py-2.5 pr-2 pl-3 text-sm text-foreground shadow-lg",
        box,
      )}
    >
      <Symbol aria-hidden="true" className={cn("mt-0.5 size-4 flex-none", icon)} />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{m.title}</p>
        {m.text && <div className="mt-0.5 text-foreground-2">{m.text}</div>}
        {m.action && (
          <button
            type="button"
            className="mt-1.5 rounded-sm font-medium text-foreground underline underline-offset-2 hover:no-underline"
            onClick={() => {
              m.action!.onClick();
              toast.dismiss(m.id);
            }}
          >
            {m.action.label}
          </button>
        )}
      </div>
      <button
        type="button"
        aria-label={t("toast.close")}
        title={t("toast.close")}
        onClick={() => toast.dismiss(m.id)}
        className="grid size-6 flex-none place-items-center rounded-sm text-foreground-2 hover:bg-surface-active hover:text-foreground"
      >
        <X aria-hidden="true" className="size-4" />
      </button>
    </div>
  );
}
