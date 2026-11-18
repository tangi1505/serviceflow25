// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Approvals at a record (WF6, WF-Plan E17, picture 1 and 2 of the mockup "Genehmigungen"): the
// tab "Genehmigung" of every type with the trait approvals. Each stage with its least number of
// yes and its questions: who is asked, the decision, who decided (a substitute "for" the
// person), when, the comment. Whoever is asked now answers below; a refusal needs a reason.
// Whoever may change the record asks for approval in stages, or withdraws an open one. The
// portal shows the same without asking and withdrawing. A change that waits for the approval
// (SD17, SD-Plan N22) shows field by field, text word by word as in the versions.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  qualifiedName,
  type ActorRef,
  type Approval,
  type ApprovalDraft,
  type ApprovalStage,
  type ApprovalStep,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { Td, Th, THead, Tr } from "@/ui/table";

import { fieldLabel, formatMoment } from "./fields";
import { RefName } from "./RefName";
import { RefPicker } from "./RefPicker";
import { DensityTable } from "./TableTools";
import { ChangeValue } from "./Timeline";
import { plain, Words } from "./Versions";
import { wordDiff } from "./wordDiff";

export const approvalsKey = (typeName: string, id: string) => [
  "records",
  typeName,
  id,
  "approvals",
];

const detail = (err: unknown) =>
  err instanceof ApiError ? (err.problem?.detail ?? err.message) : String(err);

// stateClass colours a state with a word beside it, never by colour alone.
const stateClass: Record<string, string> = {
  approved: "text-success",
  rejected: "text-destructive",
  open: "text-foreground-2",
  waiting: "text-muted-foreground",
  dropped: "text-muted-foreground",
  withdrawn: "text-muted-foreground",
};

function useRefresh(typeName: string, id: string) {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: ["records", typeName, id] });
    void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
    void queryClient.invalidateQueries({ queryKey: ["approvals"] });
  };
}

export function Approvals({
  type,
  id,
  record,
  manage = true,
}: {
  type: TypeDefinition;
  id: string;
  // record is the record as it is: a change that waits shows against it (SD17).
  record?: RecordData;
  // manage offers asking and withdrawing (the workspace); the portal only answers.
  manage?: boolean;
}) {
  const { t } = useTranslation();
  const typeName = qualifiedName(type);
  const list = useQuery({
    queryKey: approvalsKey(typeName, id),
    queryFn: () => api.approvals(typeName, id),
  });
  const [asking, setAsking] = useState(false);
  const refresh = useRefresh(typeName, id);
  const withdraw = useMutation({
    mutationFn: (approvalId: string) => api.withdrawApproval(typeName, id, approvalId),
    onSuccess: refresh,
  });
  if (list.isPending) {
    return <p className="text-sm text-muted-foreground">{t("approvals.loading")}</p>;
  }
  if (list.isError) {
    return (
      <Alert variant="destructive">{t("approvals.error", { detail: detail(list.error) })}</Alert>
    );
  }
  const approvals = list.data;
  const open = approvals.find((a) => a.state === "open");
  return (
    <section aria-labelledby="approvals-heading" className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id="approvals-heading" className="text-[15px] font-semibold">
          {t("approvals.title")}
        </h2>
        {manage && (
          <div className="flex flex-wrap gap-2">
            {open && (
              <Button
                variant="outline"
                size="sm"
                disabled={withdraw.isPending}
                onClick={() => withdraw.mutate(open.id)}
              >
                {t("approvals.withdraw")}
              </Button>
            )}
            {!open && !asking && (
              <Button variant="outline" size="sm" onClick={() => setAsking(true)}>
                {t("approvals.request")}
              </Button>
            )}
          </div>
        )}
      </div>
      {withdraw.isError && (
        <Alert variant="destructive">
          {t("approvals.error", { detail: detail(withdraw.error) })}
        </Alert>
      )}
      {asking && (
        <RequestForm
          type={type}
          id={id}
          onDone={() => {
            setAsking(false);
            refresh();
          }}
          onCancel={() => setAsking(false)}
        />
      )}
      {approvals.length === 0 && !asking && (
        <p className="text-sm text-muted-foreground">{t("approvals.none")}</p>
      )}
      {approvals.map((a, i) => (
        <ApprovalView
          key={a.id}
          type={type}
          id={id}
          record={record}
          approval={a}
          latest={i === 0}
        />
      ))}
    </section>
  );
}

function ApprovalView({
  type,
  id,
  record,
  approval,
  latest,
}: {
  type: TypeDefinition;
  id: string;
  record?: RecordData;
  approval: Approval;
  latest: boolean;
}) {
  const { t, i18n } = useTranslation();
  const typeName = qualifiedName(type);
  const mine = approval.stages.flatMap((s) => s.steps).filter((s) => s.mine);
  return (
    <article
      className={
        "flex flex-col gap-3 rounded-lg border bg-card p-4 " + (latest ? "" : "opacity-80")
      }
      aria-label={t("approvals.of", { when: formatMoment(approval.requested_at, i18n.language) })}
    >
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <span className={"text-sm font-semibold " + (stateClass[approval.state] ?? "")}>
          {t(`approvals.state.${approval.state}`)}
        </span>
        <span className="text-[13px] text-muted-foreground">
          {t("approvals.asked")}{" "}
          {approval.requester ? (
            <RefName refTo={{ type: "core.person", id: approval.requester }} />
          ) : null}{" "}
          {formatMoment(approval.requested_at, i18n.language)}
          {approval.message ? ` · „${approval.message}“` : ""}
        </span>
      </div>
      {approval.stages.map((stage) => (
        <StageView key={stage.position} stage={stage} count={approval.stages.length} />
      ))}
      {approval.changes && <ChangeView type={type} record={record} changes={approval.changes} />}
      {mine.length > 0 && (
        <DecisionForm typeName={typeName} id={id} approvalId={approval.id} step={mine[0]!} />
      )}
    </article>
  );
}

// ChangeView is the change that waits for an approval (SD17, picture 1 of the mockup "Freigabe
// von Wissen"): each field with what it is and what it would be; a text word by word, a
// translation with its language.
function ChangeView({
  type,
  record,
  changes,
}: {
  type: TypeDefinition;
  record?: RecordData;
  changes: Record<string, unknown>;
}) {
  const { t, i18n } = useTranslation();
  const rows: { key: string; label: string; field: string; from: unknown; to: unknown }[] = [];
  for (const [name, value] of Object.entries(changes)) {
    const f = type.fields.find((x) => x.name === name);
    const label = f ? fieldLabel(f, i18n.language) : name;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      // Values in other languages (SD16c): {"de": …, "en": …}.
      for (const [lang, v] of Object.entries(value as Record<string, unknown>)) {
        const from = lang === "de" ? record?.[name] : undefined;
        rows.push({
          key: `${name}.${lang}`,
          label: `${label} (${lang})`,
          field: name,
          from,
          to: v,
        });
      }
    } else {
      rows.push({ key: name, label, field: name, from: record?.[name], to: value });
    }
  }
  return (
    <section aria-label={t("approvals.changes")} className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold">{t("approvals.changes")}</h3>
      <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm">
        {rows.map((r) => {
          const f = type.fields.find((x) => x.name === r.field);
          const words =
            f?.type === "text"
              ? wordDiff(plain(r.from, f.format === "rich"), plain(r.to, f.format === "rich"))
              : null;
          return (
            <div key={r.key} className="contents">
              <dt className="text-foreground-2">{r.label}</dt>
              <dd className="break-words">
                {words ? (
                  <>
                    <Words segments={words.old.filter((s) => s.kind === "del")} />{" "}
                    <Words segments={words.new} />
                  </>
                ) : (
                  <>
                    <del className="text-foreground-2">
                      <ChangeValue type={type} field={r.field} value={r.from} />
                    </del>{" "}
                    <ins className="bg-surface-active underline decoration-2">
                      <ChangeValue type={type} field={r.field} value={r.to} />
                    </ins>
                  </>
                )}
              </dd>
            </div>
          );
        })}
      </dl>
    </section>
  );
}

function StageView({ stage, count }: { stage: ApprovalStage; count: number }) {
  const { t, i18n } = useTranslation();
  const need = stage.need === 0 ? stage.steps.length : stage.need;
  const yes = stage.steps.filter((s) => s.state === "approved").length;
  return (
    <div className="flex flex-col gap-2 rounded-md border border-border-soft bg-surface-subtle p-3">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h3 className="text-sm font-semibold">
          {t("approvals.stage", { n: stage.position, of: count })}
          {stage.name ? ` · ${stage.name}` : ""}
        </h3>
        <span className="text-[13px] text-muted-foreground">
          {t("approvals.need", { need, of: stage.steps.length })} ·{" "}
          {t("approvals.yes", { count: yes })}
        </span>
        <span className={"text-[13px] " + (stateClass[stage.state] ?? "")}>
          {t(`approvals.stageState.${stage.state}`)}
        </span>
      </div>
      {((stage.reminders ?? 0) > 0 || stage.remind_at) && (
        <p className="text-[13px] text-muted-foreground">
          {(stage.reminders ?? 0) > 0 && stage.reminded_at
            ? t("approvals.reminded", {
                count: stage.reminders,
                last: formatMoment(stage.reminded_at, i18n.language),
              })
            : t("approvals.notReminded")}
          {stage.state === "open" && stage.remind_at
            ? " · " +
              t("approvals.nextReminder", { at: formatMoment(stage.remind_at, i18n.language) })
            : ""}
        </p>
      )}
      <DensityTable>
        <THead>
          <tr>
            <Th>{t("approvals.approver")}</Th>
            <Th>{t("approvals.decision")}</Th>
            <Th>{t("approvals.decidedBy")}</Th>
            <Th>{t("approvals.when")}</Th>
            <Th>{t("approvals.comment")}</Th>
          </tr>
        </THead>
        <tbody>
          {stage.steps.map((s) => (
            <StepRow key={s.id} step={s} language={i18n.language} />
          ))}
        </tbody>
      </DensityTable>
    </div>
  );
}

function StepRow({ step, language }: { step: ApprovalStep; language: string }) {
  const { t } = useTranslation();
  return (
    <Tr className={step.mine ? "bg-accent-soft" : undefined}>
      <Td>
        <RefName refTo={step.approver as ActorRef} />
        {step.mine && (
          <span className="ml-2 text-[12.5px] text-muted-foreground">{t("approvals.you")}</span>
        )}
      </Td>
      <Td className={stateClass[step.state] ?? ""}>{t(`approvals.stepState.${step.state}`)}</Td>
      <Td>
        {step.decided_by && <RefName refTo={{ type: "core.person", id: step.decided_by }} />}
        {step.on_behalf && (
          <>
            {" "}
            {t("approvals.for")} <RefName refTo={{ type: "core.person", id: step.on_behalf }} />
          </>
        )}
      </Td>
      <Td className="font-mono text-[12.5px]">
        {step.decided_at ? formatMoment(step.decided_at, language) : ""}
      </Td>
      <Td>{step.comment ?? ""}</Td>
    </Tr>
  );
}

// DecisionForm answers one open question: agree, or refuse with a reason.
export function DecisionForm({
  typeName,
  id,
  approvalId,
  step,
  onDone,
}: {
  typeName: string;
  id: string;
  approvalId: string;
  step: ApprovalStep;
  onDone?: () => void;
}) {
  const { t } = useTranslation();
  const fieldId = useId();
  const [comment, setComment] = useState("");
  const [missing, setMissing] = useState(false);
  const refresh = useRefresh(typeName, id);
  const decide = useMutation({
    mutationFn: (decision: "approve" | "reject") =>
      api.decideApproval(typeName, id, approvalId, { decision, comment, step: step.id }),
    onSuccess: () => {
      refresh();
      onDone?.();
    },
  });
  return (
    <form
      className="flex flex-col gap-2.5 rounded-md border border-ring bg-card p-3"
      aria-labelledby={`${fieldId}-h`}
      onSubmit={(e) => e.preventDefault()}
    >
      <h3 id={`${fieldId}-h`} className="text-sm font-semibold">
        {t("approvals.yourDecision")} <RefName refTo={step.approver as ActorRef} />
      </h3>
      <div className="flex flex-col gap-1">
        <Label htmlFor={fieldId}>
          {t("approvals.comment")}{" "}
          <span className="text-[12.5px] font-normal text-muted-foreground">
            {t("approvals.commentHint")}
          </span>
        </Label>
        <textarea
          id={fieldId}
          value={comment}
          onChange={(e) => {
            setComment(e.target.value);
            setMissing(false);
          }}
          aria-invalid={missing}
          aria-describedby={missing ? `${fieldId}-err` : undefined}
          className="min-h-16 rounded-md border border-input bg-card px-2.5 py-2 text-sm"
        />
      </div>
      {missing && (
        <p id={`${fieldId}-err`} role="alert" className="text-[12.5px] text-destructive">
          {t("approvals.reasonMissing")}
        </p>
      )}
      {decide.isError && (
        <Alert variant="destructive">
          {t("approvals.error", { detail: detail(decide.error) })}
        </Alert>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="button" disabled={decide.isPending} onClick={() => decide.mutate("approve")}>
          {t("approvals.approve")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={decide.isPending}
          onClick={() => (comment.trim() ? decide.mutate("reject") : setMissing(true))}
        >
          {t("approvals.reject")}
        </Button>
      </div>
    </form>
  );
}

// Draft is a stage being asked; roles names, by approver id, the role of the record an
// approver of a template comes from (WF7).
type Draft = { name: string; need: number; approvers: ActorRef[]; roles?: Record<string, string> };

// roleLabel names a role of a template the way the type does: "Gruppe", or "Leitung von
// Gruppe" for the responsible persons of its groups.
function roleLabel(type: TypeDefinition, spec: string, language: string, lead: string) {
  const responsible = spec.endsWith(".responsible");
  const name = responsible ? spec.slice(0, -".responsible".length) : spec;
  const role = type.actors?.find((a) => a.name === name);
  const label = labelOf(role?.label, language, name);
  return responsible ? lead.replace("{{role}}", label) : label;
}

// RequestForm asks for approval: stages one under the other, each with its name, the least
// number of yes (all by default) and its persons and groups. A template (WF7) fills the text
// and the stages, its roles already resolved; everything may still change.
function RequestForm({
  type,
  id,
  onDone,
  onCancel,
}: {
  type: TypeDefinition;
  id: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t, i18n } = useTranslation();
  const typeName = qualifiedName(type);
  const base = useId();
  const [stages, setStages] = useState<Draft[]>([{ name: "", need: 0, approvers: [] }]);
  const [message, setMessage] = useState("");
  const [template, setTemplate] = useState<ApprovalDraft>();
  const templates = useQuery({
    queryKey: ["records", typeName, id, "approval-templates"],
    queryFn: () => api.approvalTemplates(typeName, id),
  });
  const pick = (tid: string) => {
    const d = templates.data?.find((x) => x.template === tid);
    setTemplate(d);
    setMessage(d?.message ?? "");
    setStages(
      d
        ? d.stages.map((s) => ({
            name: s.name,
            need: s.need,
            approvers: s.approvers.map((a) => ({ type: a.type, id: a.id })),
            roles: Object.fromEntries(
              s.approvers.filter((a) => a.role).map((a) => [a.id, a.role!]),
            ),
          }))
        : [{ name: "", need: 0, approvers: [] }],
    );
  };
  const set = (i: number, patch: Partial<Draft>) =>
    setStages(stages.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const send = useMutation({
    mutationFn: () =>
      api.requestApproval(typeName, id, {
        message,
        template: template?.template,
        stages: stages.map((s) => ({
          name: s.name,
          need: s.need,
          approvers: s.approvers.map((a) => ({
            type: a.type as "core.person" | "core.group",
            id: a.id,
          })),
        })),
      }),
    onSuccess: onDone,
  });
  const ready = stages.every((s) => s.approvers.length > 0);
  return (
    <form
      className="flex flex-col gap-3 rounded-lg border bg-card p-4"
      aria-labelledby={`${base}-h`}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) {
          send.mutate();
        }
      }}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h3 id={`${base}-h`} className="text-[15px] font-semibold">
          {t("approvals.request")}
        </h3>
        <span className="text-[12.5px] text-muted-foreground">{t("approvals.requestHint")}</span>
      </div>
      {(templates.data?.length ?? 0) > 0 && (
        <div className="flex max-w-md flex-col gap-1">
          <Label htmlFor={`${base}-tpl`}>{t("approvals.template")}</Label>
          <select
            id={`${base}-tpl`}
            value={template?.template ?? ""}
            onChange={(e) => pick(e.target.value)}
            aria-describedby={`${base}-tpl-hint`}
            className="h-9 rounded-md border border-input bg-card px-2.5 text-sm"
          >
            <option value="">{t("approvals.noTemplate")}</option>
            {templates.data!.map((d) => (
              <option key={d.template} value={d.template}>
                {d.name}
              </option>
            ))}
          </select>
          <span id={`${base}-tpl-hint`} className="text-[12.5px] text-muted-foreground">
            {t("approvals.templateHint")}
          </span>
        </div>
      )}
      {stages.map((s, i) => (
        <fieldset
          key={i}
          className="flex flex-col gap-3 rounded-md border border-border-soft bg-surface-subtle p-3"
        >
          <legend className="px-1 text-sm font-semibold">
            {t("approvals.stage", { n: i + 1, of: stages.length })}
          </legend>
          <div className="flex flex-wrap items-end gap-3">
            <div className="flex min-w-60 flex-1 flex-col gap-1">
              <Label htmlFor={`${base}-name-${i}`}>{t("approvals.stageName")}</Label>
              <Input
                id={`${base}-name-${i}`}
                value={s.name}
                onChange={(e) => set(i, { name: e.target.value })}
              />
            </div>
            <div className="flex flex-col gap-1">
              <Label htmlFor={`${base}-need-${i}`}>{t("approvals.needLabel")}</Label>
              <select
                id={`${base}-need-${i}`}
                value={s.need}
                onChange={(e) => set(i, { need: Number(e.target.value) })}
                className="h-9 rounded-md border border-input bg-card px-2.5 text-sm"
              >
                <option value={0}>{t("approvals.all")}</option>
                {s.approvers.slice(0, -1).map((_, n) => (
                  <option key={n} value={n + 1}>
                    {t("approvals.atLeast", { count: n + 1 })}
                  </option>
                ))}
              </select>
            </div>
            {stages.length > 1 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setStages(stages.filter((_, j) => j !== i))}
              >
                {t("approvals.removeStage")}
              </Button>
            )}
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${base}-who-${i}`}>{t("approvals.approvers")}</Label>
            <RefPicker
              id={`${base}-who-${i}`}
              targets={["core.person", "core.group"]}
              value={s.approvers}
              onChange={(approvers) =>
                set(i, { approvers, need: s.need >= approvers.length ? 0 : s.need })
              }
            />
            {s.approvers
              .filter((a) => s.roles?.[a.id])
              .map((a) => (
                <span key={a.id} className="text-[12.5px] text-muted-foreground">
                  <RefName refTo={a} />{" "}
                  {t("approvals.fromRole", {
                    role: roleLabel(type, s.roles![a.id]!, i18n.language, t("approvals.roleLead")),
                  })}
                </span>
              ))}
          </div>
        </fieldset>
      ))}
      <Button
        variant="outline"
        size="sm"
        className="self-start"
        onClick={() => setStages([...stages, { name: "", need: 0, approvers: [] }])}
      >
        {t("approvals.addStage")}
      </Button>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${base}-msg`}>{t("approvals.message")}</Label>
        <textarea
          id={`${base}-msg`}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          className="min-h-16 rounded-md border border-input bg-card px-2.5 py-2 text-sm"
        />
      </div>
      <p className="text-[12.5px] text-muted-foreground">
        {template?.remind_days === 0
          ? t("approvals.remindNone")
          : template?.remind_days
            ? t("approvals.remindTemplate", { count: template.remind_days })
            : t("approvals.remindSetting")}
      </p>
      {send.isError && (
        <Alert variant="destructive">{t("approvals.error", { detail: detail(send.error) })}</Alert>
      )}
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="outline" onClick={onCancel}>
          {t("approvals.cancel")}
        </Button>
        <Button type="submit" disabled={!ready || send.isPending}>
          {t("approvals.send")}
        </Button>
      </div>
    </form>
  );
}
