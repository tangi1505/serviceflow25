// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// answer clicks a button of the common question (ui/confirm.tsx, UI64) once it is open.
import { screen, within } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

export async function answer(user: UserEvent, button: string) {
  const dialog = await screen.findByRole("alertdialog");
  await user.click(within(dialog).getByRole("button", { name: button }));
}
