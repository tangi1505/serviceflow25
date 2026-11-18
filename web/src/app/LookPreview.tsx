// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// A small picture of the interface (UI55): the head with the mark, the side bar wide or
// narrow, a list with rows of the chosen height, light or dark, in the accent. It stands beside
// the defaults of the appearance and beside "Darstellung" of the own settings, and changes at
// once when a choice changes. It is one picture to a screen reader, named by the choices.
import { useEffect, useState, type CSSProperties } from "react";
import { useTranslation } from "react-i18next";

import { accentTokens } from "@/ui/accent";
import { cn } from "@/ui/utils";

export type LookPreviewProps = {
  scheme: string;
  density: string;
  nav: string;
  // accent is the colour of the organisation or the person, as #RRGGBB or the name of one of
  // the six; null keeps the accent of the page.
  accent?: string | null;
  primary?: boolean;
  name: string;
  // The logos of the organisation for light and dark; without one the mark "sf" stands there.
  logoLight?: string | null;
  logoDark?: string | null;
};

function systemDark(): MediaQueryList | null {
  return typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;
}

// useSystemDark follows the mode of the system, for the choice "wie das System".
function useSystemDark(): boolean {
  const [dark, setDark] = useState(() => systemDark()?.matches === true);
  useEffect(() => {
    const media = systemDark();
    if (!media) {
      return;
    }
    const follow = (e: MediaQueryListEvent) => setDark(e.matches);
    media.addEventListener("change", follow);
    return () => media.removeEventListener("change", follow);
  }, []);
  return dark;
}

export function LookPreview({
  scheme,
  density,
  nav,
  accent,
  primary,
  name,
  logoLight,
  logoDark,
}: LookPreviewProps) {
  const { t } = useTranslation();
  const systemDark = useSystemDark();
  const dark = scheme === "dark" || (scheme === "system" && systemDark);
  const narrow = nav === "narrow";
  const compact = density === "compact";
  const logo = dark ? (logoDark ?? logoLight) : (logoLight ?? logoDark);
  const style: Record<string, string> = {};
  if (accent) {
    const tokens = accentTokens(accent);
    style["--pick-accent-light"] = tokens.light;
    style["--pick-accent-dark"] = tokens.dark;
    style["--pick-accent-soft-light"] = tokens.softLight;
    style["--pick-accent-soft-dark"] = tokens.softDark;
    if (primary) {
      style["--pick-primary-light"] = tokens.light;
      style["--pick-primary-dark"] = tokens.dark;
    }
  }
  const label = t("look.label", {
    scheme: t(`colorScheme.${scheme}`, { defaultValue: scheme }),
    density: t(`list.densities.${density}`, { defaultValue: density }),
    nav: t(`settings.navWidths.${nav}`, { defaultValue: nav }),
  });
  const rows = [0, 1, 2, 3, 4];
  return (
    <div
      role="img"
      aria-label={label}
      data-color-scheme={dark ? "dark" : "light"}
      style={style as CSSProperties}
      className="flex aspect-[16/11] w-full max-w-80 flex-col overflow-hidden rounded-md border bg-background text-foreground select-none"
    >
      <div className="flex h-6 flex-none items-center gap-1.5 border-b bg-card px-2">
        {logo ? (
          <img src={logo} alt="" className="h-3.5 w-auto max-w-12 object-contain" />
        ) : (
          <span className="inline-grid size-3.5 place-items-center rounded-[3px] bg-primary font-mono text-[7px] leading-none text-primary-foreground">
            sf
          </span>
        )}
        <span className="truncate text-[9px] font-semibold">{name}</span>
        <span className="ml-auto h-2.5 w-16 rounded-sm border bg-background" />
      </div>
      <div className="flex min-h-0 flex-1">
        <div
          className={cn(
            "flex flex-none flex-col gap-1 border-r bg-card p-1.5 transition-[width]",
            narrow ? "w-7" : "w-20",
          )}
        >
          {[0, 1, 2, 3, 4].map((i) => (
            <span
              key={i}
              className={cn(
                "flex h-3.5 items-center gap-1 rounded-sm px-1",
                i === 1 && "bg-surface-active",
              )}
            >
              <span className="size-2 flex-none rounded-[2px] bg-muted-foreground" />
              {!narrow && (
                <span
                  className="h-1 rounded-full bg-foreground-2"
                  style={{ width: `${[60, 75, 50, 70, 55][i]}%` }}
                />
              )}
            </span>
          ))}
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1.5 p-2">
          <div className="flex items-center justify-between">
            <span className="h-1.5 w-12 rounded-full bg-foreground" />
            <span className="h-3 w-7 rounded-[3px] bg-primary" />
          </div>
          <div className="flex flex-col overflow-hidden rounded-[3px] border">
            <span className="h-2.5 flex-none bg-muted" />
            {rows.map((i) => (
              <span
                key={i}
                className={cn(
                  "flex flex-none items-center gap-2 border-t px-1.5",
                  i === 1 && "bg-accent-soft",
                )}
                style={{ height: compact ? 9 : 13 }}
              >
                <span
                  className={cn("h-1 w-8 rounded-full", i === 0 ? "bg-accent" : "bg-foreground-2")}
                />
                <span className="h-1 w-5 rounded-full bg-muted-foreground" />
                <span className="ml-auto h-1 w-4 rounded-full bg-muted-foreground" />
              </span>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
