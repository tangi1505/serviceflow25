// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// One record to approve in the portal (WF6, variant 3B of the mockup): a person without the
// workspace, a lead for example, reads what she is asked about, with its fields and the
// stages, and agrees or refuses with a reason below. She reads it because she is asked.
import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";

import { labelOf, qualifiedName, readQuery, typesQuery } from "@/api/client";
import { Approvals } from "@/render/Approvals";
import { fieldLabel, stateCategory } from "@/render/fields";
import { Alert } from "@/ui/alert";

import { FieldValue } from "./Read";

export function PortalApprovalPage() {
  const { typeName, id } = useParams({ strict: false }) as { typeName: string; id: string };
  const { t, i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const rec = useQuery({ ...readQuery(typeName, id), retry: false });
  const type = types.data?.find((x) => qualifiedName(x) === typeName);
  if (rec.isError) {
    return <Alert variant="destructive">{t("approvals.notReadable")}</Alert>;
  }
  if (!type || !rec.data) {
    return <p className="text-sm text-muted-foreground">{t("approvals.loading")}</p>;
  }
  const data = rec.data;
  const title = type.title_field ? String(data[type.title_field] ?? "") : id;
  const state = type.states?.find((s) => s.name === data["state"]);
  const fields = type.fields.filter(
    (f) =>
      f.name !== type.title_field &&
      !f.hidden &&
      data[f.name] !== undefined &&
      data[f.name] !== null &&
      data[f.name] !== "",
  );
  return (
    <div className="flex flex-col gap-5">
      <nav aria-label="breadcrumb" className="text-sm text-muted-foreground">
        <Link to="/portal/approvals" className="underline">
          {t("approvals.portalTitle")}
        </Link>
      </nav>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-col gap-1">
          <span className="text-[13px] text-muted-foreground">
            {labelOf(type.label, i18n.language, typeName)}
          </span>
          <h1 className="text-2xl font-semibold">{title}</h1>
        </div>
        {state && (
          <span
            className="rounded-full border bg-card px-3 py-0.5 text-[13px]"
            data-category={stateCategory(state)}
          >
            {labelOf(state.label, i18n.language, state.name)}
          </span>
        )}
      </div>
      {fields.length > 0 && (
        <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 rounded-md border bg-card p-4 text-sm">
          {fields.map((f) => (
            <div key={f.name} className="contents">
              <dt className="text-muted-foreground">{fieldLabel(f, i18n.language)}</dt>
              <dd>
                <FieldValue field={f} value={data[f.name]} />
              </dd>
            </div>
          ))}
        </dl>
      )}
      <Approvals type={type} id={id} manage={false} />
    </div>
  );
}
