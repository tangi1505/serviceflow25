// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A plain table with the look of the list of records (UI54): a frame, the grey head, the row
// height of the person's density and the colour of a row under the mouse. Rows that lead
// somewhere are whole rows to click, reached with Tab and moved through with the arrows, Pos1
// and Ende like the list (UI53); Enter opens. Tables of roles, types, workflow, changes,
// approvals and substitutes use it, the list of records keeps its own (it is virtual).
import {
  createContext,
  useContext,
  type HTMLAttributes,
  type KeyboardEvent,
  type ReactNode,
  type TdHTMLAttributes,
  type ThHTMLAttributes,
} from "react";

import { cn } from "@/ui/utils";

export type Density = "normal" | "compact";

// The heights of a row, the same as in the list of records.
export const rowHeights: Record<Density, number> = { compact: 36, normal: 52 };

const DensityContext = createContext<Density>("normal");

export function Table({
  density = "normal",
  className,
  children,
  ...rest
}: { density?: Density } & HTMLAttributes<HTMLTableElement>) {
  return (
    <DensityContext.Provider value={density}>
      <div className="overflow-x-auto rounded-md border">
        <table className={cn("w-full border-collapse text-left text-sm", className)} {...rest}>
          {children}
        </table>
      </div>
    </DensityContext.Provider>
  );
}

export function THead({ children }: { children: ReactNode }) {
  return <thead className="bg-muted">{children}</thead>;
}

export function Th({ className, ...rest }: ThHTMLAttributes<HTMLTableCellElement>) {
  return (
    <th
      scope="col"
      className={cn("px-3 py-2 text-left font-medium whitespace-nowrap", className)}
      {...rest}
    />
  );
}

// Td is a cell; its height follows the density of the table.
export function Td({ className, style, ...rest }: TdHTMLAttributes<HTMLTableCellElement>) {
  const density = useContext(DensityContext);
  return (
    <td
      className={cn("px-3 py-1.5 align-middle", className)}
      style={{ height: rowHeights[density], ...style }}
      {...rest}
    />
  );
}

// move gives the focus to the row before or after, the first or the last of its body.
function move(e: KeyboardEvent<HTMLTableRowElement>) {
  const rows = Array.from(
    e.currentTarget.parentElement?.querySelectorAll<HTMLTableRowElement>(":scope > tr[tabindex]") ??
      [],
  );
  const at = rows.indexOf(e.currentTarget);
  const to =
    e.key === "ArrowDown"
      ? at + 1
      : e.key === "ArrowUp"
        ? at - 1
        : e.key === "Home"
          ? 0
          : rows.length - 1;
  rows[to]?.focus();
}

// Tr is a row of the body. With onOpen the whole row leads there: by click, by Enter, and the
// arrows move to the next row; controls inside the row keep their own keys and clicks.
export function Tr({
  onOpen,
  className,
  children,
  ...rest
}: { onOpen?: () => void } & HTMLAttributes<HTMLTableRowElement>) {
  if (!onOpen) {
    return (
      <tr className={cn("border-t", className)} {...rest}>
        {children}
      </tr>
    );
  }
  return (
    <tr
      tabIndex={0}
      className={cn(
        "cursor-pointer border-t outline-none hover:bg-surface-active focus-visible:bg-surface-active focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
        className,
      )}
      onClick={(e) => {
        // A button or link inside the row does its own thing.
        if ((e.target as HTMLElement).closest("a, button, input, select, textarea, label")) {
          return;
        }
        onOpen();
      }}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) {
          return;
        }
        if (e.key === "Enter") {
          e.preventDefault();
          onOpen();
        } else if (["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
          e.preventDefault();
          move(e);
        }
      }}
      {...rest}
    >
      {children}
    </tr>
  );
}
