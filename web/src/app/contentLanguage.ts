// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Content follows the language of the person (SD16c): when it changes, the records and hits
// read so far are read again, so that lists, names and the portal show the other translation.
import { useQueryClient } from "@tanstack/react-query";
import i18n from "i18next";
import { useEffect } from "react";

export function useContentLanguage(): void {
  const queryClient = useQueryClient();
  useEffect(() => {
    const changed = () =>
      void queryClient.invalidateQueries({
        predicate: (q) => q.queryKey[0] === "records" || q.queryKey[0] === "search",
      });
    i18n.on("languageChanged", changed);
    return () => i18n.off("languageChanged", changed);
  }, [queryClient]);
}
