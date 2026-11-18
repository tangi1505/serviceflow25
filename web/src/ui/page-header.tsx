// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The head of a page (UI47, UI-Plan E54, E56): the title, below it a line such as the number of
// records, and on the right the actions as buttons. "Zurück" to the page it came from stands
// before the title as a button, not as an underlined word: links stay for references to
// records in text, tables and the timeline.
import { Link, type LinkProps } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";

import { barButton } from "@/ui/menu";
import { cn } from "@/ui/utils";

export function PageHeader({
  title,
  line,
  back,
  actions,
  className,
}: {
  title: ReactNode;
  // line is a short line under the title, read out when it changes (the number of records).
  line?: ReactNode;
  // back leads to the page this one belongs to, such as the list of a record.
  back?: { label: string } & LinkProps;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("mb-4 flex flex-col gap-2", className)}>
      {back && <BackButton {...back} />}
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-col gap-0.5">
          <h1 className="text-2xl font-semibold break-words">{title}</h1>
          {line !== undefined && (
            <span aria-live="polite" className="text-[13px] text-foreground-2">
              {line}
            </span>
          )}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </div>
  );
}

// BackButton leads back as a button with an arrow (a link underneath, since it goes to a page).
export function BackButton({ label, ...link }: { label: string } & LinkProps) {
  return (
    <Link {...link} className={cn(barButton, "self-start pl-2")}>
      <ArrowLeft aria-hidden="true" className="size-4 text-foreground-2" />
      {label}
    </Link>
  );
}

// LinkButton is another page of the same thing, shown as a button (Typ-Editor ↔ Workflow).
export function LinkButton({
  children,
  icon,
  ...link
}: LinkProps & { children: ReactNode; icon?: ReactNode }) {
  return (
    <Link {...link} className={barButton}>
      {icon}
      {children}
    </Link>
  );
}
