// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The relations of a record (SD4, SD-Plan E7, UI-Plan E38): a table of both directions, each
// row named by its kind (the label from the source, the inverse from the target), the type, the
// record with its number and state, and a button to remove it after asking. Above it the kind
// and the record to link, found over every type the kind allows. A kind may name a trait
// instead of a type ("trait:sd.ticket", N11).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  ApiError,
  api,
  labelOf,
  qualifiedName,
  readQuery,
  relationKindsQuery,
  relationsQuery,
  typesQuery,
  type ActorRef,
  type Relation,
  type RelationKind,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { StatePill } from "@/ui/marks";

import { formatValue, stateCategory } from "./fields";
import { RefPicker, searchable } from "./RefPicker";
import { titleOf } from "./RefName";
import { Select } from "@/ui/select";

const traitPrefix = "trait:";

// relates tells whether a side of a kind (types, "*" or traits) takes the type.
export function relates(side: string[], t: TypeDefinition): boolean {
  const name = qualifiedName(t);
  return side.some((a) => {
    if (a.startsWith(traitPrefix)) {
      const trait = a.slice(traitPrefix.length);
      const own =
        t.package && trait.startsWith(`${t.package}.`) ? trait.slice(t.package.length + 1) : "";
      return t.traits?.includes(trait) === true || (own !== "" && t.traits?.includes(own) === true);
    }
    return a === "*" || a === name;
  });
}

// A choice of the form: a kind with the direction this record stands in.
type Choice = { key: string; kind: RelationKind; out: boolean; label: string };

function choices(kinds: RelationKind[], type: TypeDefinition, lang: string): Choice[] {
  const out: Choice[] = [];
  for (const k of kinds) {
    const name = `${k.package}.${k.name}`;
    const label = labelOf(k.label, lang, name);
    const inverse = labelOf(k.inverse, lang, name);
    if (relates(k.from, type)) {
      out.push({ key: `${name}>`, kind: k, out: true, label });
    }
    // A kind that reads the same both ways (related) is offered once.
    if (relates(k.to, type) && !(relates(k.from, type) && label === inverse)) {
      out.push({ key: `${name}<`, kind: k, out: false, label: inverse });
    }
  }
  return out;
}

// targets are the types the other end may have; "*" means every type the search finds.
function targets(choice: Choice | undefined, types: TypeDefinition[]): string[] {
  if (!choice) {
    return [];
  }
  const side = choice.out ? choice.kind.to : choice.kind.from;
  return types
    .filter((t) => (side.includes("*") ? searchable(t) : relates(side, t)))
    .map(qualifiedName);
}

// Row is one relation: kind, type, the other record with number and state, and removing it
// after asking.
function Row({
  end,
  label,
  types,
  asking,
  removing,
  onAsk,
  onRemove,
  onCancel,
}: {
  end: ActorRef;
  label: string;
  types: TypeDefinition[];
  asking: boolean;
  removing: boolean;
  onAsk: () => void;
  onRemove: () => void;
  onCancel: () => void;
}) {
  const { t, i18n } = useTranslation();
  const rec = useQuery({ ...readQuery(end.type, end.id), retry: false });
  const type = types.find((x) => qualifiedName(x) === end.type);
  const number = type?.fields.find((f) => f.type === "sequence");
  const title = rec.data ? titleOf(type, rec.data) : end.id.slice(-6);
  const shown =
    number && rec.data?.[number.name] != null
      ? `${formatValue(number, rec.data[number.name], i18n.language)} · ${title}`
      : title;
  const state = type?.states?.find((s) => s.name === rec.data?.["state"]);
  return (
    <tr className="border-t">
      <td className="p-2">{label}</td>
      <td className="p-2">{labelOf(type?.label, i18n.language, end.type)}</td>
      <td className="p-2">
        <Link
          to="/types/$typeName/$id"
          params={{ typeName: end.type, id: end.id }}
          className="underline"
        >
          {shown}
        </Link>
      </td>
      <td className="p-2">
        {state && (
          <StatePill
            label={labelOf(state.label, i18n.language, state.name)}
            category={stateCategory(state)}
          />
        )}
      </td>
      <td className="p-2 text-right">
        {asking ? (
          <span className="inline-flex gap-2">
            <Button size="sm" variant="destructive" disabled={removing} onClick={onRemove}>
              {t("relations.remove")}
            </Button>
            <Button size="sm" variant="outline" onClick={onCancel}>
              {t("form.cancel")}
            </Button>
          </span>
        ) : (
          <Button
            size="sm"
            variant="ghost"
            aria-label={t("relations.removeOne", { kind: label, name: shown })}
            onClick={onAsk}
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.6"
              aria-hidden="true"
            >
              <path d="M3 3l8 8M11 3l-8 8" />
            </svg>
          </Button>
        )}
      </td>
    </tr>
  );
}

export function Relations({ type, id }: { type: TypeDefinition; id: string }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const formId = useId();
  const types = useQuery(typesQuery);
  const kinds = useQuery(relationKindsQuery);
  const list = useQuery(relationsQuery(typeName, id));
  const options = choices(kinds.data ?? [], type, i18n.language);
  const [chosen, setChosen] = useState("");
  const choice = options.find((c) => c.key === chosen) ?? options[0];
  const [other, setOther] = useState<ActorRef[]>([]);
  const [asking, setAsking] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const me = { type: typeName, id };
  const refresh = (end?: ActorRef) => {
    void queryClient.invalidateQueries({ queryKey: relationsQuery(typeName, id).queryKey });
    if (end) {
      void queryClient.invalidateQueries({ queryKey: relationsQuery(end.type, end.id).queryKey });
    }
  };
  const failed = (err: unknown) =>
    setFailure(
      t("relations.error", {
        detail: err instanceof ApiError ? (err.problem?.detail ?? err.message) : String(err),
      }),
    );
  const relate = useMutation({
    mutationFn: ({ c, end }: { c: Choice; end: ActorRef }) =>
      api.relate({
        kind: `${c.kind.package}.${c.kind.name}`,
        from: c.out ? me : end,
        to: c.out ? end : me,
      }),
    onMutate: () => setFailure(null),
    onSuccess: (_, { end }) => {
      setOther([]);
      refresh(end);
    },
    onError: failed,
  });
  const unrelate = useMutation({
    mutationFn: (r: Relation) => api.unrelate(r.id),
    onMutate: () => setFailure(null),
    onSuccess: (_, r) => {
      setAsking(null);
      refresh(r.from.id === id ? r.to : r.from);
    },
    onError: failed,
  });
  const kindOf = (name: string) => kinds.data?.find((k) => `${k.package}.${k.name}` === name);
  const rows = (list.data ?? []).map((r) => {
    const out = r.from.type === typeName && r.from.id === id;
    const k = kindOf(r.kind);
    return {
      r,
      end: out ? r.to : r.from,
      label: labelOf(out ? k?.label : k?.inverse, i18n.language, r.kind),
    };
  });

  return (
    <div className="flex flex-col gap-4">
      {options.length > 0 && (
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            const end = other[0];
            if (choice && end) {
              relate.mutate({ c: choice, end });
            }
          }}
        >
          <div className="flex flex-col gap-1">
            <Label htmlFor={`${formId}-kind`}>{t("relations.kind")}</Label>
            <Select
              id={`${formId}-kind`}
              value={choice?.key ?? ""}
              onChange={(v) => {
                setChosen(v);
                setOther([]);
              }}
              options={options.map((c) => ({ value: c.key, label: c.label }))}
            />
          </div>
          <div className="flex min-w-64 flex-1 flex-col gap-1">
            <Label htmlFor={`${formId}-record`}>{t("relations.record")}</Label>
            <RefPicker
              id={`${formId}-record`}
              key={choice?.key}
              targets={targets(choice, types.data ?? [])}
              value={other}
              onChange={setOther}
              single
            />
          </div>
          <Button type="submit" variant="outline" disabled={other.length === 0 || relate.isPending}>
            {t("relations.add")}
          </Button>
        </form>
      )}
      {failure && <Alert variant="destructive">{failure}</Alert>}
      {list.isPending && <p>{t("list.loading")}</p>}
      {list.isSuccess && rows.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("relations.empty")}</p>
      )}
      {rows.length > 0 && (
        <div className="overflow-x-auto rounded-md border">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-surface-subtle text-left text-xs uppercase text-foreground-2">
                <th className="p-2 font-medium">{t("relations.kind")}</th>
                <th className="p-2 font-medium">{t("relations.type")}</th>
                <th className="p-2 font-medium">{t("relations.record")}</th>
                <th className="p-2 font-medium">{t("relations.state")}</th>
                <th className="p-2 font-medium">
                  <span className="sr-only">{t("relations.remove")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.map(({ r, end, label }) => (
                <Row
                  key={r.id}
                  end={end}
                  label={label}
                  types={types.data ?? []}
                  asking={asking === r.id}
                  removing={unrelate.isPending}
                  onAsk={() => setAsking(r.id)}
                  onRemove={() => unrelate.mutate(r)}
                  onCancel={() => setAsking(null)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
