// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// pick chooses in the select field of the kit (UI57, UI58) as a person does: open the field,
// click the option by its value or else by its label. It takes the place of selectOption, which
// knows only the native <select>.
import { expect, type Locator } from "@playwright/test";

export async function pick(field: Locator, value: string) {
  await field.click();
  await expect(field).toHaveAttribute("aria-expanded", "true");
  const id = await field.getAttribute("aria-controls");
  const list = field.page().locator(`[id="${id}"]`);
  const byValue = list.locator(`[role="option"][data-value="${value}"]`);
  if ((await byValue.count()) > 0) {
    await byValue.click();
  } else {
    await list.getByRole("option", { name: value, exact: true }).click();
  }
  await expect(field).toHaveAttribute("aria-expanded", "false");
}
