// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The text modules in the editor (SD8): with the keyboard only, Alt+T opens the list, the
// search narrows it, the modules of the person's groups come first, the preview shows the
// filled-in text and Enter puts it at the caret; Esc closes and gives the text back the focus.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import type { Editor } from "@tiptap/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { RichEditor } from "./RichEditor";

const group = { type: "core.group", id: "01a15a3c-8545-7691-b780-aaaaaaaaaaaa" };

function Harness({ recordId, onHtml }: { recordId: string; onHtml: (h: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <RichEditor
      id="ed"
      label="Neuer Eintrag"
      value={value}
      onChange={(h) => {
        setValue(h);
        onHtml(h);
      }}
      snippets={{ use: "answer", typeName: "probe.probe", recordId }}
    />
  );
}

describe("text modules", () => {
  let state: FakeState;
  // jsdom has no layout; the editor scrolls the caret into view when it takes the focus.
  beforeAll(() => {
    const rect = {
      x: 0,
      y: 0,
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      width: 0,
      height: 0,
      toJSON: () => ({}),
    };
    Range.prototype.getBoundingClientRect = () => rect as DOMRect;
    Range.prototype.getClientRects = () => [rect] as unknown as DOMRectList;
  });
  beforeEach(() => {
    state = newState(1);
    state.signedIn = true;
    state.records[0]!["title"] = "Drucker & Fax";
    state.snippets = [
      {
        id: "s1",
        name: "Fehlerprotokoll des Browsers",
        use: "answer",
        body: "<p>Browser</p>",
        actors: {},
      },
      {
        id: "s2",
        name: "Protokoll anfordern",
        use: "answer",
        body: "<p>Zu „{{record.title}}“ bitte das Protokoll.</p>",
        actors: { shared: [group] },
      },
      {
        id: "s3",
        name: "Gelöst durch Neustart",
        use: "solution",
        body: "<p>Neustart</p>",
        actors: {},
      },
    ];
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("are chosen and inserted with the keyboard only", async () => {
    const user = userEvent.setup();
    let html = "";
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <Harness recordId={String(state.records[0]!["id"])} onHtml={(h) => (html = h)} />
      </QueryClientProvider>,
    );
    const box = screen.getByRole("textbox", { name: "Neuer Eintrag" });
    const editor = (box as unknown as { editor: Editor }).editor;
    // The caret stands after the greeting; the focus is in the text (no click: jsdom has no
    // coordinates for ProseMirror to place the caret by).
    act(() => {
      editor.commands.insertContent("<p>Guten Tag,</p>");
      box.focus();
    });

    await user.keyboard("{Alt>}t{/Alt}");
    const dialog = await screen.findByRole("dialog", { name: "Textbaustein wählen" });
    const search = within(dialog).getByRole("combobox", { name: "Textbaustein suchen" });
    expect(search).toHaveFocus();
    // The modules of the person's groups first; the one of another use is not offered.
    await waitFor(() =>
      expect(
        within(dialog)
          .getAllByRole("option")
          .map((o) => o.textContent),
      ).toEqual(["Protokoll anfordern", "Fehlerprotokoll des Browsers"]),
    );
    expect(within(dialog).getByRole("group", { name: "Meine Gruppen" })).toBeInTheDocument();
    await user.type(search, "anford");
    expect(within(dialog).getAllByRole("option")).toHaveLength(1);
    // The preview is filled in by the server, the title escaped.
    expect(
      await within(dialog).findByText("Zu „Drucker & Fax“ bitte das Protokoll."),
    ).toBeInTheDocument();
    await expectAccessible(container);

    await user.keyboard("{Enter}");
    await waitFor(() => expect(html).toContain("Zu „Drucker &amp; Fax“ bitte das Protokoll."));
    expect(html.startsWith("<p>Guten Tag,</p>")).toBe(true);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    // The editor takes the focus in the next frame.
    await waitFor(() => expect(box).toHaveFocus());

    // Esc closes without inserting; the button opens the list as well.
    await user.click(screen.getByRole("button", { name: /Textbaustein/ }));
    await screen.findByRole("dialog", { name: "Textbaustein wählen" });
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    // The editor takes the focus in the next frame.
    await waitFor(() => expect(box).toHaveFocus());
  });

  it("are not offered without a record", () => {
    render(<RichEditor id="ed" label="Lösung" value="" onChange={() => {}} />);
    expect(screen.queryByRole("button", { name: /Textbaustein/ })).not.toBeInTheDocument();
  });
});
