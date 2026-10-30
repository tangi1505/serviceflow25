// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Change the own password; with an expired password this is the only page that works until
// the change is done (Hauptplan E77). The rules of E73 are checked here for a quick answer and
// again by the server.
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useId, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";

import { Alert } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/ui/card";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { api, ApiError, meQuery } from "@/api/client";

const strong = (pw: string) =>
  pw.length >= 8 &&
  /[A-Z]/.test(pw) &&
  /[a-z]/.test(pw) &&
  /[0-9]/.test(pw) &&
  /[^A-Za-z0-9]/.test(pw);

const schema = z
  .object({
    current_password: z.string().min(1, "required"),
    new_password: z.string().refine(strong, "weak"),
    confirm: z.string(),
  })
  .refine((v) => v.new_password === v.confirm, { path: ["confirm"], message: "mismatch" });
type Change = z.infer<typeof schema>;

export function PasswordPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const me = useQuery(meQuery);
  const id = useId();
  const [failure, setFailure] = useState<string | null>(null);
  const {
    register,
    handleSubmit,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<Change>({ resolver: zodResolver(schema) });

  const submit = async (change: Change) => {
    setFailure(null);
    try {
      await api.changePassword({
        current_password: change.current_password,
        new_password: change.new_password,
      });
      await queryClient.invalidateQueries({ queryKey: meQuery.queryKey });
      await navigate({ to: "/" });
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        const field = err.fieldErrors[0]?.field;
        if (field === "current_password") {
          setError("current_password", { message: "wrong" });
          return;
        }
        if (field === "new_password") {
          setError("new_password", { message: "weak" });
          return;
        }
      }
      setFailure(t("password.error", { detail: err instanceof Error ? err.message : String(err) }));
    }
  };

  const message = (name: keyof Change) => {
    const code = errors[name]?.message;
    if (!code) {
      return null;
    }
    return (
      <p id={`${id}-${name}-error`} className="text-sm text-destructive">
        {t(`password.${code}`)}
      </p>
    );
  };
  const field = (name: keyof Change, label: string, autoComplete: string) => (
    <div className="flex flex-col gap-2">
      <Label htmlFor={`${id}-${name}`}>{label}</Label>
      <Input
        id={`${id}-${name}`}
        type="password"
        autoComplete={autoComplete}
        aria-invalid={errors[name] ? true : undefined}
        aria-describedby={errors[name] ? `${id}-${name}-error` : undefined}
        {...register(name)}
      />
      {message(name)}
    </div>
  );

  return (
    <Card className="max-w-md">
      <CardHeader>
        <CardTitle>{t("password.title")}</CardTitle>
        {me.data?.password_expired && <Alert variant="destructive">{t("password.expired")}</Alert>}
        <p className="text-sm text-muted-foreground">{t("password.rules")}</p>
      </CardHeader>
      <CardContent>
        <form className="flex flex-col gap-4" onSubmit={handleSubmit(submit)} noValidate>
          {field("current_password", t("password.current"), "current-password")}
          {field("new_password", t("password.new"), "new-password")}
          {field("confirm", t("password.confirm"), "new-password")}
          {failure && <Alert variant="destructive">{failure}</Alert>}
          <Button type="submit" disabled={isSubmitting}>
            {t("password.submit")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}
