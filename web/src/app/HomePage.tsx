// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The start page of the workplace: a greeting, and below it the dashboard the person chose in
// "Meine Einstellungen" or the one set for her unit (UI13); "none" keeps the greeting alone.
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { meQuery, mySettingsQuery, versionQuery } from "@/api/client";
import { DashboardScreen } from "@/app/Dashboard";

export function HomePage() {
  const { t } = useTranslation();
  const me = useQuery(meQuery);
  const version = useQuery(versionQuery);
  const settings = useQuery(mySettingsQuery);
  const start = settings.data?.start_dashboard.value ?? "";
  return (
    <>
      <h1 className="text-2xl font-semibold">
        {t("home.welcome", { name: me.data?.display_name ?? "" })}
      </h1>
      {start === "" || start === "none" ? (
        <>
          <p className="mt-2 text-muted-foreground">{t("home.intro")}</p>
          <p className="mt-6 text-sm text-muted-foreground" aria-live="polite">
            {version.isPending && t("app.version.loading")}
            {version.data && t("app.version.value", { version: version.data.version })}
            {version.isError && t("app.version.error")}
          </p>
        </>
      ) : (
        <div className="mt-4">
          <DashboardScreen id={start} embedded />
        </div>
      )}
    </>
  );
}
