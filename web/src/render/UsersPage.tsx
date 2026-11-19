// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Users (UI66, UI-Plan E63, Kern-Plan PL77): a table over the full width with name, username,
// origin, state, last sign-in and roles; search and filters. "Neuer Benutzer" makes person and
// local account in one step, with an invitation by mail or a one-time password. The page of a
// user shows the account and its roles: add a role at a unit, remove one after the question;
// block and allow, a new one-time password, a new invitation.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useId, useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  type AccountInfo,
  type Assignment,
  type OrgNode,
  type Role,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { confirm } from "@/ui/confirm";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { Menu, MenuItem } from "@/ui/menu";
import { BackButton, PageHeader } from "@/ui/page-header";
import { Select } from "@/ui/select";
import { Table, Td, Th, THead, Tr } from "@/ui/table";
import { toast } from "@/ui/toast";

import { formatMoment } from "./fields";
import { useDensity } from "./TableTools";

const accountsQuery = { queryKey: ["accounts"], queryFn: api.accounts };
const assignmentsQuery = { queryKey: ["assignments"], queryFn: api.assignments };
const rolesQuery = { queryKey: ["roles"], queryFn: api.roles };
const nodesQuery = { queryKey: ["org", "nodes"], queryFn: api.orgNodes };

function useNames() {
  const { i18n } = useTranslation();
  const roles = useQuery(rolesQuery);
  const nodes = useQuery(nodesQuery);
  return useMemo(() => {
    const role = new Map((roles.data ?? []).map((r: Role) => [r.name, r]));
    const node = new Map((nodes.data ?? []).map((n: OrgNode) => [n.path, n]));
    return {
      roles: roles.data ?? [],
      nodes: nodes.data ?? [],
      roleLabel: (name: string) => labelOf(role.get(name)?.label, i18n.language, name),
      nodeLabel: (path: string) => labelOf(node.get(path)?.label, i18n.language, path),
    };
  }, [roles.data, nodes.data, i18n.language]);
}

// rolesOf are the assignments of the person of an account.
function rolesOf(a: AccountInfo, assignments: Assignment[] | undefined): Assignment[] {
  return a.person_id ? (assignments ?? []).filter((x) => x.person_id === a.person_id) : [];
}

function origin(a: AccountInfo, t: (k: string) => string): string {
  return a.origin === "oidc"
    ? a.issuer
      ? `${t("users.oidc")} (${a.issuer})`
      : t("users.oidc")
    : t("users.local");
}

function state(
  a: AccountInfo,
  t: (k: string, o?: Record<string, unknown>) => string,
  lang: string,
) {
  if (!a.active) return t("users.blocked");
  if (a.invited_until) return t("users.invited", { until: formatMoment(a.invited_until, lang) });
  if (a.password_expired) return t("users.mustChange");
  return t("users.active");
}

export function UsersPage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const navigate = useNavigate();
  const density = useDensity();
  const accounts = useQuery(accountsQuery);
  const assignments = useQuery(assignmentsQuery);
  const names = useNames();
  const [q, setQ] = useState("");
  const [from, setFrom] = useState("");
  const [role, setRole] = useState("");
  const rows = (accounts.data ?? []).filter((a) => {
    const text = `${a.display_name} ${a.username}`.toLowerCase();
    if (q.trim() && !text.includes(q.trim().toLowerCase())) return false;
    if (from && a.origin !== from) return false;
    if (role && !rolesOf(a, assignments.data).some((x) => x.role === role)) return false;
    return true;
  });
  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title={t("users.title")}
        line={accounts.data ? t("users.count", { count: rows.length }) : undefined}
        actions={
          <Button size="bar" onClick={() => void navigate({ to: "/users/new" })}>
            {t("users.new")}
          </Button>
        }
      />
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-q`} className="text-sm font-normal text-foreground-2">
            {t("users.search")}
          </Label>
          <Input
            id={`${id}-q`}
            size="bar"
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <span id={`${id}-from`} className="text-sm text-foreground-2">
            {t("users.origin")}
          </span>
          <Select
            labelledBy={`${id}-from`}
            size="bar"
            value={from}
            onChange={setFrom}
            options={[
              { value: "", label: t("users.all") },
              { value: "local", label: t("users.local") },
              { value: "oidc", label: t("users.oidc") },
            ]}
          />
        </div>
        <div className="flex flex-col gap-1">
          <span id={`${id}-role`} className="text-sm text-foreground-2">
            {t("users.role")}
          </span>
          <Select
            labelledBy={`${id}-role`}
            size="bar"
            value={role}
            onChange={setRole}
            options={[
              { value: "", label: t("users.all") },
              ...names.roles.map((r) => ({ value: r.name, label: names.roleLabel(r.name) })),
            ]}
          />
        </div>
      </div>
      {accounts.isError && (
        <Alert variant="destructive">
          {accounts.error instanceof ApiError && accounts.error.status === 403
            ? t("users.forbidden")
            : t("users.error")}
        </Alert>
      )}
      {accounts.isPending && <p>{t("list.loading")}</p>}
      {rows.length > 0 && (
        <Table density={density}>
          <THead>
            <tr>
              <Th>{t("users.name")}</Th>
              <Th>{t("users.username")}</Th>
              <Th>{t("users.origin")}</Th>
              <Th>{t("users.state")}</Th>
              <Th>{t("users.lastLogin")}</Th>
              <Th>{t("users.roles")}</Th>
            </tr>
          </THead>
          <tbody>
            {rows.map((a) => (
              <Tr
                key={a.id}
                onOpen={() => void navigate({ to: "/users/$id", params: { id: a.id } })}
              >
                <Td className="font-medium">{a.display_name}</Td>
                <Td className="font-mono text-[12.5px]">{a.username}</Td>
                <Td>{origin(a, t)}</Td>
                <Td className={a.active ? undefined : "text-destructive"}>
                  {state(a, t, i18n.language)}
                </Td>
                <Td className="font-mono text-[12.5px]">
                  {a.last_login_at ? formatMoment(a.last_login_at, i18n.language) : "–"}
                </Td>
                <Td className="max-w-96 truncate">
                  {rolesOf(a, assignments.data)
                    .map((x) => `${names.roleLabel(x.role)} (${names.nodeLabel(x.org_path)})`)
                    .join(", ") || "–"}
                </Td>
              </Tr>
            ))}
          </tbody>
        </Table>
      )}
      {accounts.isSuccess && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("users.none")}</p>
      )}
    </div>
  );
}

// reason says why an action failed; without a mail server (503) it names the way out.
function reason(err: unknown, t: (key: string) => string): string {
  if (err instanceof ApiError && err.status === 503) return t("users.noMail");
  return err instanceof Error ? err.message : String(err);
}

// fieldError shows the problem of the server at a field.
function fieldErrors(err: unknown): Record<string, string> {
  if (!(err instanceof ApiError)) return {};
  return Object.fromEntries(err.fieldErrors.map((f) => [f.field, f.message]));
}

export function NewUserPage() {
  const { t } = useTranslation();
  const id = useId();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const names = useNames();
  const [first, setFirst] = useState("");
  const [last, setLast] = useState("");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [unit, setUnit] = useState("root");
  const [way, setWay] = useState<"invite" | "password">("invite");
  const [password, setPassword] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});
  const create = useMutation({
    mutationFn: () =>
      api.createAccount({
        username: username.trim(),
        last_name: last.trim(),
        ...(first.trim() ? { first_name: first.trim() } : {}),
        ...(email.trim() ? { email: email.trim() } : {}),
        org_path: unit,
        ...(way === "password" ? { password } : {}),
      }),
    onMutate: () => setErrors({}),
    onSuccess: (a) => {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      toast.success(
        way === "invite" ? t("users.invitedTo", { email: email.trim() }) : t("users.createdOnce"),
        a.display_name,
      );
      void navigate({ to: "/users/$id", params: { id: a.id } });
    },
    onError: (err) => {
      const f = fieldErrors(err);
      setErrors(f);
      if (err instanceof ApiError && err.status === 409) {
        setErrors({ username: t("users.taken") });
      } else if (Object.keys(f).length === 0) {
        toast.error(t("users.createFailed"), reason(err, t));
      }
    },
  });
  const field = (key: string, label: string, el: ReactNode, required = false) => (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={`${id}-${key}`}>
        {label}
        {required && " *"}
      </Label>
      {el}
      {errors[key] && (
        <p id={`${id}-${key}-error`} className="text-sm text-destructive">
          {errors[key]}
        </p>
      )}
    </div>
  );
  const describe = (key: string) => (errors[key] ? `${id}-${key}-error` : undefined);
  return (
    <div className="flex flex-col gap-4">
      <BackButton to="/users" label={t("users.title")} />
      <h1 className="text-2xl font-semibold">{t("users.new")}</h1>
      <form
        className="flex max-w-2xl flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <div className="grid gap-4 sm:grid-cols-2">
          {field(
            "first_name",
            t("users.firstName"),
            <Input
              id={`${id}-first_name`}
              value={first}
              onChange={(e) => setFirst(e.target.value)}
            />,
          )}
          {field(
            "last_name",
            t("users.lastName"),
            <Input
              id={`${id}-last_name`}
              value={last}
              required
              aria-invalid={errors["last_name"] ? true : undefined}
              aria-describedby={describe("last_name")}
              onChange={(e) => setLast(e.target.value)}
            />,
            true,
          )}
          {field(
            "username",
            t("users.username"),
            <Input
              id={`${id}-username`}
              value={username}
              required
              autoCapitalize="none"
              aria-invalid={errors["username"] ? true : undefined}
              aria-describedby={describe("username") ?? `${id}-username-hint`}
              onChange={(e) => setUsername(e.target.value)}
            />,
            true,
          )}
          {field(
            "email",
            t("users.email"),
            <Input
              id={`${id}-email`}
              type="email"
              value={email}
              aria-invalid={errors["email"] ? true : undefined}
              aria-describedby={describe("email")}
              onChange={(e) => setEmail(e.target.value)}
            />,
            way === "invite",
          )}
        </div>
        <p id={`${id}-username-hint`} className="-mt-2 text-[12.5px] text-muted-foreground">
          {t("users.usernameHint")}
        </p>
        <div className="flex flex-col gap-1.5">
          <span id={`${id}-unit`} className="text-sm font-medium">
            {t("users.unit")}
          </span>
          <Select
            labelledBy={`${id}-unit`}
            value={unit}
            onChange={setUnit}
            wrapClassName="max-w-md"
            options={names.nodes.map((n) => ({
              value: n.path,
              label: `${names.nodeLabel(n.path)} (${n.path})`,
            }))}
          />
        </div>
        <fieldset className="flex flex-col gap-2">
          <legend className="mb-1 text-sm font-medium">{t("users.firstPassword")}</legend>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="radio"
              name={`${id}-way`}
              checked={way === "invite"}
              onChange={() => setWay("invite")}
              className="mt-1"
            />
            <span>
              {t("users.wayInvite")}
              <span className="block text-[12.5px] text-muted-foreground">
                {t("users.wayInviteHint")}
              </span>
            </span>
          </label>
          <label className="flex items-start gap-2 text-sm">
            <input
              type="radio"
              name={`${id}-way`}
              checked={way === "password"}
              onChange={() => setWay("password")}
              className="mt-1"
            />
            <span>
              {t("users.wayPassword")}
              <span className="block text-[12.5px] text-muted-foreground">
                {t("users.wayPasswordHint")}
              </span>
            </span>
          </label>
        </fieldset>
        {way === "password" &&
          field(
            "password",
            t("users.oneTimePassword"),
            <Input
              id={`${id}-password`}
              type="text"
              autoComplete="off"
              value={password}
              aria-invalid={errors["password"] ? true : undefined}
              aria-describedby={describe("password")}
              onChange={(e) => setPassword(e.target.value)}
              className="max-w-md"
            />,
            true,
          )}
        <div className="flex gap-2">
          <Button type="submit" disabled={create.isPending}>
            {way === "invite" ? t("users.createInvite") : t("users.create")}
          </Button>
          <Button type="button" variant="outline" onClick={() => void navigate({ to: "/users" })}>
            {t("form.cancel")}
          </Button>
        </div>
      </form>
    </div>
  );
}

export function UserPage() {
  const { t, i18n } = useTranslation();
  const id = useId();
  const { id: accountId } = useParams({ from: "/shell/users/$id" });
  const queryClient = useQueryClient();
  const density = useDensity();
  const accounts = useQuery(accountsQuery);
  const assignments = useQuery(assignmentsQuery);
  const names = useNames();
  const [role, setRole] = useState("");
  const [unit, setUnit] = useState("root");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const account = accounts.data?.find((a) => a.id === accountId);
  const reload = () => {
    void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    void queryClient.invalidateQueries({ queryKey: ["assignments"] });
  };
  const failed = (err: unknown) => toast.error(t("users.failed"), reason(err, t));
  const run = async (work: () => Promise<unknown>, done: string) => {
    try {
      await work();
      toast.success(done, account?.display_name);
      reload();
    } catch (err) {
      failed(err);
    }
  };
  if (accounts.isPending) return <p>{t("list.loading")}</p>;
  if (!account) {
    return <Alert variant="destructive">{t("users.notFound")}</Alert>;
  }
  const mine = rolesOf(account, assignments.data);
  const local = account.origin === "local";
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-start justify-between gap-2">
        <BackButton to="/users" label={t("users.title")} />
        <Menu label={t("trash.actions")} trigger={t("trash.actions")} width="w-64">
          {account.active ? (
            <MenuItem
              className="text-destructive"
              onSelect={() =>
                void confirm({
                  title: t("users.blockAsk", { name: account.display_name }),
                  text: t("users.blockText"),
                  action: t("users.block"),
                }).then(
                  (yes) =>
                    yes && void run(() => api.disableAccount(account.id), t("users.blockedDone")),
                )
              }
            >
              {t("users.block")}
            </MenuItem>
          ) : (
            <MenuItem
              onSelect={() => void run(() => api.enableAccount(account.id), t("users.allowedDone"))}
            >
              {t("users.allow")}
            </MenuItem>
          )}
          {local && (
            <MenuItem onSelect={() => setShowPassword(true)}>{t("users.setOnce")}</MenuItem>
          )}
          {local && (
            <MenuItem
              onSelect={() =>
                void confirm({
                  title: t("users.reinviteAsk", { name: account.display_name }),
                  text: t("users.reinviteText"),
                  action: t("users.reinvite"),
                }).then(
                  (yes) =>
                    yes && void run(() => api.inviteAccount(account.id), t("users.reinvited")),
                )
              }
            >
              {t("users.reinvite")}
            </MenuItem>
          )}
        </Menu>
      </div>
      <div>
        <h1 className="text-2xl font-semibold">{account.display_name}</h1>
        <p className="text-sm text-muted-foreground">
          {account.username} · {origin(account, t)} · {state(account, t, i18n.language)}
        </p>
      </div>
      {showPassword && (
        <form
          aria-label={t("users.setOnce")}
          className="flex flex-wrap items-end gap-2 rounded-md border bg-card p-4"
          onSubmit={(e) => {
            e.preventDefault();
            void run(() => api.setAccountPassword(account.id, password), t("users.onceSet")).then(
              () => {
                setPassword("");
                setShowPassword(false);
              },
            );
          }}
        >
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-once`}>{t("users.oneTimePassword")}</Label>
            <Input
              id={`${id}-once`}
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={password === ""}>
            {t("users.setOnceShort")}
          </Button>
          <Button type="button" variant="outline" onClick={() => setShowPassword(false)}>
            {t("form.cancel")}
          </Button>
          <p className="w-full text-[12.5px] text-muted-foreground">{t("users.wayPasswordHint")}</p>
        </form>
      )}
      <section aria-labelledby={`${id}-account`} className="flex flex-col gap-2">
        <h2 id={`${id}-account`} className="text-[15px] font-semibold">
          {t("users.account")}
        </h2>
        <dl className="grid grid-cols-[12rem_minmax(0,1fr)] gap-x-3 gap-y-1.5 text-sm">
          <dt className="text-foreground-2">{t("users.username")}</dt>
          <dd className="font-mono">{account.username}</dd>
          <dt className="text-foreground-2">{t("users.origin")}</dt>
          <dd>{origin(account, t)}</dd>
          <dt className="text-foreground-2">{t("users.state")}</dt>
          <dd>{state(account, t, i18n.language)}</dd>
          <dt className="text-foreground-2">{t("users.lastLogin")}</dt>
          <dd>
            {account.last_login_at ? formatMoment(account.last_login_at, i18n.language) : "–"}
          </dd>
          {account.invited_email && (
            <>
              <dt className="text-foreground-2">{t("users.invitedEmail")}</dt>
              <dd>{account.invited_email}</dd>
            </>
          )}
          {account.person_id && (
            <>
              <dt className="text-foreground-2">{t("users.person")}</dt>
              <dd>
                <Link
                  to="/types/$typeName/$id"
                  params={{ typeName: "core.person", id: account.person_id }}
                  className="underline"
                >
                  {account.display_name}
                </Link>
              </dd>
            </>
          )}
        </dl>
      </section>
      <section aria-labelledby={`${id}-roles`} className="flex flex-col gap-3">
        <h2 id={`${id}-roles`} className="text-[15px] font-semibold">
          {t("users.roles")}
        </h2>
        {!account.person_id && (
          <p className="text-sm text-muted-foreground">{t("users.noPerson")}</p>
        )}
        {mine.length > 0 && (
          <Table density={density}>
            <THead>
              <tr>
                <Th>{t("users.role")}</Th>
                <Th>{t("users.unit")}</Th>
                <Th>
                  <span className="sr-only">{t("users.actions")}</span>
                </Th>
              </tr>
            </THead>
            <tbody>
              {mine.map((x) => (
                <Tr key={x.id}>
                  <Td>{names.roleLabel(x.role)}</Td>
                  <Td>{names.nodeLabel(x.org_path)}</Td>
                  <Td className="text-right">
                    <Button
                      size="sm"
                      variant="destructive"
                      aria-label={t("users.removeRole", {
                        role: names.roleLabel(x.role),
                        unit: names.nodeLabel(x.org_path),
                      })}
                      onClick={() =>
                        void confirm({
                          title: t("users.removeRoleAsk", {
                            role: names.roleLabel(x.role),
                            unit: names.nodeLabel(x.org_path),
                          }),
                          action: t("customize.removeShort"),
                        }).then(
                          (yes) =>
                            yes &&
                            void run(() => api.deleteAssignment(x.id), t("users.roleRemoved")),
                        )
                      }
                    >
                      {t("customize.removeShort")}
                    </Button>
                  </Td>
                </Tr>
              ))}
            </tbody>
          </Table>
        )}
        {account.person_id && mine.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("users.noRoles")}</p>
        )}
        {account.person_id && (
          <form
            aria-label={t("users.addRole")}
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (!role) return;
              void run(
                () => api.createAssignment({ role, person_id: account.person_id!, org_path: unit }),
                t("users.roleAdded"),
              ).then(() => setRole(""));
            }}
          >
            <div className="flex flex-col gap-1">
              <span id={`${id}-add-role`} className="text-sm text-foreground-2">
                {t("users.role")}
              </span>
              <Select
                labelledBy={`${id}-add-role`}
                size="bar"
                value={role}
                onChange={setRole}
                placeholder={t("users.chooseRole")}
                options={names.roles.map((r) => ({
                  value: r.name,
                  label: names.roleLabel(r.name),
                }))}
              />
            </div>
            <div className="flex flex-col gap-1">
              <span id={`${id}-add-unit`} className="text-sm text-foreground-2">
                {t("users.unit")}
              </span>
              <Select
                labelledBy={`${id}-add-unit`}
                size="bar"
                value={unit}
                onChange={setUnit}
                options={names.nodes.map((n) => ({
                  value: n.path,
                  label: names.nodeLabel(n.path),
                }))}
              />
            </div>
            <Button type="submit" size="bar" disabled={!role}>
              {t("users.addRole")}
            </Button>
          </form>
        )}
      </section>
    </div>
  );
}
