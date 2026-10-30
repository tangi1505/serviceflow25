# E29: Konflikterkennung statt Sperren

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E29
- **Architektur:** [`architektur.md`](../architektur.md) §3, §6

## Kontext

Sperren bleiben hängen und bremsen Middleware-Updates.

## Entscheidung

**Keine Sperren beim Bearbeiten:** Jeder kann öffnen; beim Speichern erkennt der Kern, wenn jemand dazwischen gespeichert hat, und zeigt die Änderungen. Live sichtbar, wer gerade bearbeitet

## Verworfen

Sperren wie GLPI; je Typ wählbar

## Folgen

Jeder Datensatz hat `version`; Ändern braucht `If-Match`. Die Oberfläche zeigt live, wer gerade bearbeitet, und bei einem Konflikt die Änderungen.
