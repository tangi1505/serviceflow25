// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A button that opens a menu (UI47, UI-Plan E54, E56): "Anpassen" in the page header, "Ansicht",
// "Exportieren" and the saved searches in the bar of a list. 32 px high like everything in the
// bar. The menu follows the WAI-ARIA pattern "menu button": Enter, Space or a click open it and
// put the focus on its first item, the arrow keys, Home and End move, Esc closes it and gives
// the focus back to the button (usePopover, menuKeys).
import { Link, type LinkProps } from "@tanstack/react-router";
import { Check, ChevronDown } from "lucide-react";
import { createContext, useContext, type ComponentProps, type ReactNode } from "react";

import { menuKeys, usePopover } from "@/ui/popover";
import { cn } from "@/ui/utils";

// barButton is the look of a button in a page header or a bar: 32 px, card ground, a border.
export const barButton =
  "inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-md border border-input bg-card px-2.5 text-[13px] font-medium text-foreground hover:bg-surface-active focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-60";

// iconBarButton is a button of the bar that shows only its symbol; its name is its label and
// its tooltip.
// primaryBarButton is the one main action of a page, such as "Neu" (32 px, like barButton).
export const primaryBarButton =
  "inline-flex h-8 items-center gap-1.5 whitespace-nowrap rounded-md border border-primary bg-primary px-2.5 text-[13px] font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

export const iconBarButton =
  "inline-grid size-8 flex-none place-items-center rounded-md border border-input bg-card text-foreground-2 hover:bg-surface-active hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

const itemClass =
  "flex w-full items-center gap-2.5 rounded-sm px-2 py-1.5 text-left text-sm text-foreground hover:bg-surface-active focus-visible:bg-surface-active focus-visible:outline-none";

const MenuClose = createContext<() => void>(() => undefined);

export function Menu({
  label,
  buttonLabel,
  trigger,
  title,
  iconOnly = false,
  primary = false,
  align = "end",
  width = "w-64",
  children,
}: {
  // label names the menu for assistive technology (and the button when it shows a symbol only).
  label: string;
  // buttonLabel names the button when what it shows needs context (the saved search shown).
  buttonLabel?: string;
  // trigger is what the button shows: a symbol with a name, or only a symbol.
  trigger: ReactNode;
  // title is the tooltip of the button.
  title?: string;
  iconOnly?: boolean;
  // primary makes the button the main action of the page.
  primary?: boolean;
  align?: "start" | "end";
  width?: string;
  children: ReactNode;
}) {
  const pop = usePopover();
  return (
    <div className="relative flex-none">
      <button
        ref={pop.trigger}
        type="button"
        aria-haspopup="menu"
        aria-expanded={pop.open}
        aria-label={buttonLabel ?? (iconOnly ? label : undefined)}
        title={title}
        onClick={pop.toggle}
        className={iconOnly ? iconBarButton : primary ? primaryBarButton : barButton}
      >
        {trigger}
        {!iconOnly && (
          <ChevronDown
            aria-hidden="true"
            className={cn("size-3.5 flex-none", primary ? "opacity-80" : "text-muted-foreground")}
          />
        )}
      </button>
      {pop.open && (
        <div
          ref={pop.panel}
          role="menu"
          tabIndex={-1}
          aria-label={label}
          onKeyDown={menuKeys}
          className={cn(
            "absolute top-full z-50 mt-1 flex flex-col gap-px rounded-md border border-input bg-card p-1.5 shadow-lg",
            align === "end" ? "right-0" : "left-0",
            width,
          )}
        >
          <MenuClose.Provider value={() => pop.close()}>{children}</MenuClose.Provider>
        </div>
      )}
    </div>
  );
}

// MenuItem is an entry that does something; the menu closes after it.
export function MenuItem({
  onSelect,
  description,
  icon,
  children,
  className,
  ...rest
}: Omit<ComponentProps<"button">, "onSelect"> & {
  onSelect: () => void;
  description?: string;
  icon?: ReactNode;
}) {
  const close = useContext(MenuClose);
  return (
    <button
      type="button"
      role="menuitem"
      className={cn(itemClass, description && "items-start py-2", className)}
      onClick={() => {
        close();
        onSelect();
      }}
      {...rest}
    >
      {icon}
      <Body description={description}>{children}</Body>
    </button>
  );
}

// MenuLink is an entry that leads to another page.
export function MenuLink({
  description,
  icon,
  children,
  ...link
}: LinkProps & { description?: string; icon?: ReactNode; children: ReactNode }) {
  const close = useContext(MenuClose);
  return (
    <Link
      {...link}
      role="menuitem"
      className={cn(itemClass, description && "items-start py-2")}
      onClick={() => close()}
    >
      {icon}
      <Body description={description}>{children}</Body>
    </Link>
  );
}

// MenuCheck is an entry that stands for a choice: a radio (one of several) or a checkbox.
export function MenuCheck({
  checked,
  kind = "radio",
  keepOpen = false,
  onSelect,
  children,
}: {
  checked: boolean;
  kind?: "radio" | "checkbox";
  // keepOpen leaves the menu open, so that several columns can be chosen one after another.
  keepOpen?: boolean;
  onSelect: () => void;
  children: ReactNode;
}) {
  const close = useContext(MenuClose);
  return (
    <button
      type="button"
      role={kind === "radio" ? "menuitemradio" : "menuitemcheckbox"}
      aria-checked={checked}
      className={cn(itemClass, kind === "radio" && checked && "font-medium")}
      onClick={() => {
        if (!keepOpen) {
          close();
        }
        onSelect();
      }}
    >
      {kind === "checkbox" && (
        <span
          aria-hidden="true"
          className={cn(
            "grid size-4 flex-none place-items-center rounded-[4px] border",
            checked ? "border-primary bg-primary text-primary-foreground" : "border-input",
          )}
        >
          {checked && <Check className="size-3" strokeWidth={3.5} />}
        </span>
      )}
      <span className="flex-1">{children}</span>
      {kind === "radio" && checked && (
        <Check aria-hidden="true" className="size-4 flex-none text-primary" />
      )}
    </button>
  );
}

// MenuLabel is a heading inside a menu; MenuSeparator a line between groups.
export function MenuLabel({ children }: { children: ReactNode }) {
  return (
    <p className="px-2 pt-1.5 pb-1 text-xs font-medium tracking-wide text-muted-foreground uppercase">
      {children}
    </p>
  );
}

export function MenuSeparator() {
  return <div aria-hidden="true" className="mx-0.5 my-1 h-px bg-border" />;
}

function Body({ description, children }: { description?: string; children: ReactNode }) {
  if (!description) {
    return <span className="flex-1">{children}</span>;
  }
  return (
    <span className="flex flex-1 flex-col">
      <span className="font-medium">{children}</span>
      <span className="text-xs text-foreground-2">{description}</span>
    </span>
  );
}
