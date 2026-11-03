// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The state of a record and its transitions (WF1): one button per transition of the type;
// those that do not apply are disabled and say why. A click moves the record and refreshes it.
// A transition only a role may take shows only to who holds it; one that takes a text asks
// for it first (SD-Plan N8). The portal shows only the transitions of the person's roles.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  labelOf,
  qualifiedName,
  recordQuery,
  transitionsQuery,
  type RecordData,
  type TypeDefinition,
} from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";
import { StatePill } from "@/ui/marks";

import { stateCategory } from "./fields";

export function Transitions({
  type,
  record,
  onlyRoles = false,
}: {
  type: TypeDefinition;
  record: RecordData;
  // onlyRoles shows only the transitions of the person's roles, as in the portal.
  onlyRoles?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const id = String(record["id"]);
  const infos = useQuery(transitionsQuery(typeName, id));
  const [failure, setFailure] = useState<string | null>(null);
  // asking is the transition that waits for its text.
  const [asking, setAsking] = useState<string | null>(null);
  const [text, setText] = useState("");

  const move = useMutation({
    mutationFn: ({ name, comment }: { name: string; comment?: string }) =>
      api.transition(typeName, id, name, comment),
    onMutate: () => setFailure(null),
    onSuccess: (saved) => {
      queryClient.setQueryData(recordQuery(typeName, id).queryKey, saved);
      // Field states, transitions, timeline and clocks follow the new state (UI3).
      void queryClient.invalidateQueries({
        queryKey: ["records", typeName, id],
        predicate: (q) => q.queryKey.length > 3,
      });
      void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
      setAsking(null);
      setText("");
    },
    onError: (err) => {
      setFailure(
        err instanceof ApiError && err.status === 409
          ? t("state.refused", { detail: err.problem?.detail ?? "" })
          : t("state.error", { detail: err instanceof Error ? err.message : String(err) }),
      );
      void queryClient.invalidateQueries({ queryKey: transitionsQuery(typeName, id).queryKey });
    },
  });

  if (!type.states || type.states.length === 0) {
    return null;
  }
  const state = type.states.find((s) => s.name === record["state"]);
  const stateLabel = state
    ? labelOf(state.label, i18n.language, state.name)
    : String(record["state"] ?? "");
  // The transition of a role one does not hold is not one's own: it does not show.
  const shown = (infos.data ?? []).filter((info) =>
    onlyRoles ? (info.by?.length ?? 0) > 0 && info.allowed : info.reason !== "role",
  );
  if (onlyRoles && shown.length === 0) {
    return null;
  }
  const ask = shown.find((info) => info.name === asking);

  return (
    <section aria-labelledby="state-heading" className="rounded-md border bg-card p-4">
      <h2 id="state-heading" className="text-sm font-semibold uppercase text-foreground-2">
        {onlyRoles ? t("state.yours") : t("state.title")}
      </h2>
      {!onlyRoles && (
        <p className="mt-2">
          <StatePill label={stateLabel} category={stateCategory(state)} />
        </p>
      )}
      <div className="mt-3 flex flex-wrap gap-2">
        {shown.map((info) => (
          <Button
            key={info.name}
            variant={info.allowed ? "default" : "outline"}
            size="sm"
            disabled={!info.allowed || move.isPending}
            aria-expanded={info.comment ? asking === info.name : undefined}
            title={info.allowed ? undefined : t(`state.reason.${info.reason ?? "state"}`)}
            onClick={() => (info.comment ? setAsking(info.name) : move.mutate({ name: info.name }))}
          >
            {labelOf(info.label, i18n.language, info.name)}
          </Button>
        ))}
      </div>
      {ask && (
        <form
          className="mt-3 flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            move.mutate({ name: ask.name, comment: text.trim() });
          }}
        >
          <Label htmlFor="transition-comment">
            {ask.comment === "required" ? t("state.commentRequired") : t("state.comment")}
          </Label>
          <textarea
            id="transition-comment"
            className="min-h-20 rounded-sm border border-input bg-card px-2.5 py-1.5 text-sm"
            required={ask.comment === "required"}
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <div className="flex gap-2">
            <Button
              type="submit"
              size="sm"
              disabled={move.isPending || (ask.comment === "required" && text.trim() === "")}
            >
              {labelOf(ask.label, i18n.language, ask.name)}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setAsking(null)}>
              {t("form.cancel")}
            </Button>
          </div>
        </form>
      )}
      {failure && (
        <Alert variant="destructive" className="mt-3">
          {failure}
        </Alert>
      )}
    </section>
  );
}
