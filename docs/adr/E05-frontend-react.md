# E5: Frontend in React und TypeScript

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E5
- **Architektur:** [`architektur.md`](../architektur.md) §10

## Kontext

Die Oberfläche lebt von Listen, Formularen, Dashboards, einem Formular-Builder und einem Workflow-Editor, die fast alle aus Definitionen gerendert werden.

## Entscheidung

Frontend **React + TypeScript** als eigenständige Anwendung gegen die API, mit festen Konventionen (§5)

## Verworfen

Angular (erzwungene Einheitlichkeit lohnt erst bei großen Teams, Ökosystem für Workflow-Editor und Formular-Builder dünner); Vue; Svelte; serverseitig aus Go

## Folgen

Großes Ökosystem für Editoren und Tabellen. Weil React wenig vorgibt, legen E41 und die Konventionen im Hauptplan §5 fest, welche Bibliotheken und Muster gelten.
