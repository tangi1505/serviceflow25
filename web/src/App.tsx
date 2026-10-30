// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

type VersionState = { kind: "loading" } | { kind: "ok"; version: string } | { kind: "error" };

// The empty shell of the web interface: it proves that build, i18n and the API connection work.
export function App() {
  const { t } = useTranslation();
  const [state, setState] = useState<VersionState>({ kind: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    fetch("/api/v1/version", { signal: controller.signal })
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(r.statusText))))
      .then((body: { version: string }) => setState({ kind: "ok", version: body.version }))
      .catch((err: unknown) => {
        if (!controller.signal.aborted) {
          console.error(err);
          setState({ kind: "error" });
        }
      });
    return () => controller.abort();
  }, []);

  return (
    <main>
      <h1>{t("app.title")}</h1>
      <p aria-live="polite">
        {state.kind === "loading" && t("app.version.loading")}
        {state.kind === "ok" && t("app.version.value", { version: state.version })}
        {state.kind === "error" && t("app.version.error")}
      </p>
    </main>
  );
}
