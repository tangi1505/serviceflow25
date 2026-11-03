// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import type { ComponentProps } from "react";

import { cn } from "@/ui/utils";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return (
    <input
      className={cn(
        // Fields of the kit (UI29, §2.1): 36 px, corner 6 px, border --input (3:1, E28); an error
        // has a 1.5 px red border; read-only fields stand on the ground with a quiet border.
        "flex h-9 w-full rounded-sm border border-input bg-card px-2.5 py-1 text-sm text-foreground placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60 read-only:border-border read-only:bg-background aria-invalid:border-[1.5px] aria-invalid:border-destructive",
        className,
      )}
      {...props}
    />
  );
}
