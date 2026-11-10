// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The substitutes of a person at approvals (WF6, picture 4 of the mockup): under "Meine
// Einstellungen" her own, at the person record for the administration. While a substitution
// lasts the substitute agrees or refuses for her, also for her groups; "bis" left empty means
// for good.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { api, ApiError, type ActorRef } from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

import { formatMoment } from "./fields";
import { RefName } from "./RefName";
import { RefPicker } from "./RefPicker";

const detail = (err: unknown) =>
  err instanceof ApiError ? (err.problem?.detail ?? err.message) : String(err);

// moment turns the value of a datetime-local input into RFC 3339, empty into undefined.
const moment = (v: string) => (v ? new Date(v).toISOString() : undefined);

export function Substitutes({ personId }: { personId: string }) {
  const { t, i18n } = useTranslation();
  const base = useId();
  const queryClient = useQueryClient();
  const key = ["persons", personId, "substitutes"];
  const list = useQuery({ queryKey: key, queryFn: () => api.substitutes(personId), retry: false });
  const [who, setWho] = useState<ActorRef[]>([]);
  const [from, setFrom] = useState("");
  const [until, setUntil] = useState("");
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: key });
    void queryClient.invalidateQueries({ queryKey: ["approvals"] });
  };
  const add = useMutation({
    mutationFn: () =>
      api.addSubstitute(personId, {
        substitute: who[0]!.id,
        valid_from: moment(from),
        valid_until: moment(until),
      }),
    onSuccess: () => {
      setWho([]);
      setFrom("");
      setUntil("");
      refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.removeSubstitute(personId, id),
    onSuccess: refresh,
  });
  return (
    <section
      aria-labelledby={`${base}-h`}
      className="flex flex-col gap-3 rounded-md border bg-card p-4"
    >
      <div className="flex flex-col gap-1">
        <h2 id={`${base}-h`} className="text-[15px] font-semibold">
          {t("substitutes.title")}
        </h2>
        <p className="text-[12.5px] text-muted-foreground">{t("substitutes.hint")}</p>
      </div>
      {list.isError && (
        <Alert variant="destructive">
          {t("substitutes.error", { detail: detail(list.error) })}
        </Alert>
      )}
      {list.data && list.data.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("substitutes.none")}</p>
      )}
      {list.data && list.data.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-[11.5px] uppercase tracking-wide text-muted-foreground">
                <th className="px-2 py-1 font-medium">{t("substitutes.who")}</th>
                <th className="px-2 py-1 font-medium">{t("substitutes.from")}</th>
                <th className="px-2 py-1 font-medium">{t("substitutes.until")}</th>
                <th className="px-2 py-1 font-medium">{t("substitutes.now")}</th>
                <th className="px-2 py-1">
                  <span className="sr-only">{t("substitutes.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {list.data.map((x) => (
                <tr key={x.id} className="border-t border-border-soft">
                  <td className="px-2 py-1.5">
                    <RefName refTo={{ type: "core.person", id: x.substitute }} />
                  </td>
                  <td className="px-2 py-1.5 font-mono text-[12.5px]">
                    {formatMoment(x.valid_from, i18n.language)}
                  </td>
                  <td className="px-2 py-1.5 font-mono text-[12.5px]">
                    {x.valid_until
                      ? formatMoment(x.valid_until, i18n.language)
                      : t("substitutes.forGood")}
                  </td>
                  <td
                    className={
                      "px-2 py-1.5 " + (x.active ? "text-success" : "text-muted-foreground")
                    }
                  >
                    {x.active ? t("substitutes.active") : t("substitutes.inactive")}
                  </td>
                  <td className="px-2 py-1.5">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(x.id)}
                    >
                      {t("substitutes.remove")}
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <form
        aria-label={t("substitutes.add")}
        className="flex flex-wrap items-end gap-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (who.length === 1) {
            add.mutate();
          }
        }}
      >
        <div className="flex min-w-60 flex-1 flex-col gap-1">
          <Label htmlFor={`${base}-who`}>{t("substitutes.who")}</Label>
          <RefPicker
            id={`${base}-who`}
            targets={["core.person"]}
            value={who}
            onChange={setWho}
            single
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${base}-from`}>{t("substitutes.from")}</Label>
          <Input
            id={`${base}-from`}
            type="datetime-local"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${base}-until`}>
            {t("substitutes.until")}{" "}
            <span className="font-normal text-muted-foreground">{t("substitutes.untilHint")}</span>
          </Label>
          <Input
            id={`${base}-until`}
            type="datetime-local"
            value={until}
            onChange={(e) => setUntil(e.target.value)}
          />
        </div>
        <Button type="submit" variant="outline" disabled={who.length !== 1 || add.isPending}>
          {t("substitutes.add")}
        </Button>
      </form>
      {add.isError && (
        <Alert variant="destructive">{t("substitutes.error", { detail: detail(add.error) })}</Alert>
      )}
      {remove.isError && (
        <Alert variant="destructive">
          {t("substitutes.error", { detail: detail(remove.error) })}
        </Alert>
      )}
    </section>
  );
}
