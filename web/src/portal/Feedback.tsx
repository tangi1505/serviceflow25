// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// What readers give back under a page of the portal (SD18, SD-Plan E12): whether it helped,
// yes or no, once and changeable (trait rating), and comments in its timeline, where a reader
// rule lets them comment (trait timeline). Private entries and the changes stay with the agents.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  api,
  ApiError,
  qualifiedName,
  timelineQuery,
  type Rating,
  type TypeDefinition,
} from "@/api/client";
import { formatMoment } from "@/render/fields";
import { RichEditor, RichText } from "@/render/RichEditor";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Label } from "@/ui/label";

const ratingQuery = (typeName: string, id: string) => ({
  queryKey: ["records", typeName, id, "rating"],
  queryFn: () => api.rating(typeName, id),
});

// mayComment says whether readers may comment on the type: a reader rule names comment.
export function mayComment(type: TypeDefinition): boolean {
  return (
    type.traits?.includes("timeline") === true &&
    (type.readers ?? []).some((r) => r.actions?.includes("comment"))
  );
}

export function RatingBox({ type, id }: { type: TypeDefinition; id: string }) {
  const { t } = useTranslation();
  const headId = useId();
  const typeName = qualifiedName(type);
  const queryClient = useQueryClient();
  const sums = useQuery(ratingQuery(typeName, id));
  const [failure, setFailure] = useState<string | null>(null);
  const [voted, setVoted] = useState(false);
  const rate = useMutation({
    mutationFn: (helpful: boolean) => api.rate(typeName, id, helpful),
    onMutate: () => setFailure(null),
    onSuccess: (sum: Rating) => {
      setVoted(true);
      queryClient.setQueryData(ratingQuery(typeName, id).queryKey, sum);
    },
    onError: (err) =>
      setFailure(t("rating.error", { detail: err instanceof Error ? err.message : String(err) })),
  });
  const sum = sums.data;
  const total = sum ? sum.yes + sum.no : 0;
  const line = !sum
    ? ""
    : `${voted ? `${t("rating.thanks")} ` : ""}${
        total === 0 ? t("rating.none") : t("rating.sum", { yes: sum.yes, total })
      }`;
  const choice = (helpful: boolean, label: string) => (
    <Button
      size="sm"
      variant={sum?.mine === helpful ? "default" : "outline"}
      aria-pressed={sum?.mine === helpful}
      disabled={rate.isPending}
      onClick={() => rate.mutate(helpful)}
    >
      {label}
    </Button>
  );
  return (
    <section aria-labelledby={headId} className="flex flex-col gap-2 rounded-md border bg-card p-4">
      <h2 id={headId} className="text-base font-semibold">
        {t("rating.question")}
      </h2>
      <div role="group" aria-labelledby={headId} className="flex flex-wrap items-center gap-2">
        {choice(true, t("rating.yes"))}
        {choice(false, t("rating.no"))}
        <span role="status" className="text-sm text-muted-foreground">
          {line}
        </span>
      </div>
      {failure && <Alert variant="destructive">{failure}</Alert>}
    </section>
  );
}

export function Comments({ type, id }: { type: TypeDefinition; id: string }) {
  const { t, i18n } = useTranslation();
  const formId = useId();
  const typeName = qualifiedName(type);
  const queryClient = useQueryClient();
  const items = useQuery(timelineQuery(typeName, id));
  const [body, setBody] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const add = useMutation({
    mutationFn: () => api.addEntry(typeName, id, body, false),
    onMutate: () => setFailure(null),
    onSuccess: () => {
      setBody("");
      void queryClient.invalidateQueries({ queryKey: timelineQuery(typeName, id).queryKey });
    },
    onError: (err) =>
      setFailure(
        err instanceof ApiError && err.status === 403
          ? t("comments.forbidden")
          : t("comments.error", { detail: err instanceof Error ? err.message : String(err) }),
      ),
  });
  // Public entries only, oldest first: a conversation under the page.
  const comments = (items.data ?? []).filter(
    (it) => it.kind === "entry" && it.entry && !it.entry.private,
  );
  return (
    <section aria-labelledby={`${formId}-h`} className="flex flex-col gap-3">
      <h2 id={`${formId}-h`} className="text-lg font-semibold">
        {t("comments.title")}
      </h2>
      {items.isSuccess && comments.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("comments.none")}</p>
      )}
      <ol className="flex flex-col gap-3">
        {comments.map((c) => (
          <li key={c.entry!.id} className="rounded-md border bg-card p-3">
            <p className="text-xs text-muted-foreground">
              <span className="font-medium text-foreground">{c.actor_name ?? ""}</span>
              {" · "}
              <time dateTime={c.at}>{formatMoment(c.at, i18n.language)}</time>
            </p>
            {c.entry!.format === "html" ? (
              <RichText html={c.entry!.body} className="mt-1" />
            ) : (
              <p className="mt-1 whitespace-pre-wrap text-sm">{c.entry!.body}</p>
            )}
          </li>
        ))}
      </ol>
      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (body.trim()) {
            add.mutate();
          }
        }}
      >
        <Label htmlFor={`${formId}-body`}>{t("comments.label")}</Label>
        <RichEditor
          id={`${formId}-body`}
          label={t("comments.label")}
          value={body}
          onChange={setBody}
        />
        <div className="flex flex-wrap items-center gap-3">
          <Button type="submit" size="sm" disabled={!body.trim() || add.isPending}>
            {t("comments.send")}
          </Button>
          <span className="text-sm text-muted-foreground">{t("comments.hint")}</span>
        </div>
        {failure && <Alert variant="destructive">{failure}</Alert>}
      </form>
    </section>
  );
}
