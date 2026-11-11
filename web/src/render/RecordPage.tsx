// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Pages around the Form: create (/types/<type>/new) and the record page (/types/<type>/<id>,
// UI3): a head with title, version, state and transitions; the fields with the timeline below
// them; a sidebar with roles, SLA clocks and related records. Saving stays on the page.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  fieldStatesQuery,
  labelOf,
  meQuery,
  mySettingsQuery,
  qualifiedName,
  recordQuery,
  typesQuery,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { useLive } from "@/app/live";
import { remember } from "@/app/recent";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";

import { approvalsKey } from "./Approvals";
import { fieldLabel } from "./fields";
import { Form } from "./Form";
import { PresenceLine, usePresence } from "./Presence";
import { titleOf } from "./RefName";
import { Sidebar } from "./Sidebar";
import { RecordTabs } from "./RecordTabs";
import { Transitions } from "./Transitions";

function useType(typeName: string): { type?: TypeDefinition; pending: boolean } {
  const types = useQuery(typesQuery);
  return {
    type: types.data?.find((c) => qualifiedName(c) === typeName),
    pending: types.isPending,
  };
}

function Head({
  type,
  typeName,
  title,
}: {
  type: TypeDefinition;
  typeName: string;
  title: string;
}) {
  const { i18n } = useTranslation();
  return (
    <nav aria-label="breadcrumb" className="mb-4 text-sm text-muted-foreground">
      <Link to="/types/$typeName" params={{ typeName }} className="underline">
        {labelOf(type.label, i18n.language, typeName)}
      </Link>
      <span aria-hidden="true"> / </span>
      <span>{title}</span>
    </nav>
  );
}

export function NewRecordPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { typeName } = useParams({ from: "/shell/types/$typeName/new" });
  const { type, pending } = useType(typeName);
  const states = useQuery(fieldStatesQuery(typeName));
  const settings = useQuery(mySettingsQuery);
  // The form takes its presets once, when it first draws: wait for the settings (UI9).
  if (pending || states.isPending || settings.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  const back = () => void navigate({ to: "/types/$typeName", params: { typeName } });
  return (
    <>
      <Head type={type} typeName={typeName} title={t("form.newTitle")} />
      <h1 className="mb-4 text-2xl font-semibold">{t("form.newTitle")}</h1>
      <Form
        type={type}
        states={states.data}
        presets={settings.data?.defaults[typeName]}
        onSaved={(saved: RecordData) => {
          void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
          void navigate({
            to: "/types/$typeName/$id",
            params: { typeName, id: String(saved["id"]) },
          });
        }}
        onCancel={back}
      />
    </>
  );
}

export function RecordPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { typeName, id } = useParams({ from: "/shell/types/$typeName/$id" });
  const { type, pending } = useType(typeName);
  const record = useQuery(recordQuery(typeName, id));
  const states = useQuery(fieldStatesQuery(typeName, id));
  // savedAt is the version the last save on this page produced, for the confirmation.
  const [savedAt, setSavedAt] = useState<number | null>(null);
  // Live (UI8): unsaved changes in the form, who else is on the record, and who changed it
  // while the form held changes.
  const [dirty, setDirty] = useState(false);
  const [staleBy, setStaleBy] = useState<string | null>(null);
  // Values a tab puts into the form, unsaved (SD18: "Als Lösung übernehmen").
  const [applied, setApplied] = useState<{ n: number; values: Record<string, unknown> }>();
  // A change that waits for approval (SD17, picture 1 and 2A of the mockup "Freigabe von
  // Wissen"): a band above the form, its fields read-only until it is decided.
  const [showTab, setShowTab] = useState<{ n: number; key: string }>();
  const hold = type?.approval?.hold;
  const approvals = useQuery({
    queryKey: approvalsKey(typeName, id),
    queryFn: () => api.approvals(typeName, id),
    enabled: Boolean(hold),
  });
  const waiting = hold ? approvals.data?.find((a) => a.state === "open" && a.changes) : undefined;
  const me = useQuery(meQuery);
  const others = usePresence(typeName, id, dirty);
  useLive((e) => {
    if (e.type_name !== typeName || e.record_id !== id || e.actor === me.data?.id) {
      return;
    }
    // Timeline, clocks, transitions and field states follow at once; the form only when it
    // holds nothing unsaved.
    void queryClient.invalidateQueries({
      queryKey: ["records", typeName, id],
      predicate: (q) => q.queryKey.length > 3,
    });
    if (dirty) {
      setStaleBy(others.find((o) => o.account === e.actor)?.name ?? t("live.someone"));
    } else {
      void queryClient.invalidateQueries({
        queryKey: recordQuery(typeName, id).queryKey,
        exact: true,
      });
    }
  });
  // The command line lists the records opened last (UI31).
  const account = me.data?.id;
  const opened = type && record.data ? titleOf(type, record.data) : undefined;
  useEffect(() => {
    if (account && opened !== undefined) {
      remember(account, { type: typeName, id, title: opened });
    }
  }, [account, typeName, id, opened]);
  if (pending || record.isPending || states.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  if (record.isError || !record.data) {
    return <Alert variant="destructive">{t("form.notFound")}</Alert>;
  }
  const titleField = type.title_field ?? type.fields[0]?.name ?? "id";
  const title = String(record.data[titleField] ?? id);
  const back = () => void navigate({ to: "/types/$typeName", params: { typeName } });
  const version = Number(record.data["version"]);
  const timeline = type.traits?.includes("timeline") || type.traits?.includes("history");
  return (
    <>
      <Head type={type} typeName={typeName} title={title} />
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="mb-1 text-2xl font-semibold">{title}</h1>
          <p className="text-sm text-muted-foreground">{t("form.version", { version })}</p>
          <p role="status" className="text-sm">
            {savedAt === version ? t("form.saved") : ""}
          </p>
          <PresenceLine others={others} />
        </div>
        <Transitions type={type} record={record.data} />
      </header>
      {staleBy && (
        <Alert className="mb-4 flex flex-wrap items-center gap-3">
          <span>{t("live.changedBy", { name: staleBy })}</span>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setStaleBy(null);
              setDirty(false);
              void queryClient.invalidateQueries({
                queryKey: recordQuery(typeName, id).queryKey,
                exact: true,
              });
            }}
          >
            {t("live.reload")}
          </Button>
        </Alert>
      )}
      {waiting && (
        <HeldBand
          type={type}
          fields={Object.keys(waiting.changes ?? {})}
          onShow={() => setShowTab((s) => ({ n: (s?.n ?? 0) + 1, key: "approvals" }))}
          onWithdraw={async () => {
            await api.withdrawApproval(typeName, id, waiting.id);
            void queryClient.invalidateQueries({ queryKey: ["records", typeName, id] });
          }}
        />
      )}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div>
          <Form
            key={`${String(record.data["version"])}-${states.data?.state ?? ""}-${waiting?.id ?? ""}`}
            type={type}
            record={record.data}
            states={
              waiting && states.data
                ? {
                    ...states.data,
                    read_only: [...states.data.read_only, ...Object.keys(waiting.changes ?? {})],
                  }
                : states.data
            }
            onDirtyChange={setDirty}
            apply={applied}
            onSaved={(saved: RecordData) => {
              queryClient.setQueryData(recordQuery(typeName, id).queryKey, saved);
              setSavedAt(Number(saved["version"]));
              setStaleBy(null);
              // Field states, transitions, timeline and clocks follow the new version.
              void queryClient.invalidateQueries({
                queryKey: ["records", typeName, id],
                predicate: (q) => q.queryKey.length > 3,
              });
              void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
            }}
            onCancel={back}
          />
          <RecordTabs
            type={type}
            id={id}
            timeline={timeline === true}
            version={Number(record.data["version"])}
            record={record.data}
            onApply={(values) => setApplied((a) => ({ n: (a?.n ?? 0) + 1, values }))}
            show={showTab}
          />
        </div>
        <Sidebar type={type} record={record.data} />
      </div>
    </>
  );
}

// HeldBand says that a change waits for approval and that readers see the record as it was
// until then (SD17); its fields stay read-only in the form. Withdrawing frees them.
function HeldBand({
  type,
  fields,
  onShow,
  onWithdraw,
}: {
  type: TypeDefinition;
  fields: string[];
  onShow: () => void;
  onWithdraw: () => Promise<void>;
}) {
  const { t, i18n } = useTranslation();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const names = fields.map((n) => {
    const f = type.fields.find((x) => x.name === n);
    return f ? fieldLabel(f, i18n.language) : n;
  });
  return (
    <div
      role="status"
      className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-md border border-warning-border bg-warning-background px-4 py-2.5 text-sm"
    >
      <span>{t("approvals.held", { fields: names.join(", ") })}</span>
      <span className="flex flex-wrap gap-2">
        <Button size="sm" variant="outline" onClick={onShow}>
          {t("approvals.heldShow")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={() => {
            setBusy(true);
            setFailed(false);
            onWithdraw()
              .catch(() => setFailed(true))
              .finally(() => setBusy(false));
          }}
        >
          {t("approvals.heldWithdraw")}
        </Button>
      </span>
      {failed && <span className="w-full text-destructive">{t("approvals.heldFailed")}</span>}
    </div>
  );
}
