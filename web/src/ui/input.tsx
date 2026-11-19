// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import type { ComponentProps } from "react";

import { controlHeight, type ControlSize } from "@/ui/select";
import { cn } from "@/ui/utils";

// Input is the one text field (UI57): 36 px in a form, 32 px in a bar or a table head (size
// "bar"); both heights from the tokens.
export function Input({
  className,
  size = "form",
  ...props
}: Omit<ComponentProps<"input">, "size"> & { size?: ControlSize }) {
  return (
    <input
      className={cn(
        // Fields of the kit (UI29, §2.1): corner 6 px, border --input (3:1, E28); an error has a
        // 1.5 px red border; read-only fields stand on the ground with a quiet border.
        "flex w-full rounded-sm border border-input bg-card px-2.5 py-1 text-sm text-foreground placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-60 read-only:border-border read-only:bg-background aria-invalid:border-[1.5px] aria-invalid:border-destructive",
        controlHeight[size],
        size === "bar" && "text-[13px]",
        className,
      )}
      {...props}
    />
  );
}
