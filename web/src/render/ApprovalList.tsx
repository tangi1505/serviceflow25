// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The list "Zu genehmigen" (WF6, WF-Plan E17): in the workspace below the entry of the
// navigation, in the portal as the area "Genehmigungen" (variant 3B of the mockup). Open are
// the questions the person may answer now, as substitute too; answered those she decided.
// A row leads to the record: in the workspace its page, in the portal a page to read it and
// to answer.
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { api, labelOf, qualifiedName, typesQuery, type MyApproval } from "@/api/client";
import { Alert } from "@/ui/alert";
import { Table, Td, Th, THead, Tr } from "@/ui/table";

import { formatMoment } from "./fields";
import { RefName } from "./RefName";
import { useDensity } from "./TableTools";

export function ApprovalList({ portal = false }: { portal?: boolean }) {
  const { t, i18n } = useTranslation();
  const [state, setState] = useState<"open" | "done">("open");
  const types = useQuery(typesQuery);
  const list = useQuery({
    queryKey: ["approvals", "mine", state],
    queryFn: () => api.myApprovals(state),
    retry: false,
  });
  const typeLabel = (name: string) =>
    labelOf(types.data?.find((x) => qualifiedName(x) === name)?.label, i18n.language, name);
  const seg = "inline-flex h-8 items-center rounded-md px-3.5 text-sm font-medium";
  const navigate = useNavigate();
  const density = useDensity();
  const open = (m: MyApproval) =>
    void (portal
      ? navigate({
          to: "/portal/approvals/$typeName/$id",
          params: { typeName: m.type_name, id: m.record_id },
        })
      : navigate({
          to: "/types/$typeName/$id",
          params: { typeName: m.type_name, id: m.record_id },
        }));
  const goal = (m: MyApproval) =>
    portal ? (
      <Link
        to="/portal/approvals/$typeName/$id"
        params={{ typeName: m.type_name, id: m.record_id }}
        className="underline"
        tabIndex={-1}
      >
        {m.title || m.record_id}
      </Link>
    ) : (
      <Link
        to="/types/$typeName/$id"
        params={{ typeName: m.type_name, id: m.record_id }}
        className="underline"
        tabIndex={-1}
      >
        {m.title || m.record_id}
      </Link>
    );
  return (
    <section aria-labelledby="my-approvals" className="flex flex-col gap-4">
      <h1 id="my-approvals" className="text-2xl font-semibold">
        {portal ? t("approvals.portalTitle") : t("nav.approvals")}
      </h1>
      <div
        role="group"
        aria-label={t("approvals.filter")}
        className="inline-flex gap-0.5 self-start rounded-md bg-muted p-0.5"
      >
        {(["open", "done"] as const).map((s) => (
          <button
            key={s}
            type="button"
            aria-pressed={state === s}
            onClick={() => setState(s)}
            className={
              seg +
              (state === s ? " bg-segment-active text-foreground shadow-sm" : " text-foreground-2")
            }
          >
            {t(`approvals.list.${s}`)}
          </button>
        ))}
      </div>
      {list.isError && <Alert variant="destructive">{t("approvals.listError")}</Alert>}
      {list.data && list.data.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {state === "open" ? t("approvals.nothingOpen") : t("approvals.nothingDone")}
        </p>
      )}
      {list.data && list.data.length > 0 && (
        <Table density={density}>
          <THead>
            <tr>
              <Th>{t("approvals.record")}</Th>
              <Th>{t("approvals.kind")}</Th>
              <Th>{t("approvals.from")}</Th>
              <Th>{t("approvals.stageColumn")}</Th>
              <Th>{state === "open" ? t("approvals.since") : t("approvals.decided")}</Th>
            </tr>
          </THead>
          <tbody>
            {list.data.map((m) => (
              <Tr key={m.step} onOpen={() => open(m)}>
                <Td>
                  {goal(m)}
                  {m.on_behalf && (
                    <span className="block text-[12.5px] text-muted-foreground">
                      {t("approvals.standIn")}{" "}
                      <RefName refTo={{ type: "core.person", id: m.on_behalf }} />
                    </span>
                  )}
                </Td>
                <Td>{typeLabel(m.type_name)}</Td>
                <Td>
                  {m.requester ? <RefName refTo={{ type: "core.person", id: m.requester }} /> : ""}
                </Td>
                <Td>
                  {t("approvals.stage", { n: m.stage, of: m.stages })}
                  {m.stage_name ? ` · ${m.stage_name}` : ""}
                  {state === "done" && (
                    <span
                      className={
                        "ml-2 " + (m.state === "approved" ? "text-success" : "text-destructive")
                      }
                    >
                      {t(`approvals.stepState.${m.state}`)}
                    </span>
                  )}
                </Td>
                <Td className="font-mono text-[12.5px]">
                  {formatMoment(state === "open" ? m.requested_at : m.decided_at, i18n.language)}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
    </section>
  );
}

export function ApprovalsPage() {
  return <ApprovalList />;
}

export function PortalApprovalsPage() {
  return <ApprovalList portal />;
}
