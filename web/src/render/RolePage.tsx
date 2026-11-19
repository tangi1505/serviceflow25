// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The role editor (UI27, UI-Plan E33): the roles and, per role, its permissions as a matrix of
// types and the actions read, create, update, delete and purge, with the scope in every cell; the
// condition, the field rights and further scopes of a cell in its details below; the special
// rights as boxes; every other permission in a list; and who holds the role at which node.
// Every change is one call that holds at once and stands in the security log (PL27); the
// permissions are those of the API (PL8, PL9): a line is a type, an action and a scope.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { ArrowLeft, Plus } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  assignmentsQuery,
  labelOf,
  orgNodesQuery,
  qualifiedName,
  rolesQuery,
  typesQuery,
  type ActorRef,
  type FieldRights,
  type Label as Text,
  type Permission,
  type Role,
  type Scope,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { primaryBarButton } from "@/ui/menu";
import { LinkButton, PageHeader } from "@/ui/page-header";
import { Table, Td, Th, THead, Tr } from "@/ui/table";

import { useProblemText } from "./CustomizePage";
import { fieldLabel } from "./fields";
import { RefName } from "./RefName";
import { RefPicker } from "./RefPicker";
import { DensityTable, matches, TableSearch, useDensity } from "./TableTools";
import { Select } from "@/ui/select";

const any = "*";
// purge: delete for good from the trash, its own right since PL75.
const actions = ["read", "create", "update", "delete", "purge"] as const;
const scopes: Scope[] = ["own", "group", "involved", "unit", "subtree", "all"];

// specials are the rights the code knows beyond the five actions, on the type it checks them
// on. A box sets the right with scope all.
const specials: { type: string; action: string }[] = [
  { type: "core.ui", action: "workplace" },
  { type: "core.type", action: "customize" },
  { type: "core.role", action: "manage_rights" },
  { type: "core.org", action: "manage_org" },
  { type: "core.org", action: "manage_settings" },
  { type: "core.rule", action: "manage_rules" },
  { type: "core.webhook", action: "manage_webhooks" },
  { type: "core.job", action: "manage_jobs" },
  { type: "core.security_event", action: "read" },
  { type: "core.tenant", action: "report_tenants" },
];
const isSpecial = (p: { type_name: string; action: string }) =>
  specials.some((s) => s.type === p.type_name && s.action === p.action);
const inMatrix = (p: Permission) =>
  !isSpecial(p) && (actions as readonly string[]).includes(p.action);

// useRun runs one change at a time with its outcome as a message, and reloads roles and
// assignments afterwards.
function useRun() {
  const queryClient = useQueryClient();
  const problemText = useProblemText();
  const [failure, setFailure] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);
  const m = useMutation({
    mutationFn: async ({ steps }: { steps: (() => Promise<unknown>)[]; message: string }) => {
      for (const step of steps) {
        await step();
      }
    },
    onMutate: () => {
      setFailure(null);
      setDone(null);
    },
    onSuccess: (_, { message }) => setDone(message),
    onError: (err) => setFailure(problemText(err)),
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ["roles"] });
      await queryClient.invalidateQueries({ queryKey: ["assignments"] });
    },
  });
  return {
    run: (message: string, ...steps: (() => Promise<unknown>)[]) => m.mutate({ steps, message }),
    busy: m.isPending,
    failure,
    done,
  };
}
type Run = ReturnType<typeof useRun>["run"];

function useLabels() {
  const { t, i18n } = useTranslation();
  const types = useQuery(typesQuery);
  const typeLabel = (name: string) => {
    if (name === any) {
      return t("roles.anyType");
    }
    const ty = types.data?.find((x) => qualifiedName(x) === name);
    return ty ? labelOf(ty.label, i18n.language, name) : name;
  };
  const actionLabel = (a: string) =>
    a === any ? t("roles.anyAction") : t(`roles.action.${a}`, { defaultValue: a });
  const scopeLabel = (s: string) => t(`roles.scope.${s}`, { defaultValue: s });
  return { typeLabel, actionLabel, scopeLabel, types };
}

export function RolesOverview() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const roles = useQuery(rolesQuery);
  const assignments = useQuery(assignmentsQuery);
  const { run, busy, failure, done } = useRun();
  const navigate = useNavigate();
  const [name, setName] = useState("");
  const [labelDe, setLabelDe] = useState("");
  const [labelEn, setLabelEn] = useState("");
  const [removing, setRemoving] = useState<string | null>(null);
  // The overview of roles in the look of the list (UI54): the number under the title, "Neue
  // Rolle" on the right opens the form, a field narrows the rows, a row opens its role.
  const [creating, setCreating] = useState(false);
  const [query, setQuery] = useState("");
  const density = useDensity();
  const formRef = useRef<HTMLFormElement>(null);
  useEffect(() => {
    if (creating) {
      formRef.current?.querySelector("input")?.focus();
    }
  }, [creating]);
  const nameOk = /^[a-z][a-z0-9_]{0,62}$/.test(name);
  const create = (e: FormEvent) => {
    e.preventDefault();
    if (!nameOk || labelDe.trim() === "") {
      return;
    }
    const label: Text = { de: labelDe.trim() };
    if (labelEn.trim()) {
      label["en"] = labelEn.trim();
    }
    const target = name;
    run(
      t("roles.done.created", { name: labelDe.trim() }),
      () => api.createRole(target, label),
      () => navigate({ to: "/roles/$name", params: { name: target } }),
    );
    setName("");
    setLabelDe("");
    setLabelEn("");
  };
  if (roles.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  const shown = (roles.data ?? []).filter((r) =>
    matches(query, labelOf(r.label, i18n.language, r.name), r.name),
  );
  return (
    <div className="flex max-w-5xl flex-col gap-4">
      <PageHeader
        className="mb-0"
        title={t("roles.title")}
        line={roles.data ? t("roles.count", { count: roles.data.length }) : undefined}
        actions={
          <button
            type="button"
            className={primaryBarButton}
            aria-expanded={creating}
            onClick={() => setCreating((c) => !c)}
          >
            <Plus aria-hidden="true" className="size-4" />
            {t("roles.new")}
          </button>
        }
      />
      <p className="text-sm text-muted-foreground">{t("roles.intro")}</p>
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {done && <Alert variant="success">{done}</Alert>}
      {roles.isError && <Alert variant="destructive">{t("roles.forbidden")}</Alert>}
      {creating && (
        <form
          ref={formRef}
          className="flex flex-col gap-3 rounded-md border bg-card p-3"
          aria-labelledby={`${id}-new`}
          onSubmit={create}
        >
          <h2 id={`${id}-new`} className="font-medium">
            {t("roles.new")}
          </h2>
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
                aria-invalid={name !== "" && !nameOk}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
          </div>
          <div>
            <Button type="submit" disabled={busy || !nameOk || labelDe.trim() === ""}>
              {t("roles.create")}
            </Button>
          </div>
        </form>
      )}
      {roles.data && (
        <>
          <TableSearch label={t("roles.search")} value={query} onChange={setQuery} />
          <Table density={density} aria-label={t("roles.title")}>
            <THead>
              <Tr>
                <Th>{t("roles.col.role")}</Th>
                <Th>{t("roles.col.name")}</Th>
                <Th>{t("roles.col.permissions")}</Th>
                <Th>{t("roles.col.assignments")}</Th>
                <Th>
                  <span className="sr-only">{t("customize.col.action")}</span>
                </Th>
              </Tr>
            </THead>
            <tbody>
              {shown.map((r) => {
                const label = labelOf(r.label, i18n.language, r.name);
                const held = assignments.data?.filter((a) => a.role === r.name).length ?? 0;
                return (
                  <Tr
                    key={r.name}
                    onOpen={() => void navigate({ to: "/roles/$name", params: { name: r.name } })}
                  >
                    <Td className="font-medium">{label}</Td>
                    <Td className="font-mono text-xs">{r.name}</Td>
                    <Td>{r.permissions.length}</Td>
                    <Td>{held}</Td>
                    <Td className="text-right">
                      {r.name !== "admin" &&
                        (removing === r.name ? (
                          <div className="flex flex-wrap items-center justify-end gap-2">
                            <span className="text-sm">
                              {t("roles.confirmRemove", { name: label })}
                            </span>
                            <Button
                              variant="destructive"
                              size="sm"
                              disabled={busy}
                              onClick={() => {
                                setRemoving(null);
                                run(t("roles.done.removed", { name: label }), () =>
                                  api.deleteRole(r.name),
                                );
                              }}
                            >
                              {t("roles.removeYes")}
                            </Button>
                            <Button variant="outline" size="sm" onClick={() => setRemoving(null)}>
                              {t("form.cancel")}
                            </Button>
                          </div>
                        ) : (
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setRemoving(r.name)}
                            aria-label={t("customize.remove", { name: label })}
                          >
                            {t("customize.removeShort")}
                          </Button>
                        ))}
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </Table>
          {shown.length === 0 && (
            <p className="text-sm text-muted-foreground">{t("roles.noMatch")}</p>
          )}
        </>
      )}
    </div>
  );
}

export function RolePage() {
  const { t, i18n } = useTranslation();
  const { name } = useParams({ from: "/shell/roles/$name" });
  const roles = useQuery(rolesQuery);
  const { run, busy, failure, done } = useRun();
  if (roles.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  const role = roles.data?.find((r) => r.name === name);
  if (!role) {
    return <Alert variant="destructive">{t("roles.unknown")}</Alert>;
  }
  const label = labelOf(role.label, i18n.language, role.name);
  return (
    <div className="flex max-w-6xl flex-col gap-6">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-semibold">{t("roles.roleTitle", { name: label })}</h1>
        <LinkButton
          to="/roles"
          icon={<ArrowLeft aria-hidden="true" className="size-4 text-foreground-2" />}
        >
          {t("roles.toAll")}
        </LinkButton>
      </div>
      <p className="text-sm text-muted-foreground">
        {role.name} · {t("roles.intro")}
      </p>
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {done && <Alert variant="success">{done}</Alert>}
      <Matrix role={role} busy={busy} run={run} />
      <Specials role={role} busy={busy} run={run} />
      <Others role={role} busy={busy} run={run} />
      <Assignments role={role} busy={busy} run={run} />
    </div>
  );
}

type Cell = { type: string; action: string };

function Matrix({ role, busy, run }: { role: Role; busy: boolean; run: Run }) {
  const { t } = useTranslation();
  const id = useId();
  const { typeLabel, actionLabel, scopeLabel, types } = useLabels();
  const [onlyGranted, setOnlyGranted] = useState(false);
  const [open, setOpen] = useState<Cell | null>(null);
  const lines = role.permissions.filter(inMatrix);
  const linesOf = (c: Cell) => lines.filter((p) => p.type_name === c.type && p.action === c.action);
  const names = [
    ...(lines.some((p) => p.type_name === any) ? [any] : []),
    ...(types.data ?? [])
      .map(qualifiedName)
      .sort((a, b) => typeLabel(a).localeCompare(typeLabel(b))),
  ];
  const shown = onlyGranted ? names.filter((n) => lines.some((p) => p.type_name === n)) : names;
  const where = (c: Cell) => `${typeLabel(c.type)}: ${actionLabel(c.action)}`;

  const choose = (c: Cell, value: string) => {
    const have = linesOf(c);
    const msg = (s: string) =>
      s === ""
        ? t("roles.done.unset", { where: where(c) })
        : t("roles.done.set", { where: where(c), scope: scopeLabel(s) });
    if (value === "") {
      run(msg(""), () => api.unsetPermission(role.name, c.type, c.action));
      return;
    }
    const old = have[0];
    const next: Permission = { type_name: c.type, action: c.action, scope: value as Scope };
    if (old?.condition) {
      next.condition = old.condition;
    }
    if (old?.fields) {
      next.fields = old.fields;
    }
    run(
      msg(value),
      () => api.setPermission(role.name, next),
      ...(old && old.scope !== value
        ? [() => api.unsetPermission(role.name, c.type, c.action, old.scope)]
        : []),
    );
  };

  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={`${id}-h`} className="text-lg font-semibold">
          {t("roles.permissions")}
        </h2>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={onlyGranted}
            onChange={(e) => setOnlyGranted(e.target.checked)}
          />
          {t("roles.onlyGranted")}
        </label>
      </div>
      <p className="text-sm text-muted-foreground">{t("roles.matrixHelp")}</p>
      <div className="overflow-x-auto">
        <DensityTable>
          <THead>
            <tr>
              <Th>{t("roles.col.type")}</Th>
              {actions.map((a) => (
                <Th key={a}>{actionLabel(a)}</Th>
              ))}
            </tr>
          </THead>
          <tbody>
            {shown.map((typeName) => (
              <Tr key={typeName} className="align-top">
                <th scope="row" className="px-3 py-1.5 text-left font-medium">
                  {typeLabel(typeName)}
                </th>
                {actions.map((a) => {
                  const c = { type: typeName, action: a };
                  const have = linesOf(c);
                  const isOpen = open?.type === typeName && open.action === a;
                  const extra = have.some((p) => p.condition || p.fields);
                  return (
                    <Td key={a}>
                      <div className="flex items-center gap-1">
                        <Select
                          label={where(c)}
                          className={have.length === 0 ? "text-muted-foreground" : undefined}
                          value={have.length > 1 ? "many" : (have[0]?.scope ?? "")}
                          disabled={busy || have.length > 1}
                          onChange={(v) => choose(c, v)}
                          options={[
                            { value: "", label: t("roles.none") },
                            ...scopes.map((s) => ({ value: s, label: scopeLabel(s) })),
                            ...(have.length > 1
                              ? [
                                  {
                                    value: "many",
                                    label: have.map((p) => scopeLabel(p.scope)).join(" + "),
                                  },
                                ]
                              : []),
                          ]}
                        />
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-expanded={isOpen}
                          aria-label={t("roles.details", { where: where(c) })}
                          onClick={() => setOpen(isOpen ? null : c)}
                        >
                          …
                        </Button>
                      </div>
                      {extra && (
                        <div className="text-xs text-muted-foreground">{t("roles.narrowed")}</div>
                      )}
                    </Td>
                  );
                })}
              </Tr>
            ))}
          </tbody>
        </DensityTable>
      </div>
      {open && (
        <CellDetails
          key={`${open.type}:${open.action}`}
          role={role}
          cell={open}
          lines={linesOf(open)}
          type={types.data?.find((x) => qualifiedName(x) === open.type)}
          busy={busy}
          run={run}
          onClose={() => setOpen(null)}
        />
      )}
    </section>
  );
}

// CellDetails edits the lines of one cell: a scope each, with condition and field rights; a
// further scope is one more line.
function CellDetails({
  role,
  cell,
  lines,
  type,
  busy,
  run,
  onClose,
}: {
  role: Role;
  cell: Cell;
  lines: Permission[];
  type: TypeDefinition | undefined;
  busy: boolean;
  run: Run;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const { typeLabel, actionLabel } = useLabels();
  const [adding, setAdding] = useState(false);
  const free = scopes.filter((s) => !lines.some((p) => p.scope === s));
  const where = `${typeLabel(cell.type)}: ${actionLabel(cell.action)}`;
  return (
    <section
      aria-labelledby={`${id}-h`}
      className="flex flex-col gap-3 rounded-md border bg-surface-subtle p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 id={`${id}-h`} className="font-medium">
          {t("roles.detailsTitle", { where })}
        </h3>
        <Button variant="outline" size="sm" onClick={onClose}>
          {t("roles.close")}
        </Button>
      </div>
      {lines.length === 0 && !adding && (
        <p className="text-sm text-muted-foreground">{t("roles.noLine")}</p>
      )}
      {lines.map((p) => (
        <LineEditor
          key={p.scope}
          role={role}
          cell={cell}
          line={p}
          free={free}
          type={type}
          busy={busy}
          run={run}
        />
      ))}
      {adding && free.length > 0 && (
        <LineEditor
          role={role}
          cell={cell}
          free={free}
          type={type}
          busy={busy}
          run={run}
          onDone={() => setAdding(false)}
        />
      )}
      {!adding && free.length > 0 && (
        <div>
          <Button variant="outline" size="sm" onClick={() => setAdding(true)}>
            {lines.length === 0 ? t("roles.addLine") : t("roles.addScope")}
          </Button>
        </div>
      )}
    </section>
  );
}

function LineEditor({
  role,
  cell,
  line,
  free,
  type,
  busy,
  run,
  onDone,
}: {
  role: Role;
  cell: Cell;
  // line is the line edited; without, a new one.
  line?: Permission;
  free: Scope[];
  type: TypeDefinition | undefined;
  busy: boolean;
  run: Run;
  onDone?: () => void;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const { typeLabel, actionLabel, scopeLabel } = useLabels();
  const [scope, setScope] = useState<Scope>(line?.scope ?? free[0] ?? "own");
  const [condition, setCondition] = useState(line?.condition ?? "");
  const fieldsOf = (f: unknown): FieldRights | null => (f ? (f as FieldRights) : null);
  const [fields, setFields] = useState<FieldRights | null>(fieldsOf(line?.fields));
  const where = `${typeLabel(cell.type)}: ${actionLabel(cell.action)}`;
  // The field rights that count for the action: read decides what is read, create and update
  // what is written (PL9); delete has none.
  const column: "read" | "write" | null =
    cell.action === "read" ? "read" : cell.action === "delete" ? null : "write";
  const choices = (type?.fields ?? []).filter((f) => !f.computed);
  const chosen = fields && column ? (fields[column] ?? []) : [];

  const save = (e: FormEvent) => {
    e.preventDefault();
    const next: Permission = { type_name: cell.type, action: cell.action, scope };
    if (condition.trim()) {
      next.condition = condition.trim();
    }
    if (fields && column) {
      next.fields = { [column]: chosen };
    }
    run(
      t("roles.done.saved", { where, scope: scopeLabel(scope) }),
      () => api.setPermission(role.name, next),
      ...(line && line.scope !== scope
        ? [() => api.unsetPermission(role.name, cell.type, cell.action, line.scope)]
        : []),
    );
    onDone?.();
  };
  const options = line ? [line.scope, ...free] : free;
  return (
    <form
      className="flex flex-col gap-3 rounded-md border bg-background p-3"
      aria-label={t("roles.lineOf", { where, scope: scopeLabel(line?.scope ?? scope) })}
      onSubmit={save}
    >
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-scope`}>{t("roles.col.scope")}</Label>
          <Select
            id={`${id}-scope`}
            value={scope}
            onChange={(v) => setScope(v as Scope)}
            options={options.map((s) => ({ value: s, label: scopeLabel(s) }))}
          />
        </div>
        <div className="flex flex-col gap-1 sm:col-span-2">
          <Label htmlFor={`${id}-when`}>{t("roles.condition")}</Label>
          <Input
            id={`${id}-when`}
            className="font-mono"
            value={condition}
            aria-describedby={`${id}-when-help`}
            onChange={(e) => setCondition(e.target.value)}
          />
          <p id={`${id}-when-help`} className="text-xs text-muted-foreground">
            {t("roles.conditionHelp")}
          </p>
        </div>
      </div>
      {column && cell.type !== any && (
        <fieldset className="flex flex-col gap-2">
          <legend className="text-sm font-medium">
            {t(column === "read" ? "roles.fieldsRead" : "roles.fieldsWrite")}
          </legend>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={fields === null}
              onChange={(e) =>
                setFields(e.target.checked ? null : { [column]: choices.map((f) => f.name) })
              }
            />
            {t("roles.allFields")}
          </label>
          {fields !== null && (
            <div className="grid gap-1 sm:grid-cols-3">
              {choices.map((f) => (
                <label key={f.name} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={chosen.includes(f.name)}
                    onChange={(e) =>
                      setFields({
                        [column]: e.target.checked
                          ? [...chosen, f.name]
                          : chosen.filter((x) => x !== f.name),
                      })
                    }
                  />
                  {fieldLabel(f, i18n.language)}
                </label>
              ))}
              <p className="text-xs text-muted-foreground sm:col-span-3">
                {t("roles.systemFields")}
              </p>
            </div>
          )}
        </fieldset>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={busy}>
          {line ? t("customize.save") : t("roles.addLineShort")}
        </Button>
        {line && (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() =>
              run(t("roles.done.lineRemoved", { where, scope: scopeLabel(line.scope) }), () =>
                api.unsetPermission(role.name, cell.type, cell.action, line.scope),
              )
            }
          >
            {t("roles.removeLine")}
          </Button>
        )}
        {!line && onDone && (
          <Button variant="outline" size="sm" onClick={onDone}>
            {t("form.cancel")}
          </Button>
        )}
      </div>
    </form>
  );
}

function Specials({ role, busy, run }: { role: Role; busy: boolean; run: Run }) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-2">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("roles.specials")}
      </h2>
      <div className="grid gap-2 sm:grid-cols-2">
        {specials.map((s) => {
          const held = role.permissions.some(
            (p) => p.type_name === s.type && p.action === s.action,
          );
          const label = t(`roles.special.${s.type.replace("core.", "")}_${s.action}`);
          return (
            <label key={`${s.type}:${s.action}`} className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={held}
                disabled={busy}
                onChange={() =>
                  held
                    ? run(t("roles.done.specialOff", { name: label }), () =>
                        api.unsetPermission(role.name, s.type, s.action),
                      )
                    : run(t("roles.done.specialOn", { name: label }), () =>
                        api.setPermission(role.name, {
                          type_name: s.type,
                          action: s.action,
                          scope: "all",
                        }),
                      )
                }
              />
              {label}
            </label>
          );
        })}
      </div>
    </section>
  );
}

// Others are the permissions neither the matrix nor the boxes show: other actions of a type
// (read_private, comment, …) and everything with the action *.
function Others({ role, busy, run }: { role: Role; busy: boolean; run: Run }) {
  const { t } = useTranslation();
  const id = useId();
  const { typeLabel, actionLabel, scopeLabel, types } = useLabels();
  const others = role.permissions.filter((p) => !inMatrix(p) && !isSpecial(p));
  const [typeName, setTypeName] = useState("");
  const [action, setAction] = useState("");
  const [scope, setScope] = useState<Scope>("all");
  const ready = typeName !== "" && /^([a-z][a-z0-9_]{0,62}|\*)$/.test(action);
  const add = (e: FormEvent) => {
    e.preventDefault();
    if (!ready) {
      return;
    }
    run(
      t("roles.done.set", {
        where: `${typeLabel(typeName)}: ${actionLabel(action)}`,
        scope: scopeLabel(scope),
      }),
      () => api.setPermission(role.name, { type_name: typeName, action, scope }),
    );
    setAction("");
  };
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("roles.others")}
      </h2>
      <p className="text-sm text-muted-foreground">{t("roles.othersHelp")}</p>
      {others.length > 0 && (
        <DensityTable>
          <THead>
            <tr>
              <Th>{t("roles.col.type")}</Th>
              <Th>{t("roles.col.action")}</Th>
              <Th>{t("roles.col.scope")}</Th>
              <Th>
                <span className="sr-only">{t("customize.col.action")}</span>
              </Th>
            </tr>
          </THead>
          <tbody>
            {others.map((p) => {
              const where = `${typeLabel(p.type_name)}: ${actionLabel(p.action)}`;
              return (
                <Tr key={`${p.type_name}:${p.action}:${p.scope}`}>
                  <Td>{typeLabel(p.type_name)}</Td>
                  <Td>{actionLabel(p.action)}</Td>
                  <Td>{scopeLabel(p.scope)}</Td>
                  <Td>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      aria-label={t("customize.remove", { name: where })}
                      onClick={() =>
                        run(t("roles.done.unset", { where }), () =>
                          api.unsetPermission(role.name, p.type_name, p.action, p.scope),
                        )
                      }
                    >
                      {t("customize.removeShort")}
                    </Button>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DensityTable>
      )}
      <form
        className="flex flex-wrap items-end gap-3"
        aria-label={t("roles.newOther")}
        onSubmit={add}
      >
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-type`}>{t("roles.col.type")}</Label>
          <Select
            id={`${id}-type`}
            value={typeName}
            onChange={setTypeName}
            options={[
              { value: "", label: t("builder.choose") },
              { value: any, label: t("roles.anyType") },
              ...(types.data ?? []).map((ty) => ({
                value: qualifiedName(ty),
                label: typeLabel(qualifiedName(ty)),
              })),
            ]}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-action`}>{t("roles.col.action")}</Label>
          <Input id={`${id}-action`} value={action} onChange={(e) => setAction(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-scope`}>{t("roles.col.scope")}</Label>
          <Select
            id={`${id}-scope`}
            value={scope}
            onChange={(v) => setScope(v as Scope)}
            options={scopes.map((s) => ({ value: s, label: scopeLabel(s) }))}
          />
        </div>
        <Button type="submit" variant="outline" disabled={busy || !ready}>
          {t("roles.addOther")}
        </Button>
      </form>
    </section>
  );
}

function Assignments({ role, busy, run }: { role: Role; busy: boolean; run: Run }) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const assignments = useQuery(assignmentsQuery);
  const nodes = useQuery(orgNodesQuery);
  const [who, setWho] = useState<ActorRef[]>([]);
  const [node, setNode] = useState("root");
  const mine = (assignments.data ?? []).filter((a) => a.role === role.name);
  const nodeLabel = (path: string) => {
    const n = nodes.data?.find((x) => x.path === path);
    return n ? `${labelOf(n.label, i18n.language, n.name)} (${path})` : path;
  };
  const add = (e: FormEvent) => {
    e.preventDefault();
    const ref = who[0];
    if (!ref) {
      return;
    }
    const body =
      ref.type === "core.group"
        ? { role: role.name, group_id: ref.id, org_path: node }
        : { role: role.name, person_id: ref.id, org_path: node };
    run(t("roles.done.assigned", { node: nodeLabel(node) }), () => api.createAssignment(body));
    setWho([]);
  };
  return (
    <section aria-labelledby={`${id}-h`} className="flex flex-col gap-3">
      <h2 id={`${id}-h`} className="text-lg font-semibold">
        {t("roles.assignments")}
      </h2>
      {mine.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("roles.noAssignments")}</p>
      ) : (
        <DensityTable>
          <THead>
            <tr>
              <Th>{t("roles.col.holder")}</Th>
              <Th>{t("roles.col.node")}</Th>
              <Th>
                <span className="sr-only">{t("customize.col.action")}</span>
              </Th>
            </tr>
          </THead>
          <tbody>
            {mine.map((a) => {
              const ref: ActorRef = a.group_id
                ? { type: "core.group", id: a.group_id }
                : { type: "core.person", id: a.person_id ?? "" };
              return (
                <Tr key={a.id}>
                  <Td>
                    <RefName refTo={ref} />
                  </Td>
                  <Td>{nodeLabel(a.org_path)}</Td>
                  <Td>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      aria-label={t("roles.unassign", { node: nodeLabel(a.org_path) })}
                      onClick={() =>
                        run(t("roles.done.unassigned", { node: nodeLabel(a.org_path) }), () =>
                          api.deleteAssignment(a.id),
                        )
                      }
                    >
                      {t("customize.removeShort")}
                    </Button>
                  </Td>
                </Tr>
              );
            })}
          </tbody>
        </DensityTable>
      )}
      <form
        className="flex flex-wrap items-end gap-3"
        aria-label={t("roles.newAssignment")}
        onSubmit={add}
      >
        <div className="flex min-w-64 flex-col gap-1">
          <Label htmlFor={`${id}-who`}>{t("roles.col.holder")}</Label>
          <RefPicker
            id={`${id}-who`}
            targets={["core.person", "core.group"]}
            value={who}
            onChange={setWho}
            single
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-node`}>{t("roles.col.node")}</Label>
          <Select
            id={`${id}-node`}
            value={node}
            onChange={setNode}
            options={(nodes.data ?? [{ path: "root", name: "root", label: {} }]).map((n) => ({
              value: n.path,
              label: nodeLabel(n.path),
            }))}
          />
        </div>
        <Button type="submit" variant="outline" disabled={busy || who.length === 0}>
          {t("roles.assign")}
        </Button>
      </form>
    </section>
  );
}
