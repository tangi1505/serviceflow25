// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Who else has a record open (UI8, Hauptplan E130, E29): the page beats every 20 seconds and
// at once when its form gets or loses unsaved changes; it leaves when it closes. Whoever stops
// beating is gone after 60 seconds on the server.
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { api, type PresenceEntry } from "@/api/client";

export const beatEvery = 20_000;

export function usePresence(typeName: string, id: string, editing: boolean): PresenceEntry[] {
  const [others, setOthers] = useState<PresenceEntry[]>([]);
  useEffect(() => {
    let stop = false;
    const beat = async () => {
      try {
        const out = await api.beat(typeName, id, editing);
        if (!stop) {
          setOthers(out.others);
        }
      } catch {
        // No Valkey or no right: nobody shown.
      }
    };
    void beat();
    const timer = setInterval(() => void beat(), beatEvery);
    return () => {
      stop = true;
      clearInterval(timer);
    };
  }, [typeName, id, editing]);
  useEffect(
    () => () => {
      void api.leave(typeName, id).catch(() => undefined);
    },
    [typeName, id],
  );
  return others;
}

export function PresenceLine({ others }: { others: PresenceEntry[] }) {
  const { t } = useTranslation();
  return (
    <p role="status" aria-live="polite" className="text-sm text-muted-foreground">
      {others
        .map((o) => t(o.editing ? "live.editing" : "live.viewing", { name: o.name }))
        .join(" · ")}
    </p>
  );
}
