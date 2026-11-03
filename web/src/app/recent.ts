// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The records one opened last, per account in this browser (UI31, UI-Plan E36): the command line
// shows them while nothing is typed. Only type, id and title are kept, never field values; the
// sign-out forgets them, so that no title stays in the browser (VS-NfD).

export type Recent = { type: string; id: string; title: string };

// shown is how many the command line lists.
export const shown = 5;

const key = (account: string) => `itsm.recent.${account}`;

export function recentRecords(account: string): Recent[] {
  try {
    const raw = JSON.parse(localStorage.getItem(key(account)) ?? "[]") as unknown;
    return Array.isArray(raw) ? (raw as Recent[]).slice(0, shown) : [];
  } catch {
    return [];
  }
}

// remember puts the record first; an older entry of it goes.
export function remember(account: string, r: Recent): void {
  const rest = recentRecords(account).filter((x) => x.type !== r.type || x.id !== r.id);
  try {
    localStorage.setItem(key(account), JSON.stringify([r, ...rest].slice(0, shown)));
  } catch {
    // Without storage the command line shows no recent records.
  }
}

export function forget(account: string): void {
  try {
    localStorage.removeItem(key(account));
  } catch {
    // Nothing was kept.
  }
}
