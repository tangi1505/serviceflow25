// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Sign-in with a local account (PL5, Hauptplan E75): username and password. A failed sign-in
// says only that it failed (the server does not tell which part was wrong). Below the form,
// one button per identity provider (PL11) sends the browser to the provider.
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearch } from "@tanstack/react-router";
import { useEffect, useId, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { BrandMark, defaultName, useBranding } from "@/app/Brand";
import { LanguageSwitch } from "@/app/Switches";
import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "@/ui/card";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { api, ApiError, brandingQuery, meQuery, providersQuery } from "@/api/client";
import { applyAccent } from "@/ui/accent";

const schema = z.object({
  username: z.string().trim().min(1, "required"),
  password: z.string().min(1, "required"),
});
type Credentials = z.infer<typeof schema>;

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const id = useId();
  const [failure, setFailure] = useState<string | null>(null);
  const providers = useQuery(providersQuery);
  // Before the sign-in no tenant holds: the appearance of the root (UI39, UI plan E51).
  const branding = useQuery(brandingQuery);
  useBranding(branding.data);
  useEffect(() => {
    if (branding.data) {
      applyAccent(branding.data.accent);
    }
  }, [branding.data]);
  const search = useSearch({ strict: false }) as { error?: string };
  const ssoError = search.error;
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<Credentials>({ resolver: zodResolver(schema) });

  const submit = async (credentials: Credentials) => {
    setFailure(null);
    try {
      const me = await api.login(credentials);
      queryClient.setQueryData(meQuery.queryKey, me);
      await navigate({ to: me.password_expired ? "/password" : "/" });
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setFailure(t("login.failed"));
      } else {
        setFailure(t("login.error", { detail: err instanceof Error ? err.message : String(err) }));
      }
    }
  };

  const fieldError = (name: keyof Credentials) =>
    errors[name] ? (
      <p id={`${id}-${name}-error`} className="text-sm text-destructive">
        {t("login.required")}
      </p>
    ) : null;

  return (
    <main className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>{t("login.title")}</CardTitle>
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <BrandMark branding={branding.data} />
            {branding.data?.name ?? defaultName}
          </p>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-4" onSubmit={handleSubmit(submit)} noValidate>
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${id}-username`}>{t("login.username")}</Label>
              <Input
                id={`${id}-username`}
                autoComplete="username"
                autoCapitalize="none"
                aria-invalid={errors.username ? true : undefined}
                aria-describedby={errors.username ? `${id}-username-error` : undefined}
                {...register("username")}
              />
              {fieldError("username")}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${id}-password`}>{t("login.password")}</Label>
              <Input
                id={`${id}-password`}
                type="password"
                autoComplete="current-password"
                aria-invalid={errors.password ? true : undefined}
                aria-describedby={errors.password ? `${id}-password-error` : undefined}
                {...register("password")}
              />
              {fieldError("password")}
            </div>
            {failure && <Alert variant="destructive">{failure}</Alert>}
            {ssoError && !failure && (
              <Alert variant="destructive">
                {t(`login.sso.${ssoError}`, { defaultValue: t("login.sso.sso") })}
              </Alert>
            )}
            <Button type="submit" disabled={isSubmitting}>
              {t("login.submit")}
            </Button>
          </form>
          {providers.data && providers.data.length > 0 && (
            <div className="mt-4 flex flex-col gap-2">
              {providers.data.map((p) => (
                <Button
                  key={p.name}
                  type="button"
                  variant="outline"
                  onClick={() => {
                    window.location.assign(p.start_url);
                  }}
                >
                  {t("login.with", { name: p.name })}
                </Button>
              ))}
            </div>
          )}
        </CardContent>
        <CardFooter className="flex-wrap">
          <LanguageSwitch />
        </CardFooter>
      </Card>
    </main>
  );
}
