// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { meQuery, versionQuery } from "@/api/client";

export function HomePage() {
  const { t } = useTranslation();
  const me = useQuery(meQuery);
  const version = useQuery(versionQuery);
  return (
    <>
      <h1 className="text-2xl font-semibold">
        {t("home.welcome", { name: me.data?.display_name ?? "" })}
      </h1>
      <p className="mt-2 text-muted-foreground">{t("home.intro")}</p>
      <p className="mt-6 text-sm text-muted-foreground" aria-live="polite">
        {version.isPending && t("app.version.loading")}
        {version.data && t("app.version.value", { version: version.data.version })}
        {version.isError && t("app.version.error")}
      </p>
    </>
  );
}
