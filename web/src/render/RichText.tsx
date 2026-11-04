// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// Formatted text as the page shows it (UI7), apart from the editor so that the editor's parts
// (the text modules, SD8) can show it too.
import { cn } from "@/ui/utils";

export const prose =
  "[&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:pl-3 [&_h2]:text-lg [&_h2]:font-semibold " +
  "[&_h3]:font-semibold [&_img]:my-1 [&_img]:max-h-80 [&_img]:max-w-full [&_ol]:list-decimal [&_ol]:pl-6 " +
  "[&_p]:my-1 [&_pre]:rounded [&_pre]:bg-muted [&_pre]:p-2 [&_ul]:list-disc [&_ul]:pl-6";

// RichText shows HTML the server sanitized (richtext.Sanitize): the only place that puts
// stored HTML into the page.
export function RichText({ html, className }: { html: string; className?: string }) {
  return (
    <div className={cn("text-sm", prose, className)} dangerouslySetInnerHTML={{ __html: html }} />
  );
}
