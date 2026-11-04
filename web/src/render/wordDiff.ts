// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The words two texts differ in (SD16b, your decision: no library): the longest common
// sequence of words and spaces; what only the old text has is "del", what only the new one has
// "ins". Texts longer than the limit compare as a whole, so that a huge field stays cheap.

export type Segment = { text: string; kind: "same" | "del" | "ins" };

const limit = 2000;

function words(s: string): string[] {
  return s.split(/(\s+)/).filter((w) => w !== "");
}

// push adds a piece to a side, joined with the piece before when both are of one kind.
function push(out: Segment[], text: string, kind: Segment["kind"]) {
  const last = out[out.length - 1];
  if (last && last.kind === kind) {
    last.text += text;
  } else {
    out.push({ text, kind });
  }
}

export function wordDiff(before: string, after: string): { old: Segment[]; new: Segment[] } {
  const a = words(before);
  const b = words(after);
  if (a.length > limit || b.length > limit) {
    return {
      old: before ? [{ text: before, kind: before === after ? "same" : "del" }] : [],
      new: after ? [{ text: after, kind: before === after ? "same" : "ins" }] : [],
    };
  }
  // lcs[i][j] is the length of the common sequence of a[i:] and b[j:].
  const lcs: number[][] = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0),
  );
  for (let i = a.length - 1; i >= 0; i--) {
    for (let j = b.length - 1; j >= 0; j--) {
      lcs[i]![j] =
        a[i] === b[j] ? lcs[i + 1]![j + 1]! + 1 : Math.max(lcs[i + 1]![j]!, lcs[i]![j + 1]!);
    }
  }
  const old: Segment[] = [];
  const neu: Segment[] = [];
  let i = 0;
  let j = 0;
  while (i < a.length && j < b.length) {
    if (a[i] === b[j]) {
      push(old, a[i]!, "same");
      push(neu, b[j]!, "same");
      i++;
      j++;
    } else if (lcs[i + 1]![j]! >= lcs[i]![j + 1]!) {
      push(old, a[i++]!, "del");
    } else {
      push(neu, b[j++]!, "ins");
    }
  }
  while (i < a.length) {
    push(old, a[i++]!, "del");
  }
  while (j < b.length) {
    push(neu, b[j++]!, "ins");
  }
  return { old, new: neu };
}
