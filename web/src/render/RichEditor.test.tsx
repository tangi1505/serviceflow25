// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The editor (UI7): bold from the toolbar, an image chosen with the button is uploaded and
// shown from the file store, the HTML goes out; saved HTML shows formatted, plain text of it
// in lists.
import { act, render, screen, waitFor } from "@testing-library/react";
import type { Editor } from "@tiptap/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { initI18n } from "@/i18n";
import { fakeApi, newState, type FakeState } from "@/test/fakeApi";
import { expectAccessible } from "@/test/setup";

import { plainText } from "./fields";
import { RichEditor, RichText } from "./RichEditor";

function Harness({ onHtml }: { onHtml: (h: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <RichEditor
      id="ed"
      label="Lösung"
      value={value}
      onChange={(h) => {
        setValue(h);
        onHtml(h);
      }}
    />
  );
}

describe("RichEditor", () => {
  let state: FakeState;
  beforeEach(() => {
    state = newState(0);
    state.signedIn = true;
    initI18n("de");
    vi.stubGlobal("fetch", fakeApi(state));
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("formats, uploads an image and gives the HTML", async () => {
    const user = userEvent.setup();
    let html = "";
    const { container } = render(<Harness onHtml={(h) => (html = h)} />);
    const box = screen.getByRole("textbox", { name: "Lösung" });
    const editor = (box as unknown as { editor: Editor }).editor;
    act(() => {
      editor.commands.insertContent("Kabel getauscht");
      editor.commands.selectAll();
    });
    await user.click(screen.getByRole("button", { name: "Fett" }));
    expect(html).toBe("<p><strong>Kabel getauscht</strong></p>");
    expect(screen.getByRole("button", { name: "Fett" })).toHaveAttribute("aria-pressed", "true");

    const file = new File(["png"], "router.png", { type: "image/png" });
    await user.upload(screen.getByLabelText("Bild einfügen"), file);
    await waitFor(() =>
      expect(html).toContain(
        '<img src="/api/v1/files/01a15a3c-8545-7691-b780-f11e00000002" alt="router.png">',
      ),
    );
    expect(
      state.calls.some(
        (c) =>
          c.method === "POST" && c.path === "/api/v1/files" && c.query.get("name") === "router.png",
      ),
    ).toBe(true);
    await expectAccessible(container);
  });

  it("shows saved HTML and its plain text", () => {
    render(<RichText html="<p>Siehe <strong>Bild</strong></p><ul><li>eins</li></ul>" />);
    expect(screen.getByText("Bild").tagName).toBe("STRONG");
    expect(plainText("<p>Siehe <strong>Bild</strong></p><ul><li>eins</li></ul>")).toBe(
      "Siehe Bildeins",
    );
  });
});
