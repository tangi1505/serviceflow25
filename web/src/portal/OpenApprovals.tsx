// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// OpenApprovals is "Wartet auf Ihre Genehmigung" on the start page of the portal (UI44, UI plan
// E43): the person's open questions, each with "Zustimmen" and "Ablehnen" as on the page of
// the approval; a refusal needs its reason there as here. Without an open question the section
// is not there. A decision takes the row away and lowers the number at the area
// "Genehmigungen", which reads the same query.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  myApprovalsQuery,
  qualifiedName,
  typesQuery,
  type MyApproval,
} from "@/api/client";
import { formatMoment } from "@/render/fields";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";

export function OpenApprovals() {
  const { t } = useTranslation();
  const open = useQuery(myApprovalsQuery);
  const items = open.data ?? [];
  if (items.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="portal-approvals" className="flex flex-col gap-3">
      <h2 id="portal-approvals" className="text-xl font-semibold">
        {t("approvals.waitingForYou")}
      </h2>
      <ul className="flex flex-col divide-y rounded-md border bg-card">
        {items.map((m) => (
          <Row key={m.step} item={m} />
        ))}
      </ul>
    </section>
  );
}

function Row({ item }: { item: MyApproval }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const types = useQuery(typesQuery);
  const fieldId = useId();
  // refusing shows the field for the reason; the refusal waits for it.
  const [refusing, setRefusing] = useState(false);
  const [reason, setReason] = useState("");
  const [missing, setMissing] = useState(false);
  const decide = useMutation({
    mutationFn: (decision: "approve" | "reject") =>
      api.decideApproval(item.type_name, item.record_id, item.approval, {
        decision,
        step: item.step,
        ...(decision === "reject" ? { comment: reason } : {}),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["approvals"] });
      void queryClient.invalidateQueries({ queryKey: ["records", item.type_name] });
    },
  });
  const type = types.data?.find((x) => qualifiedName(x) === item.type_name);
  const name = item.title || item.record_id;
  return (
    <li className="flex flex-col gap-3 px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <Link
            to="/portal/approvals/$typeName/$id"
            params={{ typeName: item.type_name, id: item.record_id }}
            className="font-medium underline-offset-2 hover:underline focus-visible:underline"
          >
            {name}
          </Link>
          <span className="text-sm text-muted-foreground">
            {[
              labelOf(type?.label, i18n.language, item.type_name),
              item.stages > 1 ? t("approvals.stage", { n: item.stage, of: item.stages }) : null,
              formatMoment(item.requested_at, i18n.language),
            ]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </div>
        {!refusing && (
          <div className="flex flex-none gap-2">
            <Button
              type="button"
              size="sm"
              disabled={decide.isPending}
              aria-label={t("approvals.approveOf", { title: name })}
              onClick={() => decide.mutate("approve")}
            >
              {t("approvals.approve")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={decide.isPending}
              aria-label={t("approvals.rejectOf", { title: name })}
              onClick={() => setRefusing(true)}
            >
              {t("approvals.reject")}
            </Button>
          </div>
        )}
      </div>
      {refusing && (
        <form
          className="flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            if (reason.trim()) {
              decide.mutate("reject");
            } else {
              setMissing(true);
            }
          }}
        >
          <Label htmlFor={fieldId}>{t("approvals.reason")}</Label>
          <textarea
            id={fieldId}
            // The field appears at the person's request: the focus goes there.
            // eslint-disable-next-line jsx-a11y/no-autofocus
            autoFocus
            value={reason}
            onChange={(e) => {
              setReason(e.target.value);
              setMissing(false);
            }}
            aria-invalid={missing}
            aria-describedby={missing ? `${fieldId}-err` : undefined}
            className="min-h-16 rounded-md border border-input bg-card px-2.5 py-2 text-sm"
          />
          {missing && (
            <p id={`${fieldId}-err`} role="alert" className="text-[12.5px] text-destructive">
              {t("approvals.reasonMissing")}
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" size="sm" variant="outline" disabled={decide.isPending}>
              {t("approvals.reject")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setRefusing(false);
                setReason("");
                setMissing(false);
              }}
            >
              {t("approvals.cancel")}
            </Button>
          </div>
        </form>
      )}
      {decide.isError && (
        <Alert variant="destructive">
          {t("approvals.error", {
            detail: decide.error instanceof Error ? decide.error.message : String(decide.error),
          })}
        </Alert>
      )}
    </li>
  );
}
