// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The tiles of the start page (SD14, SD-Plan E13, variant A of the mockup): "Was möchten Sie
// tun?" with the tiles the person may see (core.portal_tile; the server keeps to "Sichtbar
// für"), in their order. A tile leads to creating a record of a type, a form, a reading area,
// one record or a link outside; one whose goal the person cannot reach is left out, a link
// only with http or https. Each tile shows a symbol in a box over its name (UI42, UI plan
// E53): its own from the fixed list, else the one of its goal.
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { BookOpen, ExternalLink, type LucideIcon } from "lucide-react";
import { type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  formsQuery,
  packagesQuery,
  qualifiedName,
  typesQuery,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { packageIcon, packageIcons } from "@/app/icons";

import { creates } from "./Read";

export const TileType = "core.portal_tile";

const tilesQuery = {
  queryKey: ["portal", "tiles"],
  queryFn: async () =>
    (await api.query(TileType, { sort: ["position"], limit: 100 })) as { items: RecordData[] },
  staleTime: 60_000,
  // Without the type of the tiles there are none: no waiting for retries, the forms show.
  retry: false,
};

// Shown is a tile as the start page draws it: where it leads and its symbol, a name of the
// fixed list or "external-link" for a link outside.
export type Shown = {
  id: string;
  name: string;
  description: string;
  icon: string;
  // number says the tile shows the number of a dashboard (UI13b).
  number?: boolean;
  goal:
    | { to: "/portal/$typeName/new"; params: { typeName: string } }
    | { to: "/portal/forms/$formId"; params: { formId: string } }
    | { to: "/portal/read/$typeName"; params: { typeName: string } }
    | { to: "/portal/read/$typeName/$id"; params: { typeName: string; id: string } }
    | { to: "/portal/$typeName/$id"; params: { typeName: string; id: string } }
    | { to: "/portal" }
    | { href: string };
};

// shown resolves a tile against what the person may do: the types she reads and creates and
// the forms offered to her. iconOf gives the symbol of a type's package.
export function shown(
  tile: RecordData,
  types: TypeDefinition[],
  forms: string[],
  iconOf: (type: TypeDefinition) => string,
): Shown | null {
  const s = (k: string) => (typeof tile[k] === "string" ? String(tile[k]) : "");
  const type = types.find((x) => qualifiedName(x) === s("target_type"));
  // The tile's own symbol wins; without one, the symbol of its goal (UI42, E53).
  const own = packageIcons[s("icon")] ? s("icon") : "";
  const base = { id: s("id"), name: s("name"), description: s("description") };
  const area = (t: TypeDefinition) => t.portal?.read?.icon ?? "book-open";
  const icon = (fallback: string) => ({ icon: own || fallback });
  switch (s("kind")) {
    case "create":
      return type && creates(type)
        ? {
            ...base,
            ...icon(iconOf(type)),
            goal: { to: "/portal/$typeName/new", params: { typeName: qualifiedName(type) } },
          }
        : null;
    case "form":
      return forms.includes(s("form"))
        ? {
            ...base,
            ...icon("file-text"),
            goal: { to: "/portal/forms/$formId", params: { formId: s("form") } },
          }
        : null;
    case "area":
      return type?.portal?.read
        ? {
            ...base,
            ...icon(area(type)),
            goal: { to: "/portal/read/$typeName", params: { typeName: qualifiedName(type) } },
          }
        : null;
    case "record":
      if (!type || !s("target_id")) {
        return null;
      }
      if (type.portal?.read) {
        return {
          ...base,
          ...icon(area(type)),
          goal: {
            to: "/portal/read/$typeName/$id",
            params: { typeName: qualifiedName(type), id: s("target_id") },
          },
        };
      }
      return creates(type)
        ? {
            ...base,
            ...icon(iconOf(type)),
            goal: {
              to: "/portal/$typeName/$id",
              params: { typeName: qualifiedName(type), id: s("target_id") },
            },
          }
        : null;
    case "dashboard":
      // The number of a dashboard (UI13b): it leads to one's own records below the tiles.
      return s("widget")
        ? { ...base, ...icon("chart-column"), number: true, goal: { to: "/portal" } }
        : null;
    case "link":
      return /^https?:\/\//i.test(s("url"))
        ? { ...base, ...icon("external-link"), goal: { href: s("url") } }
        : null;
  }
  return null;
}

// useTiles reads the tiles and resolves them; pending until all it needs is there.
export function useTiles(): { pending: boolean; tiles: Shown[] } {
  const tiles = useQuery(tilesQuery);
  const types = useQuery(typesQuery);
  const forms = useQuery(formsQuery);
  // The symbols of the packages (UI36); every session may read them. Without them a box.
  const packages = useQuery(packagesQuery);
  if (tiles.isPending || types.isPending || forms.isPending || packages.isPending) {
    return { pending: true, tiles: [] };
  }
  const offered = (forms.data ?? []).map((f) => f.id);
  const iconOf = (type: TypeDefinition) =>
    (packages.data ?? []).find((p) => p.name === type.package)?.icon ?? "box";
  return {
    pending: false,
    tiles: (tiles.data?.items ?? [])
      .map((x) => shown(x, types.data ?? [], offered, iconOf))
      .filter((x): x is Shown => x !== null),
  };
}

// tileIcon is the lucide symbol of a name of Shown.
function tileIcon(name: string): LucideIcon {
  if (name === "external-link") {
    return ExternalLink;
  }
  return name === "book-open" ? BookOpen : packageIcon(name);
}

const tileClass =
  "flex min-h-24 flex-col gap-1.5 rounded-lg border bg-card p-4 text-foreground no-underline hover:border-foreground focus-visible:outline-2 focus-visible:outline-ring";

export function TileGrid({ tiles }: { tiles: Shown[] }) {
  const { t } = useTranslation();
  const body = (tile: Shown): ReactNode => {
    const Icon = tileIcon(tile.icon);
    return (
      <>
        {/* The symbol in its box over the name (UI42, E53); the kind of goal no longer stands
          above the name. */}
        <span
          aria-hidden="true"
          data-icon={tile.icon}
          className="mb-1 inline-grid size-9 flex-none place-items-center rounded-md bg-surface-active text-primary"
        >
          <Icon className="size-5" />
        </span>
        {tile.number && <TileNumber id={tile.id} />}
        <b className="text-[15px] font-semibold">{tile.name}</b>
        {tile.description && <span className="text-sm">{tile.description}</span>}
      </>
    );
  };
  return (
    <section aria-labelledby="portal-tiles" className="flex flex-col gap-3">
      <h1 id="portal-tiles" className="text-2xl font-semibold">
        {t("portal.tilesTitle")}
      </h1>
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,15rem),1fr))] gap-3">
        {tiles.map((tile) => (
          <li key={tile.id} className="flex">
            {"href" in tile.goal ? (
              <a
                href={tile.goal.href}
                target="_blank"
                rel="noopener noreferrer"
                className={`${tileClass} w-full`}
              >
                {body(tile)}
                <span className="sr-only">{t("portal.opensNew")}</span>
              </a>
            ) : (
              <Link {...tile.goal} className={`${tileClass} w-full`}>
                {body(tile)}
              </Link>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

// TileNumber is the number of a tile of the kind dashboard, with the person's rights (UI13b).
function TileNumber({ id }: { id: string }) {
  const { i18n } = useTranslation();
  const data = useQuery({
    queryKey: ["portal", "tiles", id, "data"],
    queryFn: () => api.tileData(id),
    retry: false,
  });
  const value = Number(data.data?.value ?? 0);
  return (
    <span className="text-3xl font-semibold tracking-tight">
      {data.isSuccess
        ? new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(value)
        : "…"}
    </span>
  );
}
