# E25: Schlankes Paket je Practice, erweiterbar

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E25
- **Architektur:** [`architektur.md`](../architektur.md) §4

## Kontext

Alle 34 Practices sollen früh da sein und später schnell in die Tiefe wachsen.

## Entscheidung

**Je Practice zuerst ein schlankes Paket** (Register, Status, Maßnahmen, Verknüpfungen, Bericht), nur aus Kernmitteln gebaut. Das Paketformat muss so gebaut sein, dass **ein Paket ein anderes erweitern** kann (Felder, Status, Übergänge, Formulare, Regeln, Ansichten, Berichte) und das Basispaket **ohne Verlust aktualisierbar** bleibt. Volle Tiefe kommt so schnell und einfach als Erweiterungspaket, für alle 34 Practices

## Verworfen

volle Tiefe für alle 34 sofort; nur Practices mit Gegenstück in GLPI

## Folgen

Das Paketformat muss Erweiterungen durch andere Pakete tragen; umgesetzt mit den drei Schichten (E37).
