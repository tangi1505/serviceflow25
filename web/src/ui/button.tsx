// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// shadcn/ui button (Hauptplan E41), reduced to the variants the Durchstich needs. Colours come
// from the design tokens only, so a theme changes every button (E62).
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";

import { cn } from "@/ui/utils";

export const buttonVariants = cva(
  // Buttons of the kit (UI29, §2.1): 36 px in a form, 32 px in a bar (size "bar", the same as
  // "sm"; both heights from the tokens, UI57), corner 8 px, weight 500; the focus ring is the
  // global one (2 px accent, 2 px away). Disabled: muted ground, muted text, a border.
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md border border-transparent text-sm font-medium transition-colors disabled:pointer-events-none disabled:border-border disabled:bg-muted disabled:text-foreground-2",
  {
    variants: {
      variant: {
        default: "border-primary bg-primary text-primary-foreground hover:bg-primary/90",
        secondary: "border-input bg-card text-foreground hover:bg-surface-active",
        outline: "border-input bg-card text-foreground hover:bg-surface-active",
        ghost: "text-foreground hover:bg-surface-active",
        // Danger: card ground, text and border in red.
        destructive: "border-destructive bg-card text-destructive hover:bg-destructive-background",
      },
      size: {
        default: "h-[var(--control-form)] px-3.5",
        bar: "h-[var(--control-bar)] px-2.5 text-[13px]",
        sm: "h-[var(--control-bar)] px-2.5 text-[13px]",
        lg: "h-11 px-6",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export type ButtonProps = ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & { asChild?: boolean };

export function Button({ className, variant, size, asChild = false, type, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : "button";
  return (
    <Comp
      className={cn(buttonVariants({ variant, size }), className)}
      type={asChild ? type : (type ?? "button")}
      {...props}
    />
  );
}
