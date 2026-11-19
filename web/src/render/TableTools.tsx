// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// What the plain tables (UI54) take from the person: the row height of "Ansicht" in the list,
// and a field that narrows the rows of an overview as one types.
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { useId, type ComponentProps } from "react";

import { mySettingsQuery } from "@/api/client";
import { Table, type Density } from "@/ui/table";
import { Input } from "@/ui/input";

// useDensity is the row height the person chose for lists, normal without a choice.
export function useDensity(): Density {
  const settings = useQuery(mySettingsQuery);
  return settings.data?.density?.value === "compact" ? "compact" : "normal";
}

// DensityTable is the table of ui/table.tsx in the row height of the person.
export function DensityTable(props: ComponentProps<typeof Table>) {
  const density = useDensity();
  return <Table density={density} {...props} />;
}

// matches tells whether one of the texts holds what was typed, without regard to case.
export function matches(query: string, ...texts: (string | undefined)[]): boolean {
  const q = query.trim().toLocaleLowerCase();
  return q === "" || texts.some((s) => (s ?? "").toLocaleLowerCase().includes(q));
}

// TableSearch is the field over an overview; the label is read out, the field shows it as its
// placeholder.
export function TableSearch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const id = useId();
  return (
    <div className="relative w-full max-w-72">
      <label htmlFor={id} className="sr-only">
        {label}
      </label>
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-foreground-2"
      />
      <Input
        id={id}
        size="bar"
        type="search"
        value={value}
        placeholder={label}
        onChange={(e) => onChange(e.target.value)}
        className="pr-2 pl-8 placeholder:text-foreground-2"
      />
    </div>
  );
}
