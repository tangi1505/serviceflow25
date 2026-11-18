// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A menu or a small window under a button (UI35): it opens with the button, moves the focus to
// its first item, closes with Esc (the focus goes back to the button), a click outside or the
// button again. Arrow keys move between the items of a menu (role menu).
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";

const items = '[role="menuitem"],[role="menuitemradio"],[role="menuitemcheckbox"]';
const focusable = `${items},button:not([disabled]),a[href]`;

export function usePopover() {
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const close = useCallback((refocus = true) => {
    setOpen(false);
    if (refocus) {
      trigger.current?.focus();
    }
  }, []);
  useEffect(() => {
    if (!open) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
      }
    };
    const onDown = (e: MouseEvent) => {
      const target = e.target as Node;
      if (!panel.current?.contains(target) && !trigger.current?.contains(target)) {
        close(false);
      }
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onDown);
    const p = panel.current;
    (
      p?.querySelector<HTMLElement>(`${items}[aria-checked="true"]`) ??
      p?.querySelector<HTMLElement>(focusable)
    )?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onDown);
    };
  }, [open, close]);
  return { open, toggle: () => setOpen((o) => !o), close, trigger, panel };
}

// menuKeys moves the focus between the items of a menu with the arrow keys, Home and End.
export function menuKeys(e: ReactKeyboardEvent<HTMLElement>) {
  const list = [...e.currentTarget.querySelectorAll<HTMLElement>(items)];
  const at = list.indexOf(document.activeElement as HTMLElement);
  let next = -1;
  switch (e.key) {
    case "ArrowDown":
      next = (at + 1) % list.length;
      break;
    case "ArrowUp":
      next = (at - 1 + list.length) % list.length;
      break;
    case "Home":
      next = 0;
      break;
    case "End":
      next = list.length - 1;
      break;
    default:
      return;
  }
  e.preventDefault();
  list[next]?.focus();
}
