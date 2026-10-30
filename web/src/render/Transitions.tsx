// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The state of a record and its transitions (WF1): one button per transition of the type;
// those that do not apply are disabled and say why. A click moves the record and refreshes it.
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

export function Transitions({ type, record }: { type: TypeDefinition; record: RecordData }) {
  const { t, i18n } = useTranslation();
  const queryClient = useQueryClient();
  const typeName = qualifiedName(type);
  const id = String(record["id"]);
  const infos = useQuery(transitionsQuery(typeName, id));
  const [failure, setFailure] = useState<string | null>(null);

  const move = useMutation({
    mutationFn: (name: string) => api.transition(typeName, id, name),
    onMutate: () => setFailure(null),
    onSuccess: (saved) => {
      queryClient.setQueryData(recordQuery(typeName, id).queryKey, saved);
      // Field states, transitions, timeline and clocks follow the new state (UI3).
      void queryClient.invalidateQueries({
        queryKey: ["records", typeName, id],
        predicate: (q) => q.queryKey.length > 3,
      });
      void queryClient.invalidateQueries({ queryKey: ["records", typeName, "list"] });
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

  return (
    <section aria-labelledby="state-heading" className="rounded-md border bg-card p-4">
      <h2 id="state-heading" className="text-sm font-semibold uppercase text-muted-foreground">
        {t("state.title")}
      </h2>
      <p className="mt-1 text-lg">{stateLabel}</p>
      <div className="mt-3 flex flex-wrap gap-2">
        {(infos.data ?? []).map((info) => (
          <Button
            key={info.name}
            variant={info.allowed ? "default" : "outline"}
            size="sm"
            disabled={!info.allowed || move.isPending}
            title={info.allowed ? undefined : t(`state.reason.${info.reason ?? "state"}`)}
            onClick={() => move.mutate(info.name)}
          >
            {labelOf(info.label, i18n.language, info.name)}
          </Button>
        ))}
      </div>
      {failure && (
        <Alert variant="destructive" className="mt-3">
          {failure}
        </Alert>
      )}
    </section>
  );
}
