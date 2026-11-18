// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The mark of the organisation (UI39, UI plan E51): its logo for the mode, else the badge "sf"
// of ServiceFlow25 (UI56), and its name; the name is the title of the browser, and the main
// action takes the accent if the organisation says so (E25).
import { useEffect } from "react";

import type { Branding } from "@/api/client";
import { applyPrimary } from "@/ui/accent";

export const defaultName = "ServiceFlow25";

// useBranding sets the title of the browser and the colour of the main action.
export function useBranding(branding: Branding | undefined): void {
  useEffect(() => {
    if (!branding) {
      return;
    }
    document.title = branding.name;
    applyPrimary(branding.primary_accent ? branding.accent : null);
  }, [branding]);
}

// BrandMark is the logo of the mode, else the badge. The name stands beside it, so the image
// says nothing of its own to a screen reader.
export function BrandMark({ branding }: { branding: Branding | undefined }) {
  const light = branding?.logo_light ?? branding?.logo_dark ?? null;
  const dark = branding?.logo_dark ?? branding?.logo_light ?? null;
  if (light && dark) {
    return (
      <>
        <img src={light} alt="" className="h-[26px] w-auto max-w-32 object-contain dark:hidden" />
        <img
          src={dark}
          alt=""
          className="hidden h-[26px] w-auto max-w-32 object-contain dark:block"
        />
      </>
    );
  }
  return (
    <span
      aria-hidden="true"
      className="inline-grid size-[26px] place-items-center rounded-md bg-primary font-mono text-xs font-medium text-primary-foreground"
    >
      sf
    </span>
  );
}
