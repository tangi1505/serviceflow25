// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// selectOption chooses in the select field of the kit (UI57, UI58) as a person does: it opens
// the field and clicks the option, found by its value or else by its label. It takes the place
// of user.selectOptions, which knows only the native <select>.
import type userEvent from "@testing-library/user-event";

type User = ReturnType<typeof userEvent.setup>;

export async function selectOption(user: User, box: HTMLElement, value: string) {
  if (box.getAttribute("aria-expanded") !== "true") {
    await user.click(box);
  }
  const list = document.getElementById(box.getAttribute("aria-controls") ?? "");
  if (!list) {
    throw new Error(`selectOption: the field ${box.id || box.textContent} has no open list`);
  }
  const options = [...list.querySelectorAll<HTMLElement>('[role="option"]')];
  const hit =
    options.find((o) => o.dataset["value"] === value) ??
    options.find((o) => o.textContent?.trim() === value);
  if (!hit) {
    throw new Error(
      `selectOption: no option ${value} in ${options.map((o) => o.dataset["value"]).join(", ")}`,
    );
  }
  await user.click(hit);
}
