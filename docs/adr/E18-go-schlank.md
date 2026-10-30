# E18: Go schlank: chi, pgx, sqlc, goose

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E18
- **Architektur:** [`architektur.md`](../architektur.md) §2, §3

## Kontext

Wenig Magie, volle Kontrolle über SQL, Typsicherheit für die festen Tabellen.

## Entscheidung

Go schlank: **`net/http` + `chi`**, **`pgx` + `sqlc`**, **`goose`** für Migrationen; kein ORM

## Verworfen

Ent; GORM

## Folgen

Kein ORM. Kerntabellen über `sqlc` und `goose`, Typtabellen über den eigenen Builder (E43).
