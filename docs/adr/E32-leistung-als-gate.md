# E32: Leistung ab dem ersten Code als Gate

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E32
- **Architektur:** [`architektur.md`](../architektur.md) §11

## Kontext

Architekturfehler, die erst im Lasttest am Ende auffallen, sind teuer.

## Entscheidung

**Leistung wird ab dem ersten Code gemessen, als Gate:** Verfehlt eine Änderung einen Zielwert oder wird etwas um mehr als 10 % langsamer, ist die Aufgabe nicht fertig (H11)

## Verworfen

je Meilenstein messen; erst in Meilenstein 4

## Folgen

H11 baut Lastwerkzeug und Datensatz vor dem ersten Code von PL. Eine Aufgabe ist nicht fertig, wenn ein Ziel verfehlt wird oder etwas um mehr als 10 % langsamer wird.
