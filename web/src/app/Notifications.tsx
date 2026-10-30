// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Notifications in the browser (UI8, Hauptplan E130): after the person allows them, an event
// of a record where they or their group hold a role, caused by someone else, shows as a
// notification while the tab is not in view. Only while a tab is open; no push service from
// outside (VS-NfD). A click opens the record.
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { useTranslation } from "react-i18next";

import { labelOf, meQuery, qualifiedName, typesQuery } from "@/api/client";
import { Button } from "@/ui/button";

import { useLive } from "./live";

function supported(): boolean {
  return typeof window !== "undefined" && "Notification" in window;
}

export function Notifications() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const me = useQuery(meQuery);
  const types = useQuery(typesQuery);
  const [permission, setPermission] = useState(() =>
    supported() ? Notification.permission : "denied",
  );

  useLive((e) => {
    if (!supported() || Notification.permission !== "granted" || !e.mine) {
      return;
    }
    if (e.actor === me.data?.id || document.visibilityState === "visible") {
      return;
    }
    const type = types.data?.find((x) => qualifiedName(x) === e.type_name);
    const n = new Notification(labelOf(type?.label, i18n.language, e.type_name), {
      body: t(`live.kind.${e.kind}`, { defaultValue: e.kind }),
      tag: `${e.type_name}:${e.record_id}`,
    });
    n.onclick = () => {
      window.focus();
      void navigate({
        to: "/types/$typeName/$id",
        params: { typeName: e.type_name, id: e.record_id },
      });
      n.close();
    };
  });

  if (permission !== "default") {
    return null;
  }
  return (
    <Button
      variant="outline"
      size="sm"
      onClick={() => void Notification.requestPermission().then(setPermission)}
    >
      {t("live.allow")}
    </Button>
  );
}
