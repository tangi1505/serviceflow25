// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The preview of the chosen record beside the list (UI30, UI-Plan E35): title, state with its
// transitions, the SLA clocks and the first fields; "Öffnen" leads to the record page. The list
// stays where it is, so that one can go through it with the preview open.
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { qualifiedName, readQuery, type TypeDefinition } from "@/api/client";
import { Button } from "@/ui/button";
import { PriorityMark } from "@/ui/marks";

import { fieldLabel, formatValue, isCalculated } from "./fields";
import { titleOf } from "./RefName";
import { SlaClocks } from "./Sidebar";
import { Transitions } from "./Transitions";

// The fields of the preview: what one reads first, without long text.
const shown = 6;

export function Preview({
  type,
  id,
  onClose,
}: {
  type: TypeDefinition;
  id: string;
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const typeName = qualifiedName(type);
  const record = useQuery(readQuery(typeName, id));
  const title = record.data ? titleOf(type, record.data) : "";
  const titleField = type.title_field ?? type.fields[0]?.name;
  const fields = type.fields
    .filter((f) => !f.hidden && f.name !== titleField && f.format !== "rich" && !isCalculated(f))
    .slice(0, shown);
  return (
    <aside
      aria-label={t("preview.label")}
      className="flex w-full shrink-0 flex-col gap-3 self-start rounded-lg border bg-card p-4 lg:w-[400px]"
    >
      <div className="flex items-start justify-between gap-2">
        <h2 className="text-[15px] font-semibold leading-snug">
          {record.isPending ? t("list.loading") : title}
        </h2>
        <Button variant="ghost" size="sm" aria-label={t("preview.close")} onClick={onClose}>
          ✕
        </Button>
      </div>
      {record.data && (
        <>
          <Transitions type={type} record={record.data} />
          <SlaClocks typeName={typeName} id={id} />
          <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-sm">
            {fields.map((f) => {
              const v = record.data[f.name];
              return (
                <div key={f.name} className="contents">
                  <dt className="text-foreground-2">{fieldLabel(f, i18n.language)}</dt>
                  <dd className="truncate">
                    {f.format === "priority" && typeof v === "number" ? (
                      <PriorityMark value={v} />
                    ) : (
                      formatValue(f, v, i18n.language) || "–"
                    )}
                  </dd>
                </div>
              );
            })}
          </dl>
          <div>
            <Button asChild>
              <Link to="/types/$typeName/$id" params={{ typeName, id }}>
                {t("preview.open")}
              </Link>
            </Button>
          </div>
        </>
      )}
    </aside>
  );
}
