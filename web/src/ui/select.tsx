// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The one select field (UI57, UI-Plan E59): a button in the look of "Ansicht" with an arrow, the
// list opens like a menu. The native <select> is gone because the browser draws its open list
// itself, which follows neither the theme nor the mode. Two heights from the tokens: 32 px in a
// page header, a bar or a table head, 36 px in a form.
//
// Keyboard after the WAI-ARIA pattern "select-only combobox": the focus stays on the button,
// which names the active option (aria-activedescendant). Closed: Down, Up, Enter and Space open
// it, Home and End open it on the first or last option, a letter opens it on the next option
// starting with it. Open: the arrows, Home, End and Page keys move, letters jump, Enter, Space
// and Tab take the active option, Esc closes without a change. From ten options on, the list has
// a search field on top that takes the focus; the arrows and Enter work in it as well.
import { Check, ChevronDown } from "lucide-react";
import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";

import { useAnchored } from "@/ui/anchor";
import { cn } from "@/ui/utils";

export type SelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

export type ControlSize = "bar" | "form";

// From this many options on the list has a search field (UI57).
export const searchFrom = 10;

// controlHeight is the height of a control from the tokens: 32 or 36 px (tokens.css).
export const controlHeight: Record<ControlSize, string> = {
  bar: "h-[var(--control-bar)]",
  form: "h-[var(--control-form)]",
};

// selectButton is the look of the closed field: the look of the buttons of the bar ("Ansicht").
export function selectButton(size: ControlSize, className?: string) {
  return cn(
    "inline-flex items-center justify-between gap-1.5 rounded-md border border-input bg-card text-left text-foreground hover:bg-surface-active disabled:pointer-events-none disabled:opacity-60 aria-invalid:border-[1.5px] aria-invalid:border-destructive",
    controlHeight[size],
    size === "bar" ? "px-2.5 text-[13px] font-medium" : "w-full px-2.5 text-sm",
    className,
  );
}

export function Select({
  value,
  onChange,
  options,
  id,
  label,
  labelledBy,
  describedBy,
  placeholder,
  size = "form",
  disabled = false,
  invalid = false,
  required = false,
  icon,
  className,
  listWidth,
}: {
  value: string;
  onChange: (value: string) => void;
  options: SelectOption[];
  // id puts the field under a <label htmlFor>.
  id?: string;
  // label names the field when no visible label points to it.
  label?: string;
  labelledBy?: string;
  describedBy?: string;
  // placeholder shows while no option has the value.
  placeholder?: string;
  size?: ControlSize;
  disabled?: boolean;
  invalid?: boolean;
  required?: boolean;
  // icon stands before the text, as in the buttons of the bar.
  icon?: ReactNode;
  className?: string;
  // listWidth is the width of the open list when it should be wider than the field.
  listWidth?: string;
}) {
  const { t } = useTranslation();
  const own = useId();
  const listId = `${own}-list`;
  const button = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  // The list stands in fixed position under the field: in the head of a table it is not cut off.
  const place = useAnchored(button, open);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(-1);
  const typed = useRef({ text: "", at: 0 });

  const searchable = options.length >= searchFrom;
  const shown = useMemo(() => {
    const q = query.trim().toLocaleLowerCase();
    return q === "" ? options : options.filter((o) => o.label.toLocaleLowerCase().includes(q));
  }, [options, query]);
  const chosen = options.find((o) => o.value === value);
  const optionId = (i: number) => `${own}-o${i}`;

  const enabled = (i: number) => i >= 0 && i < shown.length && !shown[i]!.disabled;
  const step = (from: number, by: number) => {
    let i = from;
    for (let n = 0; n < shown.length; n++) {
      i += by;
      if (i < 0 || i >= shown.length) {
        return from;
      }
      if (enabled(i)) {
        return i;
      }
    }
    return from;
  };
  const first = () => step(-1, 1);
  const last = () => step(shown.length, -1);

  const show = (at?: number) => {
    setQuery("");
    const current = options.findIndex((o) => o.value === value);
    setActive(at ?? (current >= 0 ? current : options.findIndex((o) => !o.disabled)));
    setOpen(true);
  };
  const close = (refocus = true) => {
    setOpen(false);
    setQuery("");
    if (refocus) {
      button.current?.focus();
    }
  };
  const take = (i: number) => {
    const o = shown[i];
    if (o && !o.disabled) {
      if (o.value !== value) {
        onChange(o.value);
      }
    }
    close();
  };

  // A letter jumps to the next option starting with what was typed in the last half second.
  const jump = (key: string, from: number) => {
    const now = Date.now();
    const t0 = typed.current;
    const text = now - t0.at < 500 ? t0.text + key : key;
    typed.current = { text, at: now };
    const needle = text.toLocaleLowerCase();
    const repeated = needle.length > 1 && [...needle].every((c) => c === needle[0]);
    const look = repeated ? needle[0]! : needle;
    const start = repeated || needle.length === 1 ? from + 1 : Math.max(from, 0);
    for (let n = 0; n < options.length; n++) {
      const i = (start + n) % options.length;
      const o = options[i]!;
      if (!o.disabled && o.label.toLocaleLowerCase().startsWith(look)) {
        return i;
      }
    }
    return from;
  };

  useEffect(() => {
    if (!open) {
      return;
    }
    if (searchable) {
      search.current?.focus();
    }
    const onDown = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) {
        close(false);
      }
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open, searchable]);

  useEffect(() => {
    if (open && active >= 0) {
      document.getElementById(optionId(active))?.scrollIntoView?.({ block: "nearest" });
    }
    // optionId depends only on the id of the field.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, active]);

  // moves handles the keys that move in an open list; true when it handled the key.
  const moves = (e: ReactKeyboardEvent, inSearch: boolean) => {
    switch (e.key) {
      case "ArrowDown":
        setActive((a) => (a < 0 ? first() : step(a, 1)));
        break;
      case "ArrowUp":
        if (e.altKey) {
          take(active);
          break;
        }
        setActive((a) => (a < 0 ? last() : step(a, -1)));
        break;
      case "PageDown":
        setActive((a) => Math.min(step(Math.max(a, 0) + 9, 1), last()));
        break;
      case "PageUp":
        setActive((a) => Math.max(step(Math.max(a, 0) - 9, -1), first()));
        break;
      case "Home":
        if (inSearch) {
          return false;
        }
        setActive(first());
        break;
      case "End":
        if (inSearch) {
          return false;
        }
        setActive(last());
        break;
      case "Enter":
        take(active);
        break;
      case "Escape":
        close();
        break;
      default:
        return false;
    }
    e.preventDefault();
    e.stopPropagation();
    return true;
  };

  const onButtonKey = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (disabled) {
      return;
    }
    const letter = e.key.length === 1 && e.key !== " " && !e.ctrlKey && !e.metaKey && !e.altKey;
    if (!open) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
        e.preventDefault();
        show();
      } else if (e.key === "Home" || e.key === "End") {
        e.preventDefault();
        const usable = options.flatMap((o, i) => (o.disabled ? [] : [i]));
        show(e.key === "Home" ? usable[0] : usable[usable.length - 1]);
      } else if (letter) {
        e.preventDefault();
        const current = options.findIndex((o) => o.value === value);
        show(jump(e.key, current));
      }
      return;
    }
    if (e.key === " " && typed.current.text === "") {
      e.preventDefault();
      take(active);
      return;
    }
    if (e.key === "Tab") {
      take(active);
      return;
    }
    if (moves(e, false)) {
      return;
    }
    if (letter || e.key === " ") {
      e.preventDefault();
      setActive((a) => jump(e.key, a));
    }
  };

  // The typed text is forgotten after half a second, so Space takes the option again.
  useEffect(() => {
    if (!open) {
      return;
    }
    const timer = window.setInterval(() => {
      if (typed.current.text !== "" && Date.now() - typed.current.at >= 500) {
        typed.current = { text: "", at: 0 };
      }
    }, 250);
    return () => window.clearInterval(timer);
  }, [open]);

  return (
    <div ref={root} className={cn("relative", size === "form" ? "w-full" : "flex-none")}>
      <button
        ref={button}
        id={id}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? listId : undefined}
        aria-activedescendant={open && !searchable && active >= 0 ? optionId(active) : undefined}
        aria-label={label}
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        aria-invalid={invalid || undefined}
        aria-required={required || undefined}
        disabled={disabled}
        onClick={() => (open ? close() : show())}
        onKeyDown={onButtonKey}
        className={selectButton(size, className)}
      >
        <span className="flex min-w-0 items-center gap-1.5">
          {icon}
          <span className={cn("truncate", !chosen && "text-muted-foreground")}>
            {chosen?.label ?? placeholder ?? ""}
          </span>
        </span>
        <ChevronDown aria-hidden="true" className="size-3.5 flex-none text-muted-foreground" />
      </button>
      {open && (
        <div
          style={place}
          className={cn(
            "z-50 flex flex-col rounded-md border border-input bg-card p-1.5 shadow-lg",
            !place && "absolute top-full left-0 mt-1 min-w-full",
            listWidth,
          )}
        >
          {searchable && (
            <input
              ref={search}
              type="search"
              role="combobox"
              aria-expanded="true"
              aria-controls={listId}
              aria-autocomplete="list"
              aria-activedescendant={active >= 0 ? optionId(active) : undefined}
              aria-label={t("select.search")}
              placeholder={t("select.search")}
              autoComplete="off"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setActive(0);
              }}
              onKeyDown={(e) => {
                if (e.key === "Tab") {
                  close(false);
                  return;
                }
                moves(e, true);
              }}
              className={cn(
                "mb-1 w-full rounded-sm border border-input bg-card px-2 text-sm text-foreground placeholder:text-muted-foreground",
                controlHeight.bar,
              )}
            />
          )}
          <ul
            ref={list}
            id={listId}
            role="listbox"
            tabIndex={-1}
            aria-label={label}
            aria-labelledby={label ? undefined : (labelledBy ?? id)}
            className="flex max-h-72 min-h-0 flex-col gap-px overflow-y-auto"
          >
            {shown.map((o, i) => {
              const selected = o.value === value;
              return (
                // The options take clicks; the keyboard stays on the button or the search field.
                // eslint-disable-next-line jsx-a11y/click-events-have-key-events
                <li
                  key={o.value}
                  id={optionId(i)}
                  role="option"
                  aria-selected={selected}
                  aria-disabled={o.disabled || undefined}
                  onMouseDown={(e) => e.preventDefault()}
                  onMouseMove={() => !o.disabled && i !== active && setActive(i)}
                  onClick={() => take(i)}
                  className={cn(
                    "flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-foreground",
                    i === active && "bg-surface-active",
                    o.disabled && "cursor-default opacity-50",
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">{o.label}</span>
                  <Check
                    aria-hidden="true"
                    className={cn("size-4 flex-none text-accent", !selected && "invisible")}
                  />
                </li>
              );
            })}
            {shown.length === 0 && (
              <li role="presentation" className="px-2 py-1.5 text-sm text-muted-foreground">
                {t("select.none")}
              </li>
            )}
          </ul>
        </div>
      )}
    </div>
  );
}
