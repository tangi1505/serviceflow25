# E36: IDs als UUIDv7

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E36
- **Architektur:** [`architektur.md`](../architektur.md) §3

## Kontext

Middleware soll IDs vorab erzeugen können; Pakete werden zwischen Instanzen exportiert; IDs sollen nicht erratbar sein.

## Entscheidung

**IDs sind UUIDv7**

## Verworfen

`bigint` intern und UUID nach außen; `bigint` überall

## Folgen

Zeitlich sortiert, darum gute Index-Lokalität und Partitionierung über den Zeitanteil (E47).
