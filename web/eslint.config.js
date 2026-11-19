// Copyright (C) 2026 group24 AG
// SPDX-License-Identifier: AGPL-3.0-only
//
// This program is free software: you can redistribute it and/or modify it under the terms of
// the GNU Affero General Public License as published by the Free Software Foundation, version 3.
// It is distributed WITHOUT ANY WARRANTY; see the LICENSE file for details.

// ESLint and Prettier (Hauptplan E57): TypeScript, React hooks and accessibility (jsx-a11y, E42).
import js from "@eslint/js";
import prettier from "eslint-config-prettier";
import jsxA11y from "eslint-plugin-jsx-a11y";
import reactHooks from "eslint-plugin-react-hooks";
import { defineConfig, globalIgnores } from "eslint/config";
import globals from "globals";
import tseslint from "typescript-eslint";

export default defineConfig([
  globalIgnores(["dist", "test-results", "playwright-report"]),
  js.configs.recommended,
  tseslint.configs.recommended,
  jsxA11y.flatConfigs.recommended,
  {
    files: ["**/*.{ts,tsx}"],
    languageOptions: { globals: globals.browser },
    plugins: { "react-hooks": reactHooks },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
    },
  },
  {
    // Every control once in src/ui/ (UI57, UI-Plan E59): a page builds no select and no text
    // field of its own but takes Select and Input. Boxes, radios, colour, file and hidden fields
    // are no such control. An error since UI58 has moved all pages.
    files: ["src/**/*.tsx"],
    ignores: ["src/ui/**", "src/**/*.test.tsx"],
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXOpeningElement[name.name='select']",
          message: "Select aus @/ui/select statt <select> (UI57).",
        },
        {
          selector:
            "JSXOpeningElement[name.name='input']:not(:has(JSXAttribute[name.name='type'][value.value=/^(checkbox|radio|color|file|hidden)$/]))",
          message: "Input aus @/ui/input statt <input> (UI57).",
        },
      ],
    },
  },
  prettier,
]);
