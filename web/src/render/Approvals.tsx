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
// portal shows the same without asking and withdrawing.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  qualifiedName,
  type ActorRef,
  type Approval,
  type ApprovalStage,
  type ApprovalStep,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import { formatMoment } from "./fields";
import { RefName } from "./RefName";
import { RefPicker } from "./RefPicker";

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
  manage = true,
}: {
  type: TypeDefinition;
  id: string;
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
          typeName={typeName}
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
        <ApprovalView key={a.id} typeName={typeName} id={id} approval={a} latest={i === 0} />
      ))}
    </section>
  );
}

function ApprovalView({
  typeName,
  id,
  approval,
  latest,
}: {
  typeName: string;
  id: string;
  approval: Approval;
  latest: boolean;
}) {
  const { t, i18n } = useTranslation();
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
      {mine.length > 0 && (
        <DecisionForm typeName={typeName} id={id} approvalId={approval.id} step={mine[0]!} />
      )}
    </article>
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
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[11.5px] uppercase tracking-wide text-muted-foreground">
              <th className="px-2 py-1 font-medium">{t("approvals.approver")}</th>
              <th className="px-2 py-1 font-medium">{t("approvals.decision")}</th>
              <th className="px-2 py-1 font-medium">{t("approvals.decidedBy")}</th>
              <th className="px-2 py-1 font-medium">{t("approvals.when")}</th>
              <th className="px-2 py-1 font-medium">{t("approvals.comment")}</th>
            </tr>
          </thead>
          <tbody>
            {stage.steps.map((s) => (
              <StepRow key={s.id} step={s} language={i18n.language} />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function StepRow({ step, language }: { step: ApprovalStep; language: string }) {
  const { t } = useTranslation();
  return (
    <tr className={"border-t border-border-soft " + (step.mine ? "bg-accent-soft" : "")}>
      <td className="px-2 py-1.5">
        <RefName refTo={step.approver as ActorRef} />
        {step.mine && (
          <span className="ml-2 text-[12.5px] text-muted-foreground">{t("approvals.you")}</span>
        )}
      </td>
      <td className={"px-2 py-1.5 " + (stateClass[step.state] ?? "")}>
        {t(`approvals.stepState.${step.state}`)}
      </td>
      <td className="px-2 py-1.5">
        {step.decided_by && <RefName refTo={{ type: "core.person", id: step.decided_by }} />}
        {step.on_behalf && (
          <>
            {" "}
            {t("approvals.for")} <RefName refTo={{ type: "core.person", id: step.on_behalf }} />
          </>
        )}
      </td>
      <td className="px-2 py-1.5 font-mono text-[12.5px]">
        {step.decided_at ? formatMoment(step.decided_at, language) : ""}
      </td>
      <td className="px-2 py-1.5">{step.comment ?? ""}</td>
    </tr>
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

type Draft = { name: string; need: number; approvers: ActorRef[] };

// RequestForm asks for approval: stages one under the other, each with its name, the least
// number of yes (all by default) and its persons and groups.
function RequestForm({
  typeName,
  id,
  onDone,
  onCancel,
}: {
  typeName: string;
  id: string;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const base = useId();
  const [stages, setStages] = useState<Draft[]>([{ name: "", need: 0, approvers: [] }]);
  const [message, setMessage] = useState("");
  const set = (i: number, patch: Partial<Draft>) =>
    setStages(stages.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const send = useMutation({
    mutationFn: () =>
      api.requestApproval(typeName, id, {
        message,
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
