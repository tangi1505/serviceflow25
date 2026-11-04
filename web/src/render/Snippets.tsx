// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The text modules in the editor (SD8, Inventar G56; mockup variant A): the button
// "Textbaustein" at the end of the toolbar, or Alt+T in the text, opens a list with a search
// and a preview. Enter inserts the text the server filled in at the caret, Esc closes; the
// focus goes back to the text either way. The list holds the modules of the use (answer,
// solution, task) for the type or for every type, shared with the person, their groups or
// nobody: a filter, the right to read stays with the role.
import { queryOptions, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Editor } from "@tiptap/react";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";

import { api, type RecordData } from "@/api/client";
import { Button } from "@/ui/button";
import { Input } from "@/ui/input";
import { cn } from "@/ui/utils";

import { RichText } from "./RichText";

// Where the editor stands: the use of the modules it offers and the record they are filled in
// against.
export type SnippetContext = { use: string; typeName: string; recordId: string };

// snippetWhere is the condition of the list (the Go test TestSnippetsOverHTTP runs the same).
export function snippetWhere(use: string, typeName: string) {
  return {
    and: [
      { field: "use", eq: use },
      {
        or: [
          { field: "target_type", null: true },
          { field: "target_type", eq: typeName },
        ],
      },
      {
        or: [
          { field: "actors.shared", null: true },
          { field: "actors.shared", eq: "@my_groups" },
          { field: "actors.shared", eq: "@me" },
        ],
      },
    ],
  };
}

const snippetsQuery = (use: string, typeName: string) =>
  queryOptions({
    queryKey: ["snippets", use, typeName],
    queryFn: () =>
      api.query("core.snippet", { where: snippetWhere(use, typeName), sort: ["name"], limit: 100 }),
  });

const renderQuery = (c: SnippetContext, snippet: string) =>
  queryOptions({
    queryKey: ["snippet", c.typeName, c.recordId, snippet],
    queryFn: () => api.renderSnippet(c.typeName, c.recordId, snippet),
  });

function shared(r: RecordData): boolean {
  const actors = r["actors"] as Record<string, unknown[]> | undefined;
  return (actors?.["shared"]?.length ?? 0) > 0;
}

export function SnippetPicker({
  editor,
  context,
  open,
  onOpenChange,
}: {
  editor: Editor | null;
  context: SnippetContext;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const listId = useId();
  const search = useRef<HTMLInputElement>(null);
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const [failure, setFailure] = useState<string | null>(null);
  const list = useQuery({ ...snippetsQuery(context.use, context.typeName), enabled: open });
  const q = text.trim().toLowerCase();
  const items = (list.data?.items ?? []).filter((r) =>
    String(r["name"] ?? "")
      .toLowerCase()
      .includes(q),
  );
  // The modules of the person's groups first, then those for everybody (mockup A).
  const mine = items.filter(shared);
  const all = items.filter((r) => !shared(r));
  const shown = [...mine, ...all];
  const current = Math.min(active, shown.length - 1);
  const chosen = shown[current];
  const preview = useQuery({
    ...renderQuery(context, String(chosen?.["id"] ?? "")),
    enabled: open && !!chosen,
  });
  const optionId = (i: number) => `${listId}-${i}`;
  // Opened, the search takes the focus; closed, the text has it again (close).
  useEffect(() => {
    if (open) {
      search.current?.focus();
    }
  }, [open]);

  const close = () => {
    onOpenChange(false);
    setText("");
    setActive(0);
    editor?.commands.focus();
  };
  const insert = async (r: RecordData) => {
    setFailure(null);
    try {
      const html = await queryClient.fetchQuery(renderQuery(context, String(r["id"])));
      close();
      editor?.chain().focus().insertContent(html).run();
    } catch (err) {
      setFailure(
        t("editor.snippetError", { detail: err instanceof Error ? err.message : String(err) }),
      );
    }
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setActive(Math.min(current + 1, shown.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        setActive(Math.max(current - 1, 0));
        break;
      case "Enter":
        e.preventDefault();
        if (chosen) {
          void insert(chosen);
        }
        break;
      case "Escape":
        e.preventDefault();
        close();
        break;
    }
  };
  const option = (r: RecordData, i: number) => (
    // The keyboard works in the search (aria-activedescendant), as in the reference picker.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events
    <div
      key={String(r["id"])}
      id={optionId(i)}
      role="option"
      tabIndex={-1}
      aria-selected={i === current}
      className={cn("cursor-pointer px-3 py-1.5", i === current && "bg-secondary")}
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => void insert(r)}
    >
      {String(r["name"] ?? "")}
    </div>
  );
  const group = (label: string, rows: RecordData[], offset: number) =>
    rows.length > 0 && (
      <div role="group" aria-label={label}>
        <div
          aria-hidden="true"
          className="px-3 pt-2 pb-1 text-xs font-medium uppercase text-muted-foreground"
        >
          {label}
        </div>
        {rows.map((r, i) => option(r, offset + i))}
      </div>
    );

  return (
    <div className="relative">
      <Button
        type="button"
        size="sm"
        variant={open ? "default" : "outline"}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-keyshortcuts="Alt+T"
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => (open ? close() : onOpenChange(true))}
        disabled={!editor}
      >
        {t("editor.snippet")}
        <kbd className="ml-1 rounded border px-1 font-mono text-xs" aria-hidden="true">
          Alt T
        </kbd>
      </Button>
      {open && (
        <div
          role="dialog"
          aria-label={t("editor.snippetPick")}
          className="absolute right-0 z-20 mt-1 grid w-[min(36rem,calc(100vw-3rem))] grid-cols-1 overflow-hidden rounded-md border bg-card text-sm shadow-md sm:grid-cols-2"
          onBlur={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
              onOpenChange(false);
            }
          }}
        >
          <div className="flex min-w-0 flex-col border-r">
            <div className="border-b p-2">
              <Input
                ref={search}
                role="combobox"
                autoComplete="off"
                aria-label={t("editor.snippetSearch")}
                aria-autocomplete="list"
                aria-expanded
                aria-controls={listId}
                aria-activedescendant={chosen ? optionId(current) : undefined}
                value={text}
                onChange={(e) => {
                  setText(e.target.value);
                  setActive(0);
                }}
                onKeyDown={onKeyDown}
              />
            </div>
            <div
              id={listId}
              role="listbox"
              aria-label={t("editor.snippetPick")}
              className="max-h-64 overflow-auto py-1"
            >
              {group(t("editor.snippetMine"), mine, 0)}
              {group(t("editor.snippetAll"), all, mine.length)}
              {shown.length === 0 && (
                <p className="px-3 py-1.5 text-muted-foreground">
                  {list.isPending ? t("list.loading") : t("editor.snippetNone")}
                </p>
              )}
            </div>
            <p className="border-t bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground">
              {t("editor.snippetKeys")}
            </p>
          </div>
          <section
            aria-label={t("editor.snippetPreview")}
            className="hidden flex-col gap-1 bg-muted/20 p-3 sm:flex"
          >
            <span className="text-xs text-muted-foreground">{t("editor.snippetPreview")}</span>
            {preview.data && <RichText html={preview.data} />}
          </section>
          {failure && (
            <p role="alert" className="px-3 py-1 text-sm text-destructive sm:col-span-2">
              {failure}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
