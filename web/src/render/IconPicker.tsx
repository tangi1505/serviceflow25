// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// IconPicker is a text field of the format icon (UI42, UI plan E53): the symbols of the fixed
// list as a grid to click, "Ohne" first. They are radio buttons under a legend, so arrows move
// between them and a screen reader says the name of each symbol.
import { useTranslation } from "react-i18next";

import { packageIcons } from "@/app/icons";

export function IconPicker({
  id,
  legend,
  required,
  value,
  onChange,
  invalid,
  describedBy,
}: {
  id: string;
  legend: string;
  required?: boolean;
  value: string;
  onChange: (value: string) => void;
  invalid?: boolean;
  describedBy?: string;
}) {
  const { t } = useTranslation();
  const names = required ? Object.keys(packageIcons) : ["", ...Object.keys(packageIcons)];
  return (
    <fieldset id={id} aria-describedby={describedBy} aria-invalid={invalid} className="min-w-0">
      <legend className="mb-2 text-sm leading-none font-medium">
        {legend}
        {required && <span aria-hidden="true"> *</span>}
      </legend>
      <div className="grid max-w-md grid-cols-[repeat(auto-fill,2.75rem)] gap-1.5">
        {names.map((name) => {
          const Icon = name ? packageIcons[name] : null;
          const label = name ? t(`icons.${name}`) : t("icons.none");
          return (
            <label
              key={name || "none"}
              title={label}
              className="inline-grid size-11 cursor-pointer place-items-center rounded-md border bg-card text-foreground-2 hover:bg-surface-active has-[:checked]:border-primary has-[:checked]:bg-surface-active has-[:checked]:text-primary has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
            >
              <input
                type="radio"
                name={id}
                value={name}
                checked={value === name}
                onChange={() => onChange(name)}
                aria-label={label}
                className="sr-only"
              />
              {Icon ? (
                <Icon aria-hidden="true" className="size-5" />
              ) : (
                <span aria-hidden="true" className="text-xs">
                  {label}
                </span>
              )}
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
