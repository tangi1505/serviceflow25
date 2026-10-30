# E41: Frontend-Bibliotheken

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E41
- **Architektur:** [`architektur.md`](../architektur.md) §10

## Kontext

React gibt wenig vor; ohne feste Wahl wächst Wildwuchs.

## Entscheidung

**Frontend-Bibliotheken:** Vite; TanStack Router, Query, Table und Virtual; `react-hook-form` + `zod`; shadcn/ui auf Radix und Tailwind; React Flow; `dnd-kit`; ECharts; `i18next`; Vitest und Playwright (V2)

## Verworfen

andere UI-Bibliothek (Mantine, MUI)

## Folgen

Feste Bausteine für Routing, Daten, Tabellen, Formulare, UI, Editoren, Diagramme, Übersetzung und Tests. Abweichungen nur über eine neue Entscheidung.
