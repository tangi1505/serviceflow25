// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Deleting one record (UI62, UI-Plan E61, E62): after the common question (UI65) it goes to the
// trash; the message at the bottom right says so and offers "Rückgängig", which takes it out
// again. A restore that
// meets a unique value taken in the meantime (Kern-Plan E9) gives a red message with the reason
// of the server.

import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";

import { api, type TypeDefinition } from "@/api/client";
import { confirm } from "@/ui/confirm";
import { toast } from "@/ui/toast";

// mayDelete tells whether the session may move records of the type to the trash somewhere
// (Kern-Plan E79); the server still checks the record.
export function mayDelete(type: TypeDefinition): boolean {
  return type.may?.includes("delete") === true;
}

// useTrash gives a function that moves a record to the trash and reports it; it resolves to
// true when the record went. after runs then, before the message (the detail page goes back
// to its list).
export function useTrash() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const reload = (typeName: string) =>
    void queryClient.invalidateQueries({ queryKey: ["records", typeName] });
  return async (typeName: string, id: string, title: string, after?: () => void) => {
    const yes = await confirm({
      title: t("trash.ask", { title }),
      text: t("trash.askText"),
      action: t("trash.delete"),
    });
    if (!yes) {
      return false;
    }
    try {
      await api.trash(typeName, id);
    } catch (err) {
      toast.error(t("trash.failed", { title }), err instanceof Error ? err.message : String(err));
      return false;
    }
    after?.();
    reload(typeName);
    void queryClient.invalidateQueries({ queryKey: ["trash"] });
    toast.success(t("trash.done"), title, {
      action: {
        label: t("batch.undo"),
        onClick: () =>
          void (async () => {
            try {
              await api.restore(typeName, id);
              toast.success(t("trash.restored"), title);
            } catch (err) {
              toast.error(
                t("trash.restoreFailed", { title }),
                err instanceof Error ? err.message : String(err),
              );
            } finally {
              reload(typeName);
              void queryClient.invalidateQueries({ queryKey: ["trash"] });
            }
          })(),
      },
    });
    return true;
  };
}
