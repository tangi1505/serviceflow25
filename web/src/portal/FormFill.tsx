// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Filling in a form (UI11, UI12; SD9): its questions as the fields of a type in their sections,
// with their conditions; in the portal, in the workplace, and on the page of an offer of the
// catalog to order it (SD13).
import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  typesQuery,
  type Field,
  type FormOffer,
  type Label,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Form } from "@/render/Form";
import { Alert } from "@/ui/alert";

import { hiddenOf } from "./conditions";

// inLanguage is a text of a form in the language, else the German one (UI12).
export function inLanguage(label: Label | undefined, language: string, german: string): string {
  return (language !== "de" && label?.[language]) || german;
}

// both is a label of a form for labelOf: English falls back to German (UI12).
export function both(label: Label | undefined): Label | undefined {
  if (!label || !Object.values(label).some((v) => v)) {
    return undefined;
  }
  const de = label["de"] ?? "";
  return { ...label, de, en: label["en"] || de };
}

// formType is the form as the fields of a type: one field per question, named by the question
// id; a bound question is its field of the target type with the question's label, a role
// question a reference to the role's holders with search (SD9), a free one a field of its own.
// The sections group them.
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
      } else if (q.role) {
        const role = target.actors?.find((a) => a.name === q.role);
        if (role) {
          fields.push({
            name: q.id,
            type: "reference",
            to: role.to[0],
            label: own ?? role.label,
            description: both(q.description),
            required: q.required === true,
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

// FormFill is a form to fill in, in the portal and in the workplace (SD9): the questions as
// the fields of a type in their sections, with their conditions.
export function FormFill({
  formId,
  crumb,
  heading,
  submit,
  onSaved,
  onCancel,
}: {
  formId: string;
  crumb?: ReactNode;
  // heading puts the form in a section of a page under this heading, without the form's own
  // title: the order of an offer (SD13).
  heading?: string;
  // submit replaces submitting the form, e.g. ordering the offer with it (SD13).
  submit?: (answers: Record<string, unknown>) => Promise<RecordData>;
  onSaved: (saved: RecordData, target: TypeDefinition) => void;
  onCancel: () => void;
}) {
  const { t, i18n } = useTranslation();
  const form = useQuery({
    queryKey: ["forms", formId],
    queryFn: () => api.form(formId),
    retry: false,
  });
  const types = useQuery(typesQuery);
  if (form.isPending || types.isPending) {
    return <p>{t("list.loading")}</p>;
  }
  const target = types.data?.find((x) => qualifiedName(x) === form.data?.target_type);
  if (!form.data || !target) {
    return <Alert variant="destructive">{t("portal.formUnknown")}</Alert>;
  }
  const { type, sections } = formType(form.data, target);
  const fill = (
    <Form
      type={type}
      withRoles={false}
      groups={sections.map((s) => ({
        key: s.id,
        title: labelOf(both(s.title), i18n.language, ""),
        fields: s.questions.map((q) => q.id),
      }))}
      submit={submit ?? ((answers) => api.submitForm(formId, answers))}
      visibility={(values) => hiddenOf(form.data.definition, values)}
      onSaved={(saved: RecordData) => onSaved(saved, target)}
      onCancel={onCancel}
      sendLabel={heading}
    />
  );
  if (heading) {
    return (
      <section
        aria-labelledby="fill-heading"
        className="flex flex-col gap-3 rounded-md border bg-card p-4"
      >
        <h2 id="fill-heading" className="text-lg font-semibold">
          {heading}
        </h2>
        {fill}
      </section>
    );
  }
  return (
    <div className="flex flex-col gap-4">
      {crumb && (
        <nav aria-label="breadcrumb" className="text-sm text-muted-foreground">
          {crumb}
        </nav>
      )}
      <h1 className="text-2xl font-semibold">
        {inLanguage(form.data.definition.title, i18n.language, form.data.name)}
      </h1>
      {form.data.description && (
        <p className="text-muted-foreground">
          {inLanguage(form.data.definition.intro, i18n.language, form.data.description)}
        </p>
      )}
      {fill}
    </div>
  );
}
