// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The export of a list (UI5, Hauptplan E125): the format, then the file with the condition,
// columns and sort the list has now. Up to 100,000 rows the browser saves it at once; a
// larger one runs as a job, and the page offers the file when it is done.
import { useEffect, useId, useState } from "react";
import { useTranslation } from "react-i18next";

import { api, ApiError } from "@/api/client";
import { Button } from "@/ui/button";

import { currentTimeZone } from "./fields";

type Format = "csv" | "xlsx" | "ods";

// save hands a Blob to the browser as a download.
function save(file: Blob, name: string) {
  const url = URL.createObjectURL(file);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function ExportButton({
  typeName,
  where,
  columns,
  sort,
}: {
  typeName: string;
  where?: Record<string, unknown>;
  columns: string[];
  sort: string;
}) {
  const { t, i18n } = useTranslation();
  const id = useId();
  const [format, setFormat] = useState<Format>("xlsx");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [jobId, setJobId] = useState<number | null>(null);
  const [file, setFile] = useState<string | null>(null);

  // A large export: ask for its job until it is done.
  useEffect(() => {
    if (jobId === null) {
      return;
    }
    let stop = false;
    const tick = async () => {
      while (!stop) {
        try {
          const job = await api.job(jobId);
          const output = job.output as { file?: string; rows?: number } | undefined;
          if (job.state === "completed" && output?.file) {
            setFile(output.file);
            setMessage(t("export.ready", { rows: output.rows ?? 0 }));
            setJobId(null);
            return;
          }
          if (job.state === "discarded" || job.state === "cancelled") {
            setMessage(t("export.failed"));
            setJobId(null);
            return;
          }
        } catch {
          // try again
        }
        await new Promise((r) => setTimeout(r, 2000));
      }
    };
    void tick();
    return () => {
      stop = true;
    };
  }, [jobId, t]);

  const run = async () => {
    setBusy(true);
    setMessage(null);
    setFile(null);
    try {
      const result = await api.export(typeName, {
        format,
        columns,
        sort,
        lang: i18n.language,
        time_zone: currentTimeZone(),
        ...(where ? { where } : {}),
      });
      if ("job" in result) {
        setJobId(result.job.id);
        setMessage(t("export.queued"));
      } else {
        save(result.file, result.name);
      }
    } catch (err) {
      setMessage(
        err instanceof ApiError
          ? t("export.error", { detail: err.problem?.detail ?? err.message })
          : t("export.error", { detail: String(err) }),
      );
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <label htmlFor={`${id}-format`} className="sr-only">
        {t("export.format")}
      </label>
      <select
        id={`${id}-format`}
        className="h-10 rounded-md border border-input bg-background px-2 text-sm"
        value={format}
        onChange={(e) => setFormat(e.target.value as Format)}
      >
        <option value="xlsx">Excel (XLSX)</option>
        <option value="ods">OpenDocument (ODS)</option>
        <option value="csv">CSV</option>
      </select>
      <Button variant="outline" disabled={busy || jobId !== null} onClick={() => void run()}>
        {t("export.run")}
      </Button>
      {message && (
        <span role="status" className="text-sm">
          {message}
        </span>
      )}
      {file && (
        <a className="text-sm underline" href={`/api/v1/files/${file}`} download>
          {t("export.download")}
        </a>
      )}
    </div>
  );
}
