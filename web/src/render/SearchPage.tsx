// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Every hit of a search (UI30): the full-text search over every type one may read (PL26), best
// first, each a link to its record. The command line (UI31) leads here with its last option.
import { useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { api, labelOf, qualifiedName, typesQuery } from "@/api/client";
import { Alert } from "@/ui/alert";

export function SearchPage() {
  const { t, i18n } = useTranslation();
  const { q = "" } = useSearch({ from: "/shell/search" });
  const types = useQuery(typesQuery);
  const hits = useQuery({
    queryKey: ["search", q],
    queryFn: () => api.search(q, "", 50),
    enabled: q.trim() !== "",
  });
  const typeLabel = (name: string) =>
    labelOf(types.data?.find((x) => qualifiedName(x) === name)?.label, i18n.language, name);
  return (
    <div className="flex max-w-4xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">{t("find.title", { q })}</h1>
      {q.trim() === "" && <p className="text-sm text-foreground-2">{t("find.empty")}</p>}
      {hits.isPending && q.trim() !== "" && <p>{t("list.loading")}</p>}
      {hits.isError && <Alert variant="destructive">{t("list.error")}</Alert>}
      {hits.data && hits.data.length === 0 && <p>{t("find.none")}</p>}
      {hits.data && hits.data.length > 0 && (
        <ul className="flex flex-col divide-y rounded-lg border bg-card">
          {hits.data.map((h) => (
            <li key={`${h.type}:${h.id}`} className="flex flex-wrap items-baseline gap-2 px-4 py-2">
              <Link
                to="/types/$typeName/$id"
                params={{ typeName: h.type, id: h.id }}
                className="font-medium text-accent underline"
              >
                {h.title ?? h.id}
              </Link>
              <span className="text-sm text-foreground-2">{typeLabel(h.type)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
