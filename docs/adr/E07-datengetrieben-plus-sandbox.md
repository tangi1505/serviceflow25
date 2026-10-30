# E7: Datengetriebene Plattform plus Sandbox

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E7
- **Architektur:** [`architektur.md`](../architektur.md) §2, §4

## Kontext

Jede der 34 Practices soll abbildbar und erweiterbar sein, ohne den Kern anzufassen und ohne Neustart.

## Entscheidung

**Datengetrieben plus Sandbox:** Entitätstypen, Felder, Beziehungen, Formulare, Listen, Rollen, Statusmodelle und Regeln sind Daten, in der Oberfläche definiert; eigene Logik läuft in einer Sandbox. Jede Practice ist ein Paket

## Verworfen

Code-Plugins als eigene Prozesse (gRPC) als erster Weg; bleibt als späterer zweiter Weg denkbar (V11)

## Folgen

Der Kern kennt keine Fachlichkeit; Incident, Change und Asset sind Pakete aus Definitionen. Fehlt einem Paket etwas, ist das eine Aufgabe für den Kern, nicht Sondercode im Paket.
