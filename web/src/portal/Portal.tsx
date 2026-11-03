// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The portal for users (UI10, Hauptplan E132; architektur.md §10 portal/): a plain frame, the
// types their package offers in the portal with one's own records, creating them with the
// fields the type names, and a lean page per record with the public timeline. What a person
// may read and change the API decides with the rights at the records; the portal only shows
// less. Agents reach it too, to see what users see. The forms (UI11) are offered to whoever may
// create their target type; filling one in creates its record.
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  meQuery,
  mySettingsQuery,
  qualifiedName,
  recordQuery,
  typesQuery,
  type Field,
  type FormOffer,
  type Label,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { forget } from "@/app/recent";
import { LanguageSwitch, ThemeSwitch } from "@/app/Switches";
import { Form } from "@/render/Form";

import { hiddenOf } from "./conditions";
import { fieldLabel, formatMoment, formatValue, setDisplayTimeZone } from "@/render/fields";
import { Timeline } from "@/render/Timeline";
import { Transitions } from "@/render/Transitions";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { applyColorScheme, isColorScheme } from "@/ui/colorScheme";
import { applyTheme, isTheme } from "@/ui/theme";

export function PortalShell() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  useEffect(() => {
    const s = settings.data;
    if (!s) {
      return;
    }
    if (s.language.value !== i18n.language) {
      void i18n.changeLanguage(s.language.value);
      document.documentElement.lang = s.language.value;
    }
    if (isTheme(s.theme.value)) {
      applyTheme(s.theme.value);
    }
    // Light, dark or like the system (UI29).
    if (isColorScheme(s.color_scheme?.value)) {
      applyColorScheme(s.color_scheme.value);
    }
    setDisplayTimeZone(s.time_zone.value);
  }, [settings.data, i18n]);
  const logout = async () => {
    // The records of the command line go with the sign-out (UI31).
    if (me.data) {
      forget(me.data.id);
    }
    try {
      await api.logout();
    } finally {
      queryClient.clear();
      await navigate({ to: "/login" });
    }
  };
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex flex-wrap items-center gap-4 border-b bg-card px-4 py-2">
        <Link to="/portal" className="text-lg font-semibold text-primary">
          {t("portal.title")}
        </Link>
        <div className="ml-auto flex flex-wrap items-center gap-4">
          <LanguageSwitch persist />
          <ThemeSwitch persist />
          {me.data?.workplace && (
            <Link to="/" className="text-sm underline">
              {t("portal.toWorkplace")}
            </Link>
          )}
          {me.data && (
            <Link to="/portal/settings" className="text-sm text-muted-foreground underline">
              {t("nav.user", { name: me.data.display_name })}
            </Link>
          )}
          <Button variant="outline" size="sm" onClick={() => void logout()}>
            {t("nav.logout")}
          </Button>
        </div>
      </header>
      <main className="mx-auto w-full max-w-4xl flex-1 p-6">
        <Outlet />
      </main>
    </div>
  );
}

// mine is the condition of one's own records: as requester, or created oneself.
function mine(type: TypeDefinition, account: string): Record<string, unknown> {
  const created = { field: "created_by", eq: account };
  return type.actors?.some((r) => r.name === "requester")
    ? { or: [{ field: "actors.requester", eq: "@me" }, created] }
    : created;
}

function MyRecords({ type }: { type: TypeDefinition }) {
  const { t, i18n } = useTranslation();
  const me = useQuery(meQuery);
  const typeName = qualifiedName(type);
  const title = type.title_field ?? type.fields[0]?.name ?? "id";
  const list = useQuery({
    queryKey: ["records", typeName, "list", "portal", me.data?.id],
    queryFn: () =>
      api.query(typeName, {
        where: mine(type, me.data!.id),
        fields: [title, "state", "updated_at"],
        sort: ["-updated_at"],
        limit: 50,
      }),
    enabled: Boolean(me.data),
  });
  const headingId = `portal-${typeName.replace(".", "-")}`;
  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col gap-3 rounded-md border bg-card p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id={headingId} className="text-lg font-semibold">
          {labelOf(type.label, i18n.language, typeName)}
        </h2>
        <Link
          to="/portal/$typeName/new"
          params={{ typeName }}
          className="rounded-md bg-primary px-3 py-1.5 text-sm text-primary-foreground"
        >
          {t("portal.new")}
        </Link>
      </div>
      {list.data && list.data.items.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("portal.none")}</p>
      )}
      <ul className="flex flex-col divide-y">
        {(list.data?.items ?? []).map((r) => {
          const state = type.states?.find((s) => s.name === r["state"]);
          return (
            <li key={String(r["id"])} className="flex flex-wrap justify-between gap-2 py-2 text-sm">
              <Link
                to="/portal/$typeName/$id"
                params={{ typeName, id: String(r["id"]) }}
                className="underline"
              >
                {String(r[title] ?? r["id"])}
              </Link>
              <span className="text-muted-foreground">
                {state ? labelOf(state.label, i18n.language, state.name) : ""}
                {" · "}
                {formatMoment(r["updated_at"], i18n.language)}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

// formsQuery are the forms offered to the session (UI11).
const formsQuery = { queryKey: ["forms"], queryFn: api.forms, staleTime: 60_000 };

// inLanguage is a text of a form in the language, else the German one (UI12).
function inLanguage(label: Label | undefined, language: string, german: string): string {
  return (language !== "de" && label?.[language]) || german;
}

// both is a label of a form for labelOf: English falls back to German (UI12).
function both(label: Label | undefined): Label | undefined {
  if (!label || !Object.values(label).some((v) => v)) {
    return undefined;
  }
  const de = label["de"] ?? "";
  return { ...label, de, en: label["en"] || de };
}

function Forms() {
  const { t, i18n } = useTranslation();
  const forms = useQuery(formsQuery);
  if (!forms.data || forms.data.length === 0) {
    return null;
  }
  return (
    <section
      aria-labelledby="portal-forms"
      className="flex flex-col gap-3 rounded-md border bg-card p-4"
    >
      <h2 id="portal-forms" className="text-lg font-semibold">
        {t("portal.forms")}
      </h2>
      <ul className="flex flex-col divide-y">
        {forms.data.map((f) => (
          <li key={f.id} className="flex flex-col gap-1 py-2 text-sm">
            <Link
              to="/portal/forms/$formId"
              params={{ formId: f.id }}
              className="font-medium underline"
            >
              {inLanguage(f.definition.title, i18n.language, f.name)}
            </Link>
            {f.description && (
              <span className="text-muted-foreground">
                {inLanguage(f.definition.intro, i18n.language, f.description)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function PortalHome() {
  const { t } = useTranslation();
  const types = useQuery(typesQuery);
  const offered = (types.data ?? []).filter((x) => x.portal);
  return (
    <div className="flex flex-col gap-6">
      <Forms />
      <h1 className="text-2xl font-semibold">{t("portal.mine")}</h1>
      {types.isSuccess && offered.length === 0 && <p>{t("portal.nothing")}</p>}
      {offered.map((type) => (
        <MyRecords key={qualifiedName(type)} type={type} />
      ))}
    </div>
  );
}

function usePortalType(typeName: string) {
  const types = useQuery(typesQuery);
  return {
    type: types.data?.find((x) => qualifiedName(x) === typeName && x.portal),
    pending: types.isPending,
  };
}

export function PortalNewPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { typeName } = useParams({ from: "/portal/$typeName/new" });
  const { type, pending } = usePortalType(typeName);
  const me = useQuery(meQuery);
  if (pending || me.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  const requester =
    me.data?.person_id && type.actors?.some((r) => r.name === "requester")
      ? { actors: { requester: [{ type: "core.person", id: me.data.person_id }] } }
      : undefined;
  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-2xl font-semibold">
        {t("portal.newTitle", { type: labelOf(type.label, i18n.language, typeName) })}
      </h1>
      <Form
        type={type}
        only={type.portal!.fields}
        withRoles={false}
        extra={requester}
        onSaved={(saved: RecordData) => {
          void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
          void navigate({
            to: "/portal/$typeName/$id",
            params: { typeName, id: String(saved["id"]) },
          });
        }}
        onCancel={() => void navigate({ to: "/portal" })}
      />
    </div>
  );
}

// formType is the form as the fields of a type: one field per question, named by the question
// id; a bound question is its field of the target type with the question's label, a free one a
// field of its own. The sections group them.
export function formType(form: FormOffer, target: TypeDefinition) {
  const fields: Field[] = [];
  for (const s of form.definition.sections) {
    for (const q of s.questions) {
      const own = both(q.label);
      if (q.field) {
        const f = target.fields.find((x) => x.name === q.field);
        if (f) {
          fields.push({
            ...f,
            name: q.id,
            label: own ?? f.label,
            description: both(q.description) ?? f.description,
            required: q.required === true || f.required === true,
          });
        }
      } else {
        fields.push({
          name: q.id,
          type: (q.type ?? "text") as Field["type"],
          label: own,
          description: both(q.description),
          required: q.required === true,
          options: q.options?.map((o) => ({ value: o.value, label: both(o.label) })),
        });
      }
    }
  }
  const type: TypeDefinition = { ...target, fields, actors: [], states: [], transitions: [] };
  return { type, sections: form.definition.sections };
}

export function PortalFormPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { formId } = useParams({ from: "/portal/forms/$formId" });
  const form = useQuery({
    queryKey: ["forms", formId],
    queryFn: () => api.form(formId),
    retry: false,
  });
  const types = useQuery(typesQuery);
  const [sent, setSent] = useState<RecordData | null>(null);
  if (form.isPending || types.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  const target = types.data?.find((x) => qualifiedName(x) === form.data?.target_type);
  if (!form.data || !target) {
    return <Alert variant="destructive">{t("portal.formUnknown")}</Alert>;
  }
  if (sent) {
    return (
      <Alert>
        <p>{t("portal.sent", { title: String(sent[target.title_field ?? "id"] ?? sent["id"]) })}</p>
        <Link to="/portal" className="underline">
          {t("portal.mine")}
        </Link>
      </Alert>
    );
  }
  const { type, sections } = formType(form.data, target);
  return (
    <div className="flex flex-col gap-4">
      <nav aria-label="breadcrumb" className="text-sm text-muted-foreground">
        <Link to="/portal" className="underline">
          {t("portal.title")}
        </Link>
      </nav>
      <h1 className="text-2xl font-semibold">
        {inLanguage(form.data.definition.title, i18n.language, form.data.name)}
      </h1>
      {form.data.description && (
        <p className="text-muted-foreground">
          {inLanguage(form.data.definition.intro, i18n.language, form.data.description)}
        </p>
      )}
      <Form
        type={type}
        withRoles={false}
        groups={sections.map((s) => ({
          key: s.id,
          title: labelOf(both(s.title), i18n.language, ""),
          fields: s.questions.map((q) => q.id),
        }))}
        submit={(answers) => api.submitForm(formId, answers)}
        visibility={(values) => hiddenOf(form.data.definition, values)}
        onSaved={(saved: RecordData) => {
          const typeName = qualifiedName(target);
          void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
          if (target.portal) {
            void navigate({
              to: "/portal/$typeName/$id",
              params: { typeName, id: String(saved["id"]) },
            });
          } else {
            // A type the portal does not show: the record exists, its page is not here.
            setSent(saved);
          }
        }}
        onCancel={() => void navigate({ to: "/portal" })}
      />
    </div>
  );
}

export function PortalRecordPage() {
  const { t, i18n } = useTranslation();
  const { typeName, id } = useParams({ from: "/portal/$typeName/$id" });
  const { type, pending } = usePortalType(typeName);
  const record = useQuery(recordQuery(typeName, id));
  if (pending || record.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  if (!type) {
    return <Alert variant="destructive">{t("type.unknown")}</Alert>;
  }
  if (!record.data) {
    return <Alert variant="destructive">{t("form.notFound")}</Alert>;
  }
  const rec = record.data;
  const title = String(rec[type.title_field ?? "id"] ?? id);
  const state = type.states?.find((s) => s.name === rec["state"]);
  const timeline = type.traits?.includes("timeline");
  return (
    <div className="flex flex-col gap-4">
      <nav aria-label="breadcrumb" className="text-sm text-muted-foreground">
        <Link to="/portal" className="underline">
          {t("portal.mine")}
        </Link>
      </nav>
      <h1 className="text-2xl font-semibold">{title}</h1>
      {state && (
        <p className="text-sm">
          {t("state.title")}: {labelOf(state.label, i18n.language, state.name)}
        </p>
      )}
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 rounded-md border bg-card p-4 text-sm">
        {type.portal!.fields.map((name) => {
          const f = type.fields.find((x) => x.name === name);
          return f ? (
            <div key={name} className="contents">
              <dt className="text-muted-foreground">{fieldLabel(f, i18n.language)}</dt>
              <dd>{formatValue(f, rec[name], i18n.language) || "–"}</dd>
            </div>
          ) : null;
        })}
      </dl>
      {/* The requester confirms or refuses the solution (SD-Plan N8). */}
      {type.transitions?.some((tr) => (tr.by?.length ?? 0) > 0) && (
        <Transitions type={type} record={rec} onlyRoles />
      )}
      {timeline && <Timeline type={type} id={id} portal />}
    </div>
  );
}
