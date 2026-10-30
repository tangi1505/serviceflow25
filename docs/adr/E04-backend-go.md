# E4: Backend in Go

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E4
- **Architektur:** [`architektur.md`](../architektur.md) §1, §2

## Kontext

Das Backend muss Massendaten schnell verarbeiten, als ein Binary im eigenen Rechenzentrum laufen und von einem Team aus zwei Personen gebaut werden.

## Entscheidung

Backend in **Go**

## Verworfen

Rust (Stärken zahlen sich bei einer Web-Business-Anwendung nicht aus, Entwicklung deutlich langsamer); TypeScript/Node; PHP/Symfony; Java/Kotlin

## Folgen

Ein statisches Binary ohne Laufzeit; Nebenläufigkeit für Worker und SSE ohne Zusatzdienst. Das Ökosystem für Business-Anwendungen ist kleiner als bei Java oder TypeScript; was fehlt, bauen wir schlank selbst (E43).
