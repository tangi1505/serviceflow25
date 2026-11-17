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
// create their target type; filling one in creates its record. Under the head a band shows
// what is going on, such as an open major incident, on every page of the portal (SD5). One's
// own records are one list over the types (UI43, MyRecords.tsx); above them the open questions
// of approval with their decision (UI44, OpenApprovals.tsx).
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Outlet, useNavigate, useParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  formsQuery,
  labelOf,
  meQuery,
  myBrandingQuery,
  mySettingsQuery,
  qualifiedName,
  readQuery,
  typesQuery,
  type PortalNotice,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { BrandMark, defaultName, useBranding } from "@/app/Brand";
import { AccountMenu } from "@/app/Header";
import { useContentLanguage } from "@/app/contentLanguage";
import { forget } from "@/app/recent";
import { Form } from "@/render/Form";

import { FormFill, inLanguage } from "./FormFill";
import { MyRecords } from "./MyRecords";
import { OpenApprovals } from "./OpenApprovals";
import { AreaNav, creates, FieldValue } from "./Read";
import { PortalSuggestions } from "./Suggest";
import { TileGrid, useTiles } from "./Tiles";
import { fieldLabel, formatMoment, setDisplayTimeZone } from "@/render/fields";
import { RichText } from "@/render/RichText";
import { Timeline } from "@/render/Timeline";
import { Transitions } from "@/render/Transitions";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { applyAccent, isAccentValue } from "@/ui/accent";
import { applyColorScheme, isColorScheme } from "@/ui/colorScheme";
import { applyTheme, isTheme } from "@/ui/theme";

export function PortalShell() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const settings = useQuery(mySettingsQuery);
  // The name, logo and main action of the organisation (UI39).
  const branding = useQuery({ ...myBrandingQuery, enabled: me.data !== undefined });
  useBranding(branding.data);
  useContentLanguage();
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
    // The accent of the person or the organisation (UI38, UI39: also a free colour).
    applyAccent(isAccentValue(s.accent?.value) ? s.accent.value : null);
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
      {/* The head over the full width (UI41, UI plan E43): brand, areas, the account. Below
          768 px the areas stand as a row to swipe under the brand. */}
      <header className="border-b bg-card">
        <div className="flex flex-wrap items-center gap-x-5 px-4 md:flex-nowrap md:px-6">
          <Link to="/portal" className="flex h-14 flex-none items-center gap-2.5 text-foreground">
            <BrandMark branding={branding.data} />
            <span className="font-semibold tracking-tight">
              {branding.data?.name ?? defaultName}
            </span>
            <span className="text-sm text-muted-foreground">{t("portal.title")}</span>
          </Link>
          <div className="order-last -mx-4 w-[calc(100%+2rem)] overflow-x-auto px-4 pb-2 md:order-none md:mx-0 md:w-auto md:min-w-0 md:flex-1 md:px-0 md:pb-0 [scrollbar-width:none]">
            <AreaNav />
          </div>
          <div className="ml-auto flex flex-none items-center gap-1">
            <AccountMenu portal onLogout={() => void logout()} />
          </div>
        </div>
      </header>
      <Notices />
      <main className="mx-auto w-full max-w-6xl flex-1 p-6">
        <Outlet />
      </main>
    </div>
  );
}

const noticesQuery = {
  queryKey: ["portal", "notices"],
  queryFn: api.portalNotices,
  staleTime: 60_000,
  refetchInterval: 60_000,
};

// Notices is the band "Aktuell" (SD5, variant A of the mockup): one line per record a notice
// of the tenant names, its message cut to one line until "Mehr" opens it.
function Notices() {
  const { t } = useTranslation();
  const notices = useQuery(noticesQuery);
  if (!notices.data || notices.data.length === 0) {
    return null;
  }
  return (
    <section
      aria-label={t("portal.current")}
      className="border-b border-warning-border bg-warning-background text-foreground"
    >
      <ul className="mx-auto flex w-full max-w-6xl flex-col gap-1 px-6 py-2">
        {notices.data.map((n) => (
          <Notice key={`${n.notice ?? ""}${n.id}`} notice={n} />
        ))}
      </ul>
    </section>
  );
}

function Notice({ notice }: { notice: PortalNotice }) {
  const { t, i18n } = useTranslation();
  const [open, setOpen] = useState(false);
  const textId = `notice-${notice.id}`;
  const state = notice.state_label
    ? labelOf(notice.state_label, i18n.language, notice.state ?? "")
    : notice.state;
  return (
    <li className="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-sm">
      {notice.title ? (
        // A notice written by hand (SD14): its title, its formatted text once opened.
        <div id={textId} className="min-w-0 flex-1">
          <p className={open ? "" : "truncate"}>
            <b className="font-semibold">{t("portal.current")}:</b> {notice.title}
          </p>
          {open && <RichText html={notice.text} className="mt-1" />}
        </div>
      ) : (
        <p id={textId} className={open ? "min-w-0 flex-1" : "min-w-0 flex-1 truncate"}>
          <b className="font-semibold">{t("portal.current")}:</b> {notice.text}
        </p>
      )}
      <span className="text-muted-foreground">
        {notice.until
          ? t("portal.until", { time: formatMoment(notice.until, i18n.language) })
          : t("portal.since", { time: formatMoment(notice.since, i18n.language) })}
        {state ? ` · ${state}` : ""}
      </span>
      <button
        type="button"
        className="underline"
        aria-expanded={open}
        aria-controls={textId}
        onClick={() => setOpen(!open)}
      >
        {open ? t("portal.less") : t("portal.more")}
      </button>
    </li>
  );
}

// formsQuery are the forms offered to the session (UI11).

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
  const types = useQuery(typesQuery);
  const offered = (types.data ?? []).filter(creates);
  // The tiles first (SD14, SD-Plan E13); they take the place of the card of the forms, which
  // only a person without any tile still gets. Below them one's own records as one list over
  // the types (UI43, UI plan E52); creating starts at a tile or a form.
  const tiles = useTiles();
  const withTiles = !tiles.pending && tiles.tiles.length > 0;
  return (
    <div className="flex flex-col gap-6">
      {withTiles && <TileGrid tiles={tiles.tiles} />}
      {/* The open questions of the person, only when there are some (UI44, UI plan E43). */}
      <OpenApprovals />
      {types.isSuccess && <MyRecords types={offered} heading={withTiles ? "h2" : "h1"} />}
      {!tiles.pending && !withTiles && <Forms />}
    </div>
  );
}

function usePortalType(typeName: string) {
  const types = useQuery(typesQuery);
  return {
    type: types.data?.find((x) => qualifiedName(x) === typeName && creates(x)),
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
  // What the form holds so far, for the suggestions beside it (SD18); a suggestion that
  // helped ends the report, the form stays behind it for "Doch melden".
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [helped, setHelped] = useState<string | null>(null);
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
      {helped !== null && (
        <div
          role="status"
          className="flex flex-col gap-2 rounded-md border border-success bg-card p-4"
        >
          <p className="font-semibold">{t("suggest.doneTitle")}</p>
          <p className="text-sm">{t("suggest.doneText", { title: helped })}</p>
          <div className="flex flex-wrap gap-2">
            <Link to="/portal" className="rounded-md border px-3 py-1.5 text-sm">
              {t("portal.mine")}
            </Link>
            <Button size="sm" variant="ghost" onClick={() => setHelped(null)}>
              {t("suggest.doneUndo")}
            </Button>
          </div>
        </div>
      )}
      <div
        hidden={helped !== null}
        className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]"
      >
        <Form
          type={type}
          only={type.portal!.fields!}
          withRoles={false}
          extra={requester}
          onValuesChange={(v) =>
            setValues((prev) => (JSON.stringify(prev) === JSON.stringify(v) ? prev : { ...v }))
          }
          onSaved={(saved: RecordData) => {
            void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
            void navigate({
              to: "/portal/$typeName/$id",
              params: { typeName, id: String(saved["id"]) },
            });
          }}
          onCancel={() => void navigate({ to: "/portal" })}
        />
        <PortalSuggestions type={type} values={values} onHelped={setHelped} />
      </div>
    </div>
  );
}

export function PortalFormPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { formId } = useParams({ from: "/portal/forms/$formId" });
  const [sent, setSent] = useState<{ saved: RecordData; target: TypeDefinition } | null>(null);
  if (sent) {
    const { saved, target } = sent;
    return (
      <Alert>
        <p>
          {t("portal.sent", { title: String(saved[target.title_field ?? "id"] ?? saved["id"]) })}
        </p>
        <Link to="/portal" className="underline">
          {t("portal.mine")}
        </Link>
      </Alert>
    );
  }
  return (
    <FormFill
      formId={formId}
      crumb={
        <Link to="/portal" className="underline">
          {t("portal.title")}
        </Link>
      }
      onSaved={(saved, target) => {
        const typeName = qualifiedName(target);
        void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
        if (creates(target)) {
          void navigate({
            to: "/portal/$typeName/$id",
            params: { typeName, id: String(saved["id"]) },
          });
        } else {
          // A type the portal does not show: the record exists, its page is not here.
          setSent({ saved, target });
        }
      }}
      onCancel={() => void navigate({ to: "/portal" })}
    />
  );
}

export function PortalRecordPage() {
  const { t, i18n } = useTranslation();
  const { typeName, id } = useParams({ from: "/portal/$typeName/$id" });
  const { type, pending } = usePortalType(typeName);
  const record = useQuery(readQuery(typeName, id));
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
        {type.portal!.fields!.map((name) => {
          const f = type.fields.find((x) => x.name === name);
          return f ? (
            <div key={name} className="contents">
              <dt className="text-muted-foreground">{fieldLabel(f, i18n.language)}</dt>
              <dd>
                <FieldValue field={f} value={rec[name]} />
              </dd>
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
