// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The workflow editor as a table (UI26.1, E26, E30): the states and transitions of a type,
// complete by keyboard alone, every row saved as one change of the customer layer (PL43) like
// the fields in the type editor (UI25). Own states and transitions (k_…) come and go freely;
// states of the package change in label and initial, transitions of the package in label,
// from, to and condition, and are disabled instead of removed. The condition is checked while
// one types (POST …:check-condition); the page points out states no transition reaches and
// transitions that are disabled. Above the tables stands the graph (UI26.2, E31): a click
// on an edge marks the transition's row, a dragged connection prefills the new transition.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { ArrowLeft, SlidersHorizontal } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  meQuery,
  qualifiedName,
  typesQuery,
  type Label as Text,
  type State,
  type Transition,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { LinkButton } from "@/ui/page-header";

import { Changes, useCustomizer, type Apply } from "./CustomizePage";
import { WorkflowGraph } from "./WorkflowGraph";

const prefix = "k_";
const own = (name: string) => name.startsWith(prefix);
const workflowOps = new Set([
  "add_state",
  "change_state",
  "remove_state",
  "add_transition",
  "change_transition",
  "remove_transition",
  "disable_transition",
  "enable_transition",
  "set_layout",
]);

// slug makes the technical name of a new state or transition from its label.
function slug(label: string): string {
  const base = label
    .toLowerCase()
    .replace(/ä/g, "ae")
    .replace(/ö/g, "oe")
    .replace(/ü/g, "ue")
    .replace(/ß/g, "ss")
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_|_$/g, "");
  return (prefix + base).slice(0, 63);
}

const nameOk = (name: string) => /^k_[a-z][a-z0-9_]{0,60}$/.test(name);

// useConditionCheck asks the server whether a condition compiles, a moment after the last
// keystroke; "" means fine, a text is the reason.
function useConditionCheck(typeName: string, when: string): { checking: boolean; error: string } {
  const [result, setResult] = useState<{ when: string; error: string }>({ when: "", error: "" });
  const check = useMutation({
    mutationFn: (w: string) => api.checkCondition(typeName, w),
    onSuccess: (r, w) => setResult({ when: w, error: r.ok ? "" : (r.error ?? "?") }),
  });
  const { mutate } = check;
  useEffect(() => {
    if (when.trim() === "") {
      setResult({ when, error: "" });
      return;
    }
    const timer = setTimeout(() => mutate(when), 300);
    return () => clearTimeout(timer);
  }, [when, mutate]);
  const pending = when.trim() !== "" && result.when !== when;
  return { checking: pending, error: pending ? "" : result.error };
}

export function WorkflowPage() {
  const { t, i18n } = useTranslation();
  const { typeName } = useParams({ from: "/shell/types/$typeName/workflow" });
  const types = useQuery(typesQuery);
  const type = types.data?.find((c) => qualifiedName(c) === typeName);
  const ed = useCustomizer(typeName);
  const me = useQuery(meQuery);
  // selected is the transition marked in graph and table; connect the last connection
  // dragged in the graph, counted so that the same pair prefills the form again.
  const [selected, setSelected] = useState<string | null>(null);
  const [connect, setConnect] = useState<{ from: string; to: string; n: number } | null>(null);
  if (types.isPending || ed.layer.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  if (ed.layer.isError) {
    return <Alert variant="destructive">{ed.problemText(ed.layer.error)}</Alert>;
  }
  const typeLabel = labelOf(type.label, i18n.language, typeName);
  return (
    <div className="flex max-w-5xl flex-col gap-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("workflow.title", { type: typeLabel })}</h1>
        <div className="flex flex-wrap gap-2">
          <LinkButton
            to="/types/$typeName/customize"
            params={{ typeName }}
            icon={<SlidersHorizontal aria-hidden="true" className="size-4 text-foreground-2" />}
          >
            {t("workflow.toFields")}
          </LinkButton>
          <LinkButton
            to="/types/$typeName"
            params={{ typeName }}
            icon={<ArrowLeft aria-hidden="true" className="size-4 text-foreground-2" />}
          >
            {t("customize.toList")}
          </LinkButton>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{t("workflow.intro")}</p>
      {ed.failure && <Alert variant="destructive">{ed.failure}</Alert>}
      {ed.done && <Alert variant="success">{ed.done}</Alert>}
      <WorkflowGraph
        type={type}
        selected={selected}
        onSelect={setSelected}
        onConnect={(from, to) => setConnect((c) => ({ from, to, n: (c?.n ?? 0) + 1 }))}
        busy={ed.busy}
        apply={ed.apply}
        storageKey={`itsm.workflow.graph.${me.data?.id ?? "anon"}`}
      />
      <Hints type={type} />
      <States type={type} busy={ed.busy} apply={ed.apply} />
      <Transitions
        type={type}
        typeName={typeName}
        busy={ed.busy}
        apply={ed.apply}
        selected={selected}
        onSelect={setSelected}
        connect={connect}
      />
      <Changes
        type={type}
        layer={ed.layer.data}
        busy={ed.busy}
        only={(c) => workflowOps.has(c.op)}
        onTakeBack={ed.takeBack}
      />
    </div>
  );
}

// Hints are what the editor sees without the server: states no transition reaches, and
// transitions that are disabled.
function Hints({ type }: { type: TypeDefinition }) {
  const { t, i18n } = useTranslation();
  const states = type.states ?? [];
  const transitions = type.transitions ?? [];
  const reached = new Set(transitions.filter((tr) => !tr.disabled).map((tr) => tr.to));
  const unreachable = states.filter((s) => !s.initial && !reached.has(s.name));
  const disabled = transitions.filter((tr) => tr.disabled);
  if (states.length === 0) {
    return <Alert>{t("workflow.noStates")}</Alert>;
  }
  if (unreachable.length === 0 && disabled.length === 0) {
    return null;
  }
  return (
    <Alert>
      <ul className="list-disc pl-5">
        {unreachable.map((s) => (
          <li key={s.name}>
            {t("workflow.hint.unreachable", { name: labelOf(s.label, i18n.language, s.name) })}
          </li>
        ))}
        {disabled.map((tr) => (
          <li key={tr.name}>
            {t("workflow.hint.disabled", { name: labelOf(tr.label, i18n.language, tr.name) })}
          </li>
        ))}
      </ul>
    </Alert>
  );
}

function States({ type, busy, apply }: { type: TypeDefinition; busy: boolean; apply: Apply }) {
  const { t } = useTranslation();
  const id = useId();
  const states = type.states ?? [];
  const [editing, setEditing] = useState<string | null>(null);
  const [labelDe, setLabelDe] = useState("");
  const [labelEn, setLabelEn] = useState("");
  const [technical, setTechnical] = useState<string | null>(null);
  const [initial, setInitial] = useState(false);
  const name = technical ?? slug(labelDe);
  const ready = labelDe.trim() !== "" && nameOk(name);
  const addState = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) {
      return;
    }
    const label: Text = { de: labelDe.trim() };
    if (labelEn.trim()) {
      label["en"] = labelEn.trim();
    }
    const state: Record<string, unknown> = { name, label };
    if (initial) {
      state["initial"] = true;
    }
    apply(
      { op: "add_state", field: name, state },
      t("workflow.done.stateAdded", { name: labelDe.trim() }),
    );
    setLabelDe("");
    setLabelEn("");
    setTechnical(null);
    setInitial(false);
  };
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("workflow.states")}
      </h2>
      {states.length > 0 && (
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b">
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.label")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.name")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.initial")}
              </th>
              <th scope="col" className="py-2">
                <span className="sr-only">{t("customize.col.action")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {states.map((s) => (
              <StateRow
                key={s.name}
                state={s}
                busy={busy}
                apply={apply}
                editing={editing === s.name}
                onEdit={(open) => setEditing(open ? s.name : null)}
              />
            ))}
          </tbody>
        </table>
      )}
      <form
        className="flex flex-col gap-3 rounded-md border p-3"
        aria-labelledby={`${id}-new`}
        onSubmit={addState}
      >
        <h3 id={`${id}-new`} className="font-medium">
          {t("workflow.newState")}
        </h3>
        <div className="grid gap-3 sm:grid-cols-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-de`}>{t("customize.labelDe")}</Label>
            <Input id={`${id}-de`} value={labelDe} onChange={(e) => setLabelDe(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-en`}>{t("customize.labelEn")}</Label>
            <Input id={`${id}-en`} value={labelEn} onChange={(e) => setLabelEn(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${id}-name`}>{t("customize.technical")}</Label>
            <Input
              id={`${id}-name`}
              value={name}
              aria-invalid={labelDe !== "" && !nameOk(name)}
              onChange={(e) => setTechnical(e.target.value)}
            />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={initial} onChange={(e) => setInitial(e.target.checked)} />
          {t("workflow.makeInitial")}
        </label>
        <div>
          <Button type="submit" disabled={busy || !ready}>
            {t("workflow.addState")}
          </Button>
        </div>
      </form>
    </section>
  );
}

function StateRow({
  state,
  busy,
  apply,
  editing,
  onEdit,
}: {
  state: State;
  busy: boolean;
  apply: Apply;
  editing: boolean;
  onEdit: (open: boolean) => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const name = labelOf(state.label, i18n.language, state.name);
  const [labelDe, setLabelDe] = useState(state.label?.["de"] ?? "");
  const [labelEn, setLabelEn] = useState(state.label?.["en"] ?? "");
  const save = (e: FormEvent) => {
    e.preventDefault();
    const label: Text = {};
    if (labelDe.trim() && labelDe !== (state.label?.["de"] ?? "")) {
      label["de"] = labelDe.trim();
    }
    if (labelEn.trim() && labelEn !== (state.label?.["en"] ?? "")) {
      label["en"] = labelEn.trim();
    }
    if (Object.keys(label).length === 0) {
      onEdit(false);
      return;
    }
    apply(
      { op: "change_state", field: state.name, set: { label } },
      t("workflow.done.stateChanged", { name: label["de"] ?? name }),
    );
    onEdit(false);
  };
  return (
    <>
      <tr className="border-b align-top">
        <td className="py-2 pr-3 font-medium">{name}</td>
        <td className="py-2 pr-3 text-muted-foreground">
          {state.name} · {own(state.name) ? t("customize.flag.own") : t("customize.flag.package")}
        </td>
        <td className="py-2 pr-3">
          <input
            type="radio"
            name="initial"
            checked={state.initial === true}
            disabled={busy}
            aria-label={t("workflow.initialOf", { name })}
            onChange={() =>
              apply(
                { op: "change_state", field: state.name, set: { initial: true } },
                t("workflow.done.initial", { name }),
              )
            }
          />
        </td>
        <td className="py-2">
          <div className="flex flex-wrap gap-1">
            <Button
              variant="outline"
              size="sm"
              aria-expanded={editing}
              onClick={() => onEdit(!editing)}
              aria-label={t("customize.edit", { name })}
            >
              {t("customize.editShort")}
            </Button>
            {own(state.name) && (
              <Button
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() =>
                  apply(
                    { op: "remove_state", field: state.name },
                    t("workflow.done.stateRemoved", { name }),
                  )
                }
                aria-label={t("customize.remove", { name })}
              >
                {t("customize.removeShort")}
              </Button>
            )}
          </div>
        </td>
      </tr>
      {editing && (
        <tr className="border-b">
          <td colSpan={4} className="py-2">
            <form
              className="grid gap-3 sm:grid-cols-3"
              aria-label={t("customize.edit", { name })}
              onSubmit={save}
            >
              <div className="flex flex-col gap-1">
                <Label htmlFor={`${id}-de`}>{t("customize.labelDe")}</Label>
                <Input
                  id={`${id}-de`}
                  value={labelDe}
                  onChange={(e) => setLabelDe(e.target.value)}
                />
              </div>
              <div className="flex flex-col gap-1">
                <Label htmlFor={`${id}-en`}>{t("customize.labelEn")}</Label>
                <Input
                  id={`${id}-en`}
                  value={labelEn}
                  onChange={(e) => setLabelEn(e.target.value)}
                />
              </div>
              <div className="flex items-end gap-2">
                <Button type="submit" disabled={busy}>
                  {t("customize.save")}
                </Button>
                <Button variant="outline" onClick={() => onEdit(false)}>
                  {t("form.cancel")}
                </Button>
              </div>
            </form>
          </td>
        </tr>
      )}
    </>
  );
}

// TransitionFields are the inputs a transition is edited with, for a new one and for a row.
function TransitionFields({
  id,
  states,
  labelDe,
  labelEn,
  from,
  to,
  when,
  onChange,
  check,
  labelRef,
}: {
  id: string;
  states: State[];
  labelDe: string;
  labelEn: string;
  from: string[];
  to: string;
  when: string;
  onChange: (next: {
    labelDe?: string;
    labelEn?: string;
    from?: string[];
    to?: string;
    when?: string;
  }) => void;
  // check is the verdict of the server on the condition, asked by the parent.
  check: { checking: boolean; error: string };
  labelRef?: RefObject<HTMLInputElement | null>;
}) {
  const { t, i18n } = useTranslation();
  const stateName = (s: State) => labelOf(s.label, i18n.language, s.name);
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-de`}>{t("customize.labelDe")}</Label>
        <Input
          id={`${id}-de`}
          ref={labelRef}
          value={labelDe}
          onChange={(e) => onChange({ labelDe: e.target.value })}
        />
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-en`}>{t("customize.labelEn")}</Label>
        <Input
          id={`${id}-en`}
          value={labelEn}
          onChange={(e) => onChange({ labelEn: e.target.value })}
        />
      </div>
      <fieldset className="flex flex-col gap-1">
        <legend className="text-sm font-medium">{t("workflow.col.from")}</legend>
        <div className="flex flex-wrap gap-3">
          {states.map((s) => (
            <label key={s.name} className="flex items-center gap-1 text-sm">
              <input
                type="checkbox"
                checked={from.includes(s.name)}
                onChange={(e) =>
                  onChange({
                    from: e.target.checked ? [...from, s.name] : from.filter((x) => x !== s.name),
                  })
                }
              />
              {stateName(s)}
            </label>
          ))}
        </div>
      </fieldset>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`${id}-to`}>{t("workflow.col.to")}</Label>
        <select
          id={`${id}-to`}
          className="h-10 rounded-md border border-input bg-background px-3 text-sm"
          value={to}
          onChange={(e) => onChange({ to: e.target.value })}
        >
          <option value="">{t("builder.choose")}</option>
          {states.map((s) => (
            <option key={s.name} value={s.name}>
              {stateName(s)}
            </option>
          ))}
        </select>
      </div>
      <div className="flex flex-col gap-1 sm:col-span-2">
        <Label htmlFor={`${id}-when`}>{t("workflow.col.when")}</Label>
        <Input
          id={`${id}-when`}
          value={when}
          aria-invalid={check.error !== ""}
          aria-describedby={`${id}-when-help`}
          onChange={(e) => onChange({ when: e.target.value })}
        />
        <p
          id={`${id}-when-help`}
          className={check.error ? "text-xs text-destructive" : "text-xs text-muted-foreground"}
        >
          {check.error
            ? t("workflow.conditionError", { detail: check.error })
            : check.checking
              ? t("workflow.conditionChecking")
              : when.trim()
                ? t("workflow.conditionOk")
                : t("workflow.conditionHelp")}
        </p>
      </div>
    </div>
  );
}

type Draft = { labelDe: string; labelEn: string; from: string[]; to: string; when: string };

function Transitions({
  type,
  typeName,
  busy,
  apply,
  selected,
  onSelect,
  connect,
}: {
  type: TypeDefinition;
  typeName: string;
  busy: boolean;
  apply: Apply;
  selected: string | null;
  onSelect: (name: string | null) => void;
  connect: { from: string; to: string; n: number } | null;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const states = type.states ?? [];
  const transitions = type.transitions ?? [];
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState<Draft>({
    labelDe: "",
    labelEn: "",
    from: [],
    to: "",
    when: "",
  });
  const [technical, setTechnical] = useState<string | null>(null);
  const condition = useConditionCheck(typeName, draft.when);
  const labelRef = useRef<HTMLInputElement>(null);
  const formRef = useRef<HTMLFormElement>(null);
  // dragged is the connection the form was prefilled from, shown until the transition is made.
  const [dragged, setDragged] = useState<{ from: string; to: string } | null>(null);
  useEffect(() => {
    if (!connect) {
      return;
    }
    setDraft((d) => ({ ...d, from: [connect.from], to: connect.to }));
    setDragged({ from: connect.from, to: connect.to });
    formRef.current?.scrollIntoView?.({ block: "nearest" });
    labelRef.current?.focus();
  }, [connect]);
  const name = technical ?? slug(draft.labelDe);
  const ready =
    draft.labelDe.trim() !== "" &&
    nameOk(name) &&
    draft.from.length > 0 &&
    draft.to !== "" &&
    condition.error === "" &&
    !condition.checking;
  const stateName = (n: string) => {
    const s = states.find((x) => x.name === n);
    return s ? labelOf(s.label, i18n.language, s.name) : n;
  };
  const addTransition = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) {
      return;
    }
    const label: Text = { de: draft.labelDe.trim() };
    if (draft.labelEn.trim()) {
      label["en"] = draft.labelEn.trim();
    }
    const transition: Record<string, unknown> = { name, label, from: draft.from, to: draft.to };
    if (draft.when.trim()) {
      transition["when"] = draft.when.trim();
    }
    apply(
      { op: "add_transition", field: name, transition },
      t("workflow.done.transitionAdded", { name: draft.labelDe.trim() }),
    );
    setDraft({ labelDe: "", labelEn: "", from: [], to: "", when: "" });
    setTechnical(null);
    setDragged(null);
  };
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("workflow.transitions")}
      </h2>
      {transitions.length > 0 && (
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b">
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.label")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.from")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.to")}
              </th>
              <th scope="col" className="py-2 pr-3">
                {t("workflow.col.when")}
              </th>
              <th scope="col" className="py-2">
                <span className="sr-only">{t("customize.col.action")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {transitions.map((tr) => (
              <TransitionRow
                key={tr.name}
                transition={tr}
                typeName={typeName}
                states={states}
                stateName={stateName}
                busy={busy}
                apply={apply}
                editing={editing === tr.name}
                onEdit={(open) => setEditing(open ? tr.name : null)}
                selected={selected === tr.name}
                onSelect={() => onSelect(selected === tr.name ? null : tr.name)}
              />
            ))}
          </tbody>
        </table>
      )}
      {states.length > 0 && (
        <form
          ref={formRef}
          className="flex flex-col gap-3 rounded-md border p-3"
          aria-labelledby={`${id}-new`}
          onSubmit={addTransition}
        >
          <h3 id={`${id}-new`} className="font-medium">
            {t("workflow.newTransition")}
          </h3>
          {dragged && (
            <p className="text-sm text-muted-foreground" role="status">
              {t("workflow.graph.connected", {
                from: stateName(dragged.from),
                to: stateName(dragged.to),
              })}
            </p>
          )}
          <TransitionFields
            id={`${id}-n`}
            states={states}
            {...draft}
            onChange={(next) => setDraft({ ...draft, ...next })}
            check={condition}
            labelRef={labelRef}
          />
          <div className="flex flex-col gap-1 sm:w-1/2">
            <Label htmlFor={`${id}-name`}>{t("customize.technical")}</Label>
            <Input
              id={`${id}-name`}
              value={name}
              aria-invalid={draft.labelDe !== "" && !nameOk(name)}
              onChange={(e) => setTechnical(e.target.value)}
            />
          </div>
          <div>
            <Button type="submit" disabled={busy || !ready}>
              {t("workflow.addTransition")}
            </Button>
          </div>
        </form>
      )}
    </section>
  );
}

function TransitionRow({
  transition: tr,
  typeName,
  states,
  stateName,
  busy,
  apply,
  editing,
  onEdit,
  selected,
  onSelect,
}: {
  transition: Transition;
  typeName: string;
  states: State[];
  stateName: (n: string) => string;
  busy: boolean;
  apply: Apply;
  editing: boolean;
  onEdit: (open: boolean) => void;
  // selected marks the row as the transition picked in the graph (UI26.2).
  selected: boolean;
  onSelect: () => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const name = labelOf(tr.label, i18n.language, tr.name);
  const [draft, setDraft] = useState<Draft>({
    labelDe: tr.label?.["de"] ?? "",
    labelEn: tr.label?.["en"] ?? "",
    from: tr.from,
    to: tr.to,
    when: tr.when ?? "",
  });
  const condition = useConditionCheck(typeName, draft.when);
  const save = (e: FormEvent) => {
    e.preventDefault();
    if (condition.error || condition.checking || draft.from.length === 0 || draft.to === "") {
      return;
    }
    const set: Record<string, unknown> = {};
    const label: Text = {};
    if (draft.labelDe.trim() && draft.labelDe !== (tr.label?.["de"] ?? "")) {
      label["de"] = draft.labelDe.trim();
    }
    if (draft.labelEn.trim() && draft.labelEn !== (tr.label?.["en"] ?? "")) {
      label["en"] = draft.labelEn.trim();
    }
    if (Object.keys(label).length > 0) {
      set["label"] = label;
    }
    if (draft.from.join(",") !== tr.from.join(",")) {
      set["from"] = draft.from;
    }
    if (draft.to !== tr.to) {
      set["to"] = draft.to;
    }
    if (draft.when.trim() !== (tr.when ?? "")) {
      set["when"] = draft.when.trim();
    }
    if (Object.keys(set).length > 0) {
      apply(
        { op: "change_transition", field: tr.name, set },
        t("workflow.done.transitionChanged", { name: label["de"] ?? name }),
      );
    }
    onEdit(false);
  };
  return (
    <>
      <tr
        className={[
          "border-b align-top",
          tr.disabled ? "text-muted-foreground" : "",
          selected ? "bg-primary/10" : "",
        ].join(" ")}
        aria-selected={selected}
        onClick={onSelect}
      >
        <td className="py-2 pr-3">
          <span className="font-medium">{name}</span>
          <div className="text-xs text-muted-foreground">
            {tr.name} · {own(tr.name) ? t("customize.flag.own") : t("customize.flag.package")}
            {tr.disabled ? ` · ${t("workflow.flag.disabled")}` : ""}
            {selected ? ` · ${t("workflow.graph.marked")}` : ""}
          </div>
        </td>
        <td className="py-2 pr-3">{tr.from.map(stateName).join(", ")}</td>
        <td className="py-2 pr-3">{stateName(tr.to)}</td>
        <td className="py-2 pr-3 font-mono text-xs">{tr.when ?? ""}</td>
        <td className="py-2">
          <div className="flex flex-wrap gap-1">
            <Button
              variant="outline"
              size="sm"
              aria-expanded={editing}
              onClick={() => onEdit(!editing)}
              aria-label={t("customize.edit", { name })}
            >
              {t("customize.editShort")}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() =>
                apply(
                  { op: tr.disabled ? "enable_transition" : "disable_transition", field: tr.name },
                  t(tr.disabled ? "workflow.done.enabled" : "workflow.done.disabled", { name }),
                )
              }
              aria-label={t(tr.disabled ? "workflow.enable" : "workflow.disable", { name })}
            >
              {t(tr.disabled ? "workflow.enableShort" : "workflow.disableShort")}
            </Button>
            {own(tr.name) && (
              <Button
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() =>
                  apply(
                    { op: "remove_transition", field: tr.name },
                    t("workflow.done.transitionRemoved", { name }),
                  )
                }
                aria-label={t("customize.remove", { name })}
              >
                {t("customize.removeShort")}
              </Button>
            )}
          </div>
        </td>
      </tr>
      {editing && (
        <tr className="border-b">
          <td colSpan={5} className="py-2">
            <form
              className="flex flex-col gap-3"
              aria-label={t("customize.edit", { name })}
              onSubmit={save}
            >
              <TransitionFields
                id={id}
                states={states}
                {...draft}
                onChange={(next) => setDraft({ ...draft, ...next })}
                check={condition}
              />
              <div className="flex gap-2">
                <Button
                  type="submit"
                  disabled={
                    busy ||
                    condition.error !== "" ||
                    condition.checking ||
                    draft.from.length === 0 ||
                    draft.to === ""
                  }
                >
                  {t("customize.save")}
                </Button>
                <Button variant="outline" onClick={() => onEdit(false)}>
                  {t("form.cancel")}
                </Button>
              </div>
            </form>
          </td>
        </tr>
      )}
    </>
  );
}
