// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// The messages at the bottom right (UI59): the newest at the bottom, at most three, success and
// info go after 5 s unless the mouse holds them, warning and error stay until closed, a button
// such as "Rückgängig" keeps a message 10 s.
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { expectAccessible } from "@/test/setup";
import { toast, Toaster } from "@/ui/toast";

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  act(() => toast.clear());
  vi.useRealTimers();
});

function messages() {
  return [...screen.queryAllByRole("status"), ...screen.queryAllByRole("alert")];
}

describe("toast", () => {
  it("stacks the four kinds with the newest at the bottom", async () => {
    const { container } = render(<Toaster />);
    act(() => {
      toast.success("Gespeichert");
      toast.info("Export läuft", "Die Datei kommt gleich.");
      toast.warning("2 von 4 geändert", "Zwei ohne Recht.");
    });
    const region = screen.getByRole("region", { name: "toast.region" });
    expect([...region.children].map((c) => c.textContent)).toEqual([
      "Gespeichert",
      "Export läuftDie Datei kommt gleich.",
      "2 von 4 geändertZwei ohne Recht.",
    ]);
    expect(screen.getAllByRole("status")).toHaveLength(2);
    expect(screen.getByRole("alert")).toHaveTextContent("2 von 4 geändert");
    vi.useRealTimers();
    await expectAccessible(container);
    vi.useFakeTimers();

    // A fourth pushes out the oldest.
    act(() => {
      toast.error("Nichts geändert");
    });
    expect(messages()).toHaveLength(3);
    expect(screen.queryByText("Gespeichert")).not.toBeInTheDocument();
    expect([...region.children].at(-1)).toHaveTextContent("Nichts geändert");
  });

  it("lets success go after 5 s and keeps it under the mouse", () => {
    render(<Toaster />);
    act(() => {
      toast.success("Gespeichert");
    });
    act(() => vi.advanceTimersByTime(4900));
    const status = screen.getByRole("status");
    fireEvent.mouseEnter(status);
    act(() => vi.advanceTimersByTime(10000));
    expect(screen.getByRole("status")).toBeInTheDocument();
    fireEvent.mouseLeave(status);
    act(() => vi.advanceTimersByTime(1100));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("keeps an error until it is closed", () => {
    render(<Toaster />);
    act(() => {
      toast.error("Speichern fehlgeschlagen", "Keine Verbindung.");
    });
    act(() => vi.advanceTimersByTime(60000));
    expect(screen.getByRole("alert")).toHaveTextContent("Speichern fehlgeschlagen");
    fireEvent.click(screen.getByRole("button", { name: "toast.close" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps a message with a button 10 s and runs the button", () => {
    const undo = vi.fn();
    render(<Toaster />);
    act(() => {
      toast.success("4 in den Papierkorb", undefined, {
        action: { label: "Rückgängig", onClick: undo },
      });
    });
    act(() => vi.advanceTimersByTime(6000));
    fireEvent.click(screen.getByRole("button", { name: "Rückgängig" }));
    expect(undo).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();

    act(() => {
      toast.info("Noch eine", undefined, { action: { label: "Rückgängig", onClick: undo } });
    });
    act(() => vi.advanceTimersByTime(9900));
    expect(screen.getByRole("status")).toBeInTheDocument();
    act(() => vi.advanceTimersByTime(200));
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
