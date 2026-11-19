// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Suggestions at a record (SD18, SD-Plan E12): a type that names this one under suggest gets a
// tab of its own on the record page, labelled as it says ("Wissen"). The tab offers what the
// fields of the record find, and a search of one's own; each hit can be linked, and where the
// type says so used: its values go into the form, unsaved, and the hit is linked.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useId, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  labelOf,
  qualifiedName,
  type Rating,
  type RecordData,
  type SuggestGroup,
  type SuggestHit,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { confirm } from "@/ui/confirm";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { toast } from "@/ui/toast";

// suggesters are the types whose suggest names this type, by its name or by a trait it has,
// as the server decides it (meta.Suggest.Suggests): a trait of the type's own package may be
// written without the package.
export function suggesters(types: TypeDefinition[], type: TypeDefinition): TypeDefinition[] {
  const name = qualifiedName(type);
  const traits = type.traits ?? [];
  const reaches = (entry: string) => {
    if (!entry.startsWith("trait:")) {
      return entry === "*" || entry === name;
    }
    const trait = entry.slice("trait:".length);
    return (
      traits.includes(trait) ||
      (trait.startsWith(`${type.package}.`) &&
        traits.includes(trait.slice(`${type.package}.`.length)))
    );
  };
  return types.filter((t) => t.suggest?.for.some(reaches));
}

// marked shows a snippet of the search with its marks, as text: the snippet may hold pieces
// of formatted text, which must not become markup. Each piece is parsed inside an element, so
// that a space at its start stays (the parser drops it at the start of a body).
export function marked(snippet: string): ReactNode[] {
  const text = (html: string) =>
    (
      new DOMParser().parseFromString(`<div>${html}</div>`, "text/html").body.textContent ?? ""
    ).replace(/\s+/g, " ");
  return snippet
    .split(/(<mark>[\s\S]*?<\/mark>)/g)
    .filter((part) => part !== "")
    .map((part, i) =>
      part.startsWith("<mark>") ? (
        <mark key={i} className="bg-warning-background px-0.5 text-foreground">
          {text(part.slice("<mark>".length, -"</mark>".length))}
        </mark>
      ) : (
        <span key={i}>{text(part)}</span>
      ),
    );
}

// ratingLine says how a hit was rated and how often it was used.
export function useRatingLine() {
  const { t } = useTranslation();
  return (rating?: Rating, uses?: number) => {
    const parts: string[] = [];
    if (rating) {
      const total = rating.yes + rating.no;
      parts.push(
        total === 0 ? t("suggest.unrated") : t("suggest.rated", { yes: rating.yes, total }),
      );
    }
    if (uses !== undefined) {
      parts.push(t("suggest.uses", { count: uses }));
    }
    return parts.join(" · ");
  };
}

// applied are the values using a hit sets: "@<field>" the hit's field, else the value as the
// server resolved it (a select list's entry by its id).
export function appliedValues(set: Record<string, unknown>, hit: RecordData) {
  const out: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(set)) {
    out[name] = typeof value === "string" && value.startsWith("@") ? hit[value.slice(1)] : value;
  }
  return out;
}

export function SuggestionTab({
  type,
  record,
  from,
  onApply,
}: {
  type: TypeDefinition;
  record: RecordData;
  // from is the type whose records are suggested.
  from: TypeDefinition;
  onApply: (values: Record<string, unknown>) => void;
}) {
  const { t, i18n } = useTranslation();
  const formId = useId();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const fromName = qualifiedName(from);
  const id = String(record["id"]);
  const [draft, setDraft] = useState("");
  const [text, setText] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const line = useRatingLine();
  const values = Object.fromEntries((from.suggest?.from ?? []).map((f) => [f, record[f]]));
  const key = ["records", typeName, id, "suggest", fromName, text];
  const found = useQuery({
    queryKey: key,
    queryFn: () => api.suggest(typeName, { values, ...(text ? { text } : {}), id, limit: 5 }),
  });
  const group: SuggestGroup | undefined = found.data?.find((g) => g.type === fromName);
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["records", typeName, id] });
  };
  const fail = (err: unknown) =>
    setFailure(t("suggest.error", { detail: err instanceof Error ? err.message : String(err) }));
  const relate = (hit: SuggestHit) =>
    api.relate({
      kind: group!.relation!,
      from: { type: typeName, id },
      to: { type: fromName, id: hit.id },
    });
  const link = useMutation({
    mutationFn: relate,
    onMutate: () => setFailure(null),
    onSuccess: refresh,
    onError: fail,
  });
  const unlink = useMutation({
    mutationFn: (relation: string) => api.unrelate(relation),
    onMutate: () => setFailure(null),
    onSuccess: refresh,
    onError: fail,
  });
  const use = useMutation({
    mutationFn: async (hit: SuggestHit) => {
      // The form edits German (SD-Plan E10): the hit as written, not as read.
      const full = await api.get(fromName, hit.id);
      onApply(appliedValues(group!.apply!.set, full));
      if (group!.relation && !hit.linked) {
        await relate(hit);
      }
      return hit;
    },
    onMutate: () => {
      setFailure(null);
    },
    onSuccess: (hit) => {
      toast.success(t("suggest.applied", { title: hit.title ?? hit.id }));
      refresh();
    },
    onError: fail,
  });
  const fields = (from.suggest?.from ?? [])
    .map((f) => type.fields.find((x) => x.name === f))
    .filter((f) => f !== undefined)
    .map((f) => labelOf(f.label, i18n.language, f.name))
    .join(", ");
  return (
    <section aria-labelledby={`${formId}-h`} className="flex max-w-3xl flex-col gap-4">
      <h2 id={`${formId}-h`} className="sr-only">
        {labelOf(from.suggest?.label, i18n.language, fromName)}
      </h2>
      <p className="text-sm text-muted-foreground">
        {text ? t("suggest.ownSearch", { text }) : t("suggest.fromFields", { fields })}
      </p>
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {found.isPending && <p className="text-sm">{t("list.loading")}</p>}
      {found.isError && (
        <Alert variant="destructive">{t("suggest.error", { detail: String(found.error) })}</Alert>
      )}
      {group && group.hits.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("suggest.none")}</p>
      )}
      {group && group.hits.length > 0 && (
        <ul className="flex flex-col">
          {group.hits.map((hit) => (
            <li
              key={hit.id}
              className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border-soft py-2"
            >
              <div className="flex min-w-0 flex-[999_1_20rem] flex-col gap-0.5">
                <Link
                  to="/types/$typeName/$id"
                  params={{ typeName: fromName, id: hit.id }}
                  className="font-medium underline"
                >
                  {hit.title ?? hit.id}
                </Link>
                <span className="text-sm">{marked(hit.snippet)}</span>
                <span className="text-xs text-muted-foreground">{line(hit.rating, hit.uses)}</span>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {hit.linked ? (
                  <>
                    <span className="text-sm text-success">{t("suggest.linked")}</span>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={unlink.isPending}
                      aria-label={t("suggest.unlinkOne", { title: hit.title ?? hit.id })}
                      onClick={() =>
                        // The common question before the relation goes (UI64).
                        void confirm({
                          title: t("suggest.confirmUnlink", { title: hit.title ?? hit.id }),
                          action: t("relations.remove"),
                        }).then((yes) => yes && unlink.mutate(hit.linked!))
                      }
                    >
                      {t("relations.remove")}
                    </Button>
                  </>
                ) : (
                  group.relation && (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={link.isPending}
                      aria-label={t("suggest.linkOne", { title: hit.title ?? hit.id })}
                      onClick={() => link.mutate(hit)}
                    >
                      {t("relations.add")}
                    </Button>
                  )
                )}
                {group.apply && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={use.isPending}
                    aria-label={`${labelOf(group.apply.label, i18n.language, "")}: ${hit.title ?? hit.id}`}
                    onClick={() => use.mutate(hit)}
                  >
                    {labelOf(group.apply.label, i18n.language, t("suggest.apply"))}
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setText(draft.trim());
        }}
      >
        <div className="flex min-w-0 flex-[1_1_16rem] flex-col gap-1">
          <Label htmlFor={`${formId}-q`}>{t("suggest.searchOwn")}</Label>
          <Input
            id={`${formId}-q`}
            value={draft}
            maxLength={200}
            onChange={(e) => setDraft(e.target.value)}
          />
        </div>
        <Button type="submit" variant="outline">
          {t("suggest.search")}
        </Button>
        {text && (
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              setDraft("");
              setText("");
            }}
          >
            {t("suggest.backToFields")}
          </Button>
        )}
      </form>
    </section>
  );
}
