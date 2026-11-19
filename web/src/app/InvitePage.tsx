// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The page of an invitation (UI66, Kern-Plan PL77): the link of the mail leads here with its
// key; the person sets her password twice and then signs in. Without a session, like /login.

import { useQuery } from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { BrandMark, defaultName, useBranding } from "@/app/Brand";
import { api, ApiError, brandingQuery } from "@/api/client";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/ui/card";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";

export function InvitePage() {
  const { t } = useTranslation();
  const id = useId();
  const { key } = useSearch({ from: "/invite" });
  const branding = useQuery(brandingQuery);
  useBranding(branding.data);
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState<string | null>(null);

  const submit = async () => {
    setFailure(null);
    if (password !== again) {
      setFailure(t("invite.mismatch"));
      return;
    }
    setBusy(true);
    try {
      const r = await api.acceptInvitation(key ?? "", password);
      setDone(r.username);
    } catch (err) {
      const field = err instanceof ApiError ? err.fieldErrors[0] : undefined;
      setFailure(
        field?.field === "password"
          ? t("invite.weak", { detail: field.message })
          : field?.field === "key"
            ? t("invite.invalid")
            : t("invite.error"),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t("invite.title")}</CardTitle>
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <BrandMark branding={branding.data} />
            {branding.data?.name ?? defaultName}
          </p>
        </CardHeader>
        <CardContent>
          {done ? (
            <div className="flex flex-col gap-4">
              <Alert>{t("invite.done", { username: done })}</Alert>
              <Button asChild>
                <Link to="/login">{t("invite.toLogin")}</Link>
              </Button>
            </div>
          ) : !key ? (
            <Alert variant="destructive">{t("invite.invalid")}</Alert>
          ) : (
            <form
              className="flex flex-col gap-4"
              onSubmit={(e) => {
                e.preventDefault();
                void submit();
              }}
            >
              <p className="text-sm text-foreground-2">{t("invite.hint")}</p>
              <div className="flex flex-col gap-2">
                <Label htmlFor={`${id}-pw`}>{t("invite.password")}</Label>
                <Input
                  id={`${id}-pw`}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor={`${id}-again`}>{t("invite.again")}</Label>
                <Input
                  id={`${id}-again`}
                  type="password"
                  autoComplete="new-password"
                  value={again}
                  onChange={(e) => setAgain(e.target.value)}
                />
              </div>
              {failure && <Alert variant="destructive">{failure}</Alert>}
              <Button type="submit" disabled={busy || password === ""}>
                {t("invite.submit")}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
