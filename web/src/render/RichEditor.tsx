// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The editor of formatted text (UI7, Hauptplan E127; TipTap under MIT, UI plan E1): bold,
// italic, lists, quote, headings, links; an image pasted, dropped or chosen is uploaded as a
// temporary file of the session and shown from /api/v1/files/{id}; the server sanitizes the
// HTML and attaches the image when the text is saved. RichText shows saved HTML, which the
// server cleaned.
import Image from "@tiptap/extension-image";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

import { api } from "@/api/client";
import { Button } from "@/ui/button";
import { cn } from "@/ui/utils";

const prose =
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

async function insertImages(editor: Editor, files: File[], onError: (msg: string) => void) {
  for (const file of files) {
    try {
      const uploaded = await api.upload(file);
      editor
        .chain()
        .focus()
        .setImage({ src: `/api/v1/files/${uploaded.id}`, alt: file.name })
        .run();
    } catch (err) {
      onError(err instanceof Error ? err.message : String(err));
    }
  }
}

function images(list: FileList | null | undefined): File[] {
  return Array.from(list ?? []).filter((f) => f.type.startsWith("image/"));
}

export function RichEditor({
  id,
  label,
  value,
  onChange,
  describedBy,
  invalid,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (html: string) => void;
  describedBy?: string;
  invalid?: boolean;
}) {
  const { t } = useTranslation();
  const fileId = useId();
  const [failure, setFailure] = useState<string | null>(null);
  const editorRef = useRef<Editor | null>(null);
  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3, 4] },
        link: { openOnClick: false, autolink: true, protocols: ["http", "https", "mailto"] },
      }),
      Image,
    ],
    content: value,
    editorProps: {
      attributes: {
        id,
        role: "textbox",
        "aria-multiline": "true",
        "aria-label": label,
        ...(describedBy ? { "aria-describedby": describedBy } : {}),
        ...(invalid ? { "aria-invalid": "true" } : {}),
        class: cn("min-h-24 px-3 py-2 text-sm focus:outline-none", prose),
      },
      handlePaste: (_view, event) => {
        const found = images(event.clipboardData?.files);
        if (found.length === 0 || !editorRef.current) {
          return false;
        }
        void insertImages(editorRef.current, found, (m) =>
          setFailure(t("editor.uploadError", { detail: m })),
        );
        return true;
      },
      handleDrop: (_view, event) => {
        const found = images((event as DragEvent).dataTransfer?.files);
        if (found.length === 0 || !editorRef.current) {
          return false;
        }
        void insertImages(editorRef.current, found, (m) =>
          setFailure(t("editor.uploadError", { detail: m })),
        );
        return true;
      },
    },
    onUpdate: ({ editor: e }) => onChange(e.isEmpty ? "" : e.getHTML()),
  });
  editorRef.current = editor;

  // A value set from outside (a reset after saving) replaces the content.
  useEffect(() => {
    if (editor && value !== (editor.isEmpty ? "" : editor.getHTML())) {
      editor.commands.setContent(value, { emitUpdate: false });
    }
  }, [editor, value]);

  const button = (name: string, active: boolean, run: () => void) => (
    <Button
      type="button"
      size="sm"
      variant={active ? "default" : "outline"}
      aria-pressed={active}
      // The editor keeps the focus and the selection: a click on the toolbar must not take
      // them (found in Playwright: typed text went to the button).
      onMouseDown={(e) => e.preventDefault()}
      onClick={run}
      disabled={!editor}
    >
      {t(`editor.${name}`)}
    </Button>
  );
  const chain = () => editor!.chain().focus();
  return (
    <div className="flex flex-col rounded-md border border-input bg-background">
      <div
        role="toolbar"
        aria-label={t("editor.toolbar", { label })}
        className="flex flex-wrap gap-1 border-b p-1"
      >
        {button("bold", editor?.isActive("bold") ?? false, () => chain().toggleBold().run())}
        {button("italic", editor?.isActive("italic") ?? false, () => chain().toggleItalic().run())}
        {button("bullets", editor?.isActive("bulletList") ?? false, () =>
          chain().toggleBulletList().run(),
        )}
        {button("numbers", editor?.isActive("orderedList") ?? false, () =>
          chain().toggleOrderedList().run(),
        )}
        {button("quote", editor?.isActive("blockquote") ?? false, () =>
          chain().toggleBlockquote().run(),
        )}
        {button("link", editor?.isActive("link") ?? false, () => {
          const href = window.prompt(
            t("editor.linkPrompt"),
            editor?.getAttributes("link")["href"] as string | undefined,
          );
          if (href === null) {
            return;
          }
          if (href === "") {
            chain().unsetLink().run();
          } else {
            chain().setLink({ href }).run();
          }
        })}
        <label
          htmlFor={fileId}
          className="inline-flex h-9 cursor-pointer items-center rounded-md border border-input px-3 text-sm hover:bg-muted"
        >
          {t("editor.image")}
        </label>
        <input
          id={fileId}
          type="file"
          accept="image/*"
          className="sr-only"
          onChange={(e) => {
            const found = images(e.target.files);
            if (editor && found.length > 0) {
              void insertImages(editor, found, (m) =>
                setFailure(t("editor.uploadError", { detail: m })),
              );
            }
            e.target.value = "";
          }}
        />
      </div>
      <EditorContent editor={editor} />
      {failure && (
        <p role="alert" className="px-3 py-1 text-sm text-destructive">
          {failure}
        </p>
      )}
    </div>
  );
}
