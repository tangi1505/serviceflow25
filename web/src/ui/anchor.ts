// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// useAnchored places an open list under its field in fixed position (UI57, UI61): a list inside a
// box that scrolls, such as the head of a table, is not cut off by the box. Above the field when
// there is more room there; it follows when the page or a box scrolls.
import { useLayoutEffect, useState, type CSSProperties, type RefObject } from "react";

export function useAnchored(
  anchor: RefObject<HTMLElement | null>,
  open: boolean,
): CSSProperties | undefined {
  const [style, setStyle] = useState<CSSProperties>();
  useLayoutEffect(() => {
    if (!open) {
      return;
    }
    const place = () => {
      const r = anchor.current?.getBoundingClientRect();
      if (!r) {
        return;
      }
      const below = window.innerHeight - r.bottom;
      const up = below < 240 && r.top > below;
      setStyle({
        position: "fixed",
        left: Math.max(8, r.left),
        minWidth: r.width,
        maxHeight: Math.max(120, (up ? r.top : below) - 12),
        ...(up ? { bottom: window.innerHeight - r.top + 4 } : { top: r.bottom + 4 }),
      });
    };
    place();
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [anchor, open]);
  return open ? style : undefined;
}
