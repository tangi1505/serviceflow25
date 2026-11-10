# Architektur: itsm (Arbeitstitel)

**Stand:** 2026-10-20, Entwurf aus Hauptplan H6. Nichts davon ist gebaut oder gemessen. Jede
Weiche ist eine Entscheidung im [Hauptplan](0_maintask.md) §1; hier steht, wie sie
zusammenspielen. Die Begründung je Entscheidung steht in [`adr/`](adr/). Grundlage sind die Karte
der Practices mit den Kernfähigkeiten `KF1` bis `KF19` (Hauptplan §2.1), das
[Funktionsinventar](funktionsinventar-glpi.md) mit den Befunden B1 bis B6 und die Leistungsziele
`LZ1` bis `LZ13` (Hauptplan §2.5). Nachgezogen am 2026-10-28: was mit WF2, WF3, WF4, WF8 und
PL41 gebaut ist und vom Entwurf abweicht (Bausteine §2, Ereignisse §7, Workflow §8).

> **Die Architektur in vier Sätzen:** Ein Go-Binary mit den Rollen `api` und `worker` vor
> PostgreSQL, Valkey und einem Dateispeicher. Jeder Entitätstyp ist eine Definition aus drei
> Schichten und hat eine echte Tabelle mit echten Spalten, die der Kern selbst anlegt und ändert.
> Rechte werden zu einer SQL-Bedingung, Regeln und Formeln zu CEL-Ausdrücken, schwere Logik zu
> WASM-Modulen. Massendaten laufen über die Bulk-API als `COPY` und Abgleich im Worker, damit die
> Oberfläche auch während eines Massenlaufs schnell bleibt.

---

## 1. Überblick

```
                  Browser (React)                Middleware (Flowzilla …)     MCP (lokal)
                       │  REST, SSE                    │  REST, Bulk, Ereignisse     │
                       ▼                               ▼                             ▼
            ┌──────────────────────────── itsm api (n Instanzen, zustandslos) ─────────────────┐
            │ HTTP · Anmeldung · Rechte · Abfragen · Schreiben · Bulk bis 1.000 · SSE · OpenAPI │
            └────┬───────────────────────┬──────────────────────────┬──────────────────────────┘
                 │ SQL (pgx)             │ Cache, Sitzungen,         │ Dateien
                 ▼                       │ Pub/Sub                   ▼
            PostgreSQL ◀─────────────────┼──────────────────────  S3-API / Dateisystem
            Kerntabellen, Typtabellen,   ▼
            Outbox, Audit, River-Jobs   Valkey
                 ▲                       ▲
                 │ SQL (pgx), eigener Pool│
            ┌────┴───────────────────────┴───────── itsm worker (m Instanzen) ──────────────────┐
            │ River-Jobs · Bulk-Abgleich (COPY) · Outbox-Verteiler · Webhooks · SLA-Uhren ·     │
            │ Regeln · Mail ein/aus · Discovery-Quellen · WASM-Sandbox · Partitionspflege      │
            └───────────────────────────────────────────────────────────────────────────────────┘
```

- **Ein Binary, zwei Rollen** (E35). `itsm serve --role=api`, `--role=worker` oder beides. In
  Compose läuft eine Instanz mit beiden Rollen, im Cluster skalieren sie getrennt. Dazu
  Unterbefehle für Betrieb: `migrate`, `package install`, `user create`, `check`.
- **Abhängigkeiten im Betrieb:** PostgreSQL, Valkey (Pflicht, E20), ein S3-Speicher oder das
  Dateisystem (E22; welcher S3-Speicher, ist Hauptplan V23).
  Sonst nichts; River läuft in PostgreSQL (E19).
- **`api` ist zustandslos.** Sitzungen, Ratenbegrenzung und Pub/Sub liegen in Valkey. Damit wächst
  der Durchsatz mit der Zahl der Instanzen (LZ12).
- **Schwere Arbeit macht der Worker.** Großer Bulk, Discovery, Webhooks und Mail laufen dort, mit
  eigenem Verbindungspool und eigener Obergrenze. So bleibt die Oberfläche bei LZ9, auch während
  eines Massenlaufs (LZ10).

---

## 2. Bausteine

Ein Go-Modul, je Baustein ein Paket unter `internal/`. Jeder Baustein hat eine schmale
Schnittstelle in seinem Wurzelpaket. Andere Bausteine sehen nur diese Schnittstelle, nie seine
Tabellen. Abhängigkeiten zeigen nur nach unten; ein Linter (`depguard`) prüft das.

| Ebene | Baustein | Paket | Aufgabe | KF | Plan |
| --- | --- | --- | --- | --- | --- |
| Eingang | HTTP | `internal/httpapi` | Router (`chi`), Middleware, OpenAPI-Erzeugung, SSE, Fehlerformat | — | PL |
| Eingang | Anmeldung | `internal/authn` | OIDC/SAML, LDAP-Sync, lokale Konten, Sitzungen, API-Zugänge (OAuth 2) | — | PL |
| Fach-Kern | Meta-Modell | `internal/meta` | Typen, Felder, Beziehungsarten, Status, Formulare, Ansichten, Übersetzungen; die drei Schichten zur wirksamen Definition zusammenführen (§4) | KF1, KF16, KF19 | PL |
| Fach-Kern | Schema | `internal/schema` | Tabellen je Typ anlegen und online ändern, Indexe, Partitionen (§3) | KF1 | PL |
| Fach-Kern | Rechte | `internal/authz` | Organisationsbaum, Rollen, Reichweiten, Feldrechte; übersetzt in SQL (§5) | KF9 | PL |
| Fach-Kern | Datensätze | `internal/records` | Lesen, Schreiben, Validieren, berechnete Felder, Summenfelder über verweisende Datensätze (E118), Versionen, Konflikterkennung (E29), Änderungen in Mengen über Kennung und Version (`:batch`, E126), Kosten und Zeiten als Untersätze, Zeitleiste mit öffentlichen und privaten Einträgen (E121) | KF1, KF6, KF17, KF18 | PL |
| Fach-Kern | Abfragen | `internal/query` | Parameter und JSON-Abfrage nach SQL, Seiten per Schlüssel, Aggregate (§6.3) | KF11 | PL |
| Fach-Kern | Beziehungen | `internal/relations` | Beziehungen mit Typen, Graph-Abfragen, Auswirkungsanalyse | KF2 | PL |
| Fach-Kern | Bulk | `internal/bulk` | Abgleich über Quelle und Schlüssel, `COPY`, nur Änderungen schreiben, Vollstand (§6.4) | KF12 | PL |
| Fach-Kern | Formatierter Text | `internal/richtext` | HTML aus dem Editor mit fester Liste erlaubter Elemente bereinigen (bluemonday), Bilder finden, Klartext für Mail, Liste und Export (E127) | — | UI |
| Fach-Kern | Export | `internal/export` | Liste als CSV, XLSX (excelize) oder ODS mit den Rechten der Person, über 100.000 Zeilen als Job mit Datei, jeder Export im Ereignisprotokoll (E125) | — | UI |
| Fach-Kern | Audit | `internal/audit` | Verlauf je Feld, partitioniert | — | PL |
| Fach-Kern | Ereignisse | `internal/events`, `internal/webhooks` | Outbox, Verteiler in Stapeln mit Abnehmern je eigener Position, endgültige Fehlschläge (E112), abrufbarer Strom, SSE (§7); Webhooks mit Abonnements, Signatur und Zustell-Jobs (E100) | KF13 | PL |
| Ablauf | Workflow | `internal/workflow` | Statusmodell, Übergänge, Genehmigungen, Workflow-Provider (§8) | KF3, KF4 | WF |
| Ablauf | Fristen | `internal/calendar`, `internal/sla` | Kalender mit Arbeitszeit und Feiertagen (gebaut, E113); SLAs und Uhren mit Pause und Verstoß (gebaut, E114); Eskalationsstufen als Regeln mit Auslöser `clock` (gebaut, E115) | KF5 | WF |
| Ablauf | Regeln | `internal/rules`, `internal/rules/engine`, `internal/recurrence` | CEL für Bedingungen (E45); die Regeln „wenn, dann“ vor dem Schreiben und als Job danach, nach Zeitplan (gebaut, E111); wiederkehrende Vorgänge (gebaut, E119) | KF7 | WF |
| Ablauf | Formulare | `internal/forms` | Kerntyp `core.form`, Prüfung des Aufbaus, Absenden mit erstem Eintrag (UI11), Bedingungen und Übersetzung (UI12), angeboten im Portal oder Arbeitsplatz, Fragen nach Rollen und feste Werte (SD9, E167); Formularziele (was ein Formular anlegt), Zugriff auch anonym mit Captcha, Umfragen als Formular mit Auswertung | KF8, KF15 | UI, SD |
| Ablauf | Benachrichtigung | `internal/notify` | Einstellungen mit übersetzbaren Vorlagen, Empfänger aus Rollen, Gruppen, Personen und den Verantwortlichen der Gruppen einer Rolle (`actors.<rolle>.responsible`, SD-Plan N10), Bedingung über den Datensatz und die Rollen des Auslösers (`by`, E165), Verweise in Platzhaltern mit ihrem Titel (SD8), Mail hinaus über SMTP (gebaut, E120); Mail hinein (IMAP), Browser (SD) | KF14 | WF, SD |
| Ablauf | Textbausteine | `internal/snippets` | Kerntyp `core.snippet`; Prüfung der Platzhalter beim Speichern, Einsetzen mit den Rechten der schreibenden Person über die Platzhalter der Benachrichtigungen, als formatierter Text (gebaut, E166) | — | SD |
| Erweiterung | Pakete | `internal/packages` | Paketformat, Installation, Aktualisierung, Export | KF16 | EX |
| Erweiterung | Sandbox | `internal/sandbox` | WASM per `wazero`, Host-Funktionen, Grenzen (§9) | KF6 | EX |
| Erweiterung | Discovery | `internal/discovery` | Quellen (Proxmox, VMware, Kubernetes, SNMP, LDAP) als Jobs, liefern an `bulk` | KF12 | CM |
| Grund | Jobs | `internal/jobs` | River, Zeitpläne, Warteschlangen mit Vorrang | — | PL |
| Grund | Suche | `internal/search` | Volltext hinter Schnittstelle, Umsetzung PostgreSQL (E21) | — | PL |
| Grund | Dateien | `internal/files` | Dateien hinter Schnittstelle, S3 oder Dateisystem (E22) | — | PL |
| Grund | Cache | `internal/cache` | Zwei Stufen: im Prozess und Valkey, Invalidierung über Pub/Sub | — | PL |
| Grund | Datenbank | `internal/db` | Pools, Transaktionen, `sqlc`-Code der Kerntabellen, eigener SQL-Builder (E43) | — | PL |
| Grund | Plattform | `internal/platform` | Konfiguration, `slog` als JSON (E28), Metriken, Traces, Health | — | BT |

**Was B1 bis B6 aus dem Inventar hier bedeuten:**

- **B1:** Akteure, Aufgaben, Kosten, Genehmigungen und Umfragen gibt es einmal im Kern. Jeder Typ
  kann sie bekommen, nicht nur Tickets.
- **B2:** Die 28 Fähigkeiten eigener Asset-Typen in GLPI werden **Merkmale**. Das sind kleine
  Definitionsbausteine wie „hat Verlauf", „hat Dokumente", „ist reservierbar" oder „hat
  Rack-Platz". Ein Typ zieht sie per Name an. Merkmale sind selbst Teil eines Pakets.
- **B3:** Eine Auswahlliste ist ein Typ mit dem Merkmal „Auswahlliste". Es gibt keine eigene
  Tabelle je Liste. Ein Eintrag ist für jede angemeldete Person an seinem Knoten und darunter
  lesbar, an der Wurzel für alle (SD-Plan N5, seit SD1); wer ein eigenes Recht auf die Liste hat,
  sieht zusätzlich, was das Recht gibt. Allgemein kann jeder Typ solche Leser-Regeln mitbringen
  (`readers`, SD-Plan N12, seit SD16a): je Regel eine Bedingung in CEL, die nach SQL übersetzbar
  ist, und auf Wunsch Rollen. Ohne Rollen lesen Personen an ihrem Knoten und darunter, mit Rollen
  deren Inhaber und die Mitglieder ihrer Gruppen; so liest jeder einen veröffentlichten Artikel
  „für alle“. Eine Auswahlliste ist die Regel ohne Bedingung. Eine Regel kann außer dem Lesen
  auch das Kommentieren erlauben (`actions: [read, comment]`, SD-Plan N15, seit SD18).
- **Vorschläge (seit SD18, SD-Plan N13):** Ein Typ sagt unter `suggest`, bei welchen Typen er
  vorgeschlagen wird (auch über ein Merkmal), aus welchen ihrer Felder gesucht wird, mit welcher
  Beziehungsart „Verknüpfen“ arbeitet und was „Übernehmen“ ins Formular setzt. Der Kern sucht
  mit „jedes Wort zählt“ und den Rechten der Person (`POST /api/v1/records/{typ}:suggest`); der
  vorgeschlagene Typ kennt den anderen, nicht umgekehrt. Ein Artikel schlägt sich so für jeden
  Vorgang vor, ohne dass Störung oder Anfrage das Wissen kennen. Das Merkmal `rating` hält
  „hat geholfen“ je Person.
- **B4:** Alle 34 Regelsammlungen aus GLPI sind Regeln in `internal/rules`. Sie unterscheiden
  sich nur im Auslöser: Anlegen, Ändern, Import, Anmeldung oder Mail.
- **B5:** Die Feldzustände Pflicht, versteckt, vorbelegt und schreibgeschützt hängen am Status
  und an der Formularvariante. Eigene Vorlagen-Klassen braucht es nicht.
  Ein Übergang kann einer Rolle am Datensatz vorbehalten sein (`by: [requester]`) und einen Text
  verlangen, der als Eintrag entsteht (`comment`); so bestätigt oder lehnt der Anforderer eine
  Lösung im Portal ab (SD-Plan N8, seit SD2).
- **B6:** Die 48 automatischen Aktionen aus GLPI sind Zeitpläne in `internal/jobs` oder Zeitregeln
  im Statusmodell.

---

## 3. Daten in PostgreSQL

### 3.1 Zwei Arten von Tabellen

- **Kerntabellen** mit festem Schema, Migrationen per `goose`, Zugriff per `sqlc` (E18). Das sind
  Konten, Organisationsbaum, Rollen, Definitionen, Pakete, Beziehungen, Audit, Outbox, Dateien,
  Jobs, die Einträge aller Auswahllisten (`list_entry`, B3, E93) und die Kosten- und
  Zeiteinträge aller Typen (`cost`, B1, E97), die Einträge der Zeitleisten (`entry`, E121) und die Übersetzungen von Feldwerten
  (`translation`, E98), die Webhook-Abonnements (`webhook`, E100) und das Ereignisprotokoll
  (`security_event`, E102). Personen und Gruppen sind
  seit E79 Typen des Kernpakets `core` (Typtabellen).
- **Typtabellen** (E33): je Entitätstyp eine Tabelle `rec_<typ>`. Der Baustein `schema` legt sie
  an und ändert sie. Gelesen und geschrieben wird über den eigenen SQL-Builder (E43). Jede
  Typtabelle hat dieselben Systemspalten:

| Spalte | Typ | Zweck |
| --- | --- | --- |
| `id` | `uuid` (v7, E36) | Schlüssel; trägt die Zeit, darum auch Partitionsschlüssel (E47) |
| `org_path` | `ltree` | Ort im Organisationsbaum, für Rechte (§5) |
| `tenant` | `ltree` | Mandant des Knotens, `root` ohne Mandanten; setzt der Trigger `set_tenant` (PL46). Bereich für eindeutige Werte und externe Schlüssel, nicht für Rechte |
| `version` | `bigint` | steigt bei jeder Änderung; Konflikterkennung über `If-Match` (E29) |
| `state` | `text` | Status aus dem Statusmodell, wenn der Typ eins hat |
| `source`, `external_key` | `text` | Herkunft für den Abgleich (E38); eindeutig zusammen je Mandant, wo gesetzt |
| `row_hash` | `bytea` | Prüfsumme der fachlichen Spalten; Bulk schreibt nur bei Änderung |
| `search` | `tsvector` | erzeugt aus den als durchsuchbar markierten Feldern (E21) |
| `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at` | | Lebenslauf, Papierkorb |
| `sla_due_at`, `sla_warn_at`, `sla_paused` | `timestamptz`, `boolean` | nächste Frist der laufenden SLA-Uhren, ihr 75-%-Punkt im Kalender der Uhr, „nur angehalten“; pflegt `internal/sla` in der Transaktion der Uhren über `records.SetDeadlineTx`, ohne Version und Verlauf; Index `(sla_due_at, id)` für Sortieren und Blättern (UI34, Migration 41) |

- **Fachfelder** sind echte Spalten mit passendem Typ: `text`, `numeric`, `timestamptz`, `date`,
  `boolean`, `uuid` als Verweis auf einen anderen Typ und `jsonb` nur für strukturierte Werte. Ein
  Mehrfachwert ist eine Beziehung, keine Liste in einer Spalte. Die **laufende Nummer** (Feldtyp
  `sequence`, PL49) ist eine `bigint`-Spalte, eindeutig je Mandant; der Kern zieht sie beim Anlegen
  aus einer PostgreSQL-Sequenz je Typ, Feld und Mandant (Name `seq_<Hash>`, beim ersten Gebrauch
  unter einer Sperre angelegt), das Präfix steht in der Definition. Lücken nach abgebrochenen
  Transaktionen sind erlaubt; nur ein Import mit Quelle darf eine Nummer mitbringen.
- **Beziehungen** liegen in einer Kerntabelle `relation` mit Art, Quelle und Ziel. Sie hat Indexe
  in beide Richtungen. Graph-Abfragen laufen als rekursive CTE mit Tiefengrenze (KF2).

### 3.2 Schema ändern, ohne anzuhalten

- **Feld dazu:** `ALTER TABLE … ADD COLUMN` ohne flüchtigen Vorgabewert. Das schreibt die Tabelle
  nicht um. Indexe entstehen danach per `CREATE INDEX CONCURRENTLY` als Job.
- **Feld weg:** Das Feld wird erst ausgeblendet und dann in einem Wartungsjob gelöscht. Bis dahin
  ist es wiederherstellbar.
- **Feldtyp ändern:** Eine neue Spalte wird angelegt, ein Job füllt sie, dann wird umgeschaltet.
  Ein Typwechsel ist also nie ein `ALTER COLUMN TYPE` auf einer großen Tabelle.
- **Jede Schemaänderung** steht als Datensatz in einer Kerntabelle. Sie läuft unter einer
  Advisory Lock, damit zwei Instanzen nicht gleichzeitig ändern.

### 3.3 Partitionen und Aufbewahrung

- **Je Typ einstellbar** (E47). Das Paket legt fest, ob ein Typ nach Erstellungsmonat
  partitioniert wird. Weil UUIDv7 die Zeit trägt, ist der Bereich des Schlüssels die Partition.
- **Immer partitioniert:** Audit nach Monat, Outbox nach Tag (Aufbewahrung 7 Tage, einstellbar,
  E46), River-Jobs nach der Aufräumregel von River.
- **Ein Wartungsjob** legt Partitionen im Voraus an und löscht oder lagert abgelaufene aus. Die
  Aufbewahrung je Typ ersetzt GLPIs Einstellung und Aufgabe zum Löschen (G175, G244 im Inventar).

---

## 4. Definitionen in drei Schichten

E37 und KF16:

```
Basispaket     incident@1.4      Typ, Felder, Status, Formulare, Regeln, Rollen, Berichte
   ▲ extends
Erweiterung    incident-major@1  + Feld „Major", + Status „Krisenstab", + Regel
   ▲
Kundenschicht  (aus der Oberfläche)  + Feld „Dienststelle", Formular umgestellt
   =
Wirksame Definition  ← im Prozess und in Valkey gecacht, Hash als Version
```

- **Ein Paket** ist ein Verzeichnis oder Archiv. Es enthält `package.yaml` (Name, Version,
  Abhängigkeiten, `extends`), Definitionen als YAML, optional WASM-Module, Übersetzungen und
  Beispieldaten.
- **Eine Erweiterung** darf hinzufügen und eng begrenzt ändern: Beschriftungen, Pflicht,
  Reihenfolge, Vorgaben. Löschen darf sie nichts, was ein anderes Paket mitbringt; sie darf es nur
  ausblenden.
- **Die Kundenschicht** hält alles, was in der Oberfläche geändert wurde, als Liste von
  Änderungen. Sie ist exportierbar und wird damit zu einem Paket. Gebaut mit PL43
  (`internal/meta/customer.go`, `internal/customer`): eigene Felder heißen `k_…`, an Feldern
  eines Pakets nur Beschriftung, Pflicht, Vorgabe, Reihenfolge, Optionen und Ausblenden;
  ein Paket-Update mit Konflikten hält an, mit `--force` werden die Änderungen stillgelegt.
  Die Oberfläche dazu ist der Typ- und Feld-Editor (UI25): jede Aktion eine Änderung, die Liste
  der Änderungen mit Zurücknehmen und der Export als Datei. Seit UI26.1 ändert die Kundenschicht
  auch das Statusmodell: eigene Status und Übergänge, Beschriftung und Anfangsstatus der
  Paketstatus, Übergänge des Pakets umleiten oder stilllegen (`Transition.Disabled`). Seit
  UI26.2 hält sie auch die Anordnung des Workflow-Graphen (`Type.Layout`, Operation
  `set_layout`): nur die Oberfläche liest sie, die Engine nie.
- **Aktualisieren:** Ein neues Basispaket ersetzt nur seine eigene Schicht. Danach baut der Kern die
  wirksame Definition neu. Widersprüche wie ein Feld, das die Kundenschicht umbenannt hat und das
  das Basispaket entfernt, zeigt er vor der Aktivierung an. Daraus folgende Schemaänderungen laufen
  nach §3.2.
- **Merkmale** (B2) sind Definitionsbausteine, die ein Typ per Name einbindet, etwa
  `traits: [history, documents, reservable]`. Die mit Verhalten (`list`, `history`,
  `documents`, `notes`, `reservable`) stehen im Kern; ein Paket bringt eigene als Feldbündel
  unter `traits/<name>.yaml` mit (E94), seit SD1 auch mit Rollen am Datensatz (`actors:`, SD-Plan
  N6); der Typ bekommt dann das Kern-Merkmal `actors` dazu. Ein Typ darf das Merkmal eines
  anderen Pakets nennen (`traits: [sd.ticket]`), wenn sein Paket davon abhängt; das Installieren
  löst es aus den installierten Paketen auf (SD-Plan N7).
- **Startdaten** (SD-Plan N9, seit SD3): Ein Paket bringt unter `data/*.yaml` Datensätze und Regeln
  mit, etwa Kalender, Fristen, Eskalationsstufen und Wartegründe. Jeder Eintrag hat einen Schlüssel;
  `@schlüssel` verweist auf einen Eintrag davor, `@paket.schlüssel` auf einen eines Pakets, von dem
  es abhängt. `internal/seed` legt sie beim Installieren je Mandant einmal an, bei einem neuen
  Mandanten auch, und merkt sich in `package_seed`, was wozu wurde. Danach gehören sie dem Kunden:
  ein Update legt nur an, was noch nie angelegt war, Gelöschtes kommt nicht wieder. Regeln heißen
  `<paket>_<schlüssel>`, in einem Mandanten mit dessen Namen dahinter, weil Regelnamen über alle
  Mandanten eindeutig sind.
- **Beziehungsarten** nennen in `from` und `to` Typen, `*` oder seit SD4 ein Merkmal
  (`trait:sd.ticket`, SD-Plan N11): „betrifft“ aus `sd` gilt so für jeden Vorgangstyp. Die
  Detailseite zeigt die Bezüge eines Datensatzes im ersten Reiter unter dem Formular, die
  Zeitleiste im letzten (UI-Plan E38); dazwischen bei Typen mit Verlauf die Fassungen mit
  Vergleich Wort für Wort und Wiederherstellen (SD16b).

---

## 5. Rechte

E34 und KF9:

- **Organisationsbaum** als `ltree`-Pfad, dazu ein GiST-Index auf `org_path` in jeder Typtabelle.
  Sichtbarkeit nach Unterbaum ist eine Bedingung `org_path <@ $1`.
- **Eine Rolle** besteht aus Berechtigungen. Jede Berechtigung hat fünf Teile:
  - **Typ;**
  - **Aktion:** lesen, anlegen, ändern, löschen oder ein Sonderrecht des Pakets;
  - **Reichweite:** eigene, Gruppe, beteiligt, Organisationseinheit, Unterbaum oder alle;
    eine Rolle darf für Typ und Aktion mehrere Reichweiten halten, sie gelten mit ODER
    (Kern-Plan E58);
  - **Bedingung** in CEL, etwa „nur nicht geschlossene";
  - **Feldrechte.**
- **Persönliche Einstellungen** (Sprache an der Person, Zeitzone, Theme, Darstellung hell, dunkel oder wie das System, Zeilenhöhe der Listen, Vorbelegung je Konto,
  `internal/prefs`) gehen vor den Vorgaben am Baum (`ui.*`, E131). Nur im Browser liegen je
  Konto die zuletzt geöffneten Datensätze der Befehlszeile (Typ, ID, Titel; UI31, UI-Plan E36);
  die Abmeldung löscht sie.
- **Den Baum selbst** ändern die Sonderrechte `manage_org` (Knoten) und `manage_settings`
  (Einstellungen) am Pseudotyp `core.org`, mit der Reichweite der Zuweisung (Kern-Plan E59).
- **Zuweisung:** Person oder Gruppe bekommt eine Rolle für einen Knoten des Organisationsbaums.
  Regeln weisen das bei der Anmeldung aus LDAP- oder OIDC-Gruppen zu (G121).
- **Übersetzung:** Je Person und Typ entsteht einmal eine SQL-Bedingung plus eine Liste erlaubter
  Spalten. Sie wird im Prozess und in Valkey gecacht. Jede Rollenänderung invalidiert den Cache
  über Pub/Sub. CEL-Bedingungen werden dafür in SQL übersetzt; was sich nicht übersetzen lässt,
  lehnt der Editor beim Speichern der Rolle ab. Der Rollen-Editor (UI27) zeigt die Rechte einer
  Rolle als Matrix Typen × Aktionen mit der Reichweite je Zelle und pflegt die Zuweisungen;
  jede Änderung ist ein Aufruf der API und gilt sofort.
- **Schreiben** prüft dieselben Regeln gegen den Datensatz vor und nach der Änderung.
- **Keine Row-Level Security** in PostgreSQL. Der Grund ist Connection-Pooling und Bulk (E34). Ein
  Prüftest stellt sicher, dass jede Abfrage über `query` oder `records` läuft. Direktes SQL auf
  Typtabellen außerhalb dieser Bausteine bricht den Build (Linter-Regel).

### 5.1 Mandanten (E136)

Deine Vorgabe vom 2026-10-29: In einer Installation sollen mehrere Mandanten möglich sein, deren
Daten und Nutzer vollständig voneinander getrennt sind; nur Administratoren dürfen auf mehrere
Mandanten zugreifen. E16 (eine Installation je Kunde) bleibt als Ausweg für eine physische
Trennung, ist aber nicht mehr die einzige Form. Die offenen Punkte stehen als V24 bis V28 im
Hauptplan, hier mit ihrer Beta-Antwort.

- **Ein Mandant ist ein Knoten direkt unter der Wurzel** mit der Eigenschaft `tenant`. Darunter
  baut jeder Mandant seinen eigenen Organisationsbaum. Ohne Mandantenknoten ist die Wurzel der
  einzige Mandant und alles bleibt wie bisher (E138). `itsm tenant create <name> --adopt
  [--admin <konto>]` legt den ersten an und verschiebt Baum, Datensätze, Zuweisungen,
  Anmelderegeln und Verzeichnisse hinein; danach entstehen direkt unter der Wurzel nur noch
  Mandanten.
- **Was zum Mandanten gehört:** alle Datensätze (ihr `org_path` liegt unter dem Mandantenknoten),
  Konten, Personen, Gruppen, Zuweisungen, Einstellungen am Baum, Formulare, gespeicherte Suchen,
  Webhooks, API-Clients, Dateien, Zeitleiste, Verlauf, Ereignisse, Jobs, Exporte und das
  Ereignisprotokoll.
- **Was zur Installation gehört:** Pakete und Typdefinitionen samt Kundenschicht (PL43, Beta V26:
  für alle Mandanten gleich), Rollen als Mengen von Berechtigungen (zugewiesen wird je Mandant),
  die Konten der Installations-Admins und der Betrieb des Kerns (Aufbewahrung, Messung).
- **Die Grenze steht in jeder Abfrage.** Die Sitzung trägt genau einen Mandanten. Der Rechtefilter
  (E34) setzt vor jede Reichweite `org_path <@ <Mandant>`; damit endet auch „alle“ an der Grenze.
  Ein Datensatz eines anderen Mandanten ist 404, nie 403. Nebentabellen ohne `org_path` bekommen
  eine Spalte `tenant` und dieselbe Bedingung.
- **Konten gehören zu einem Mandanten.** Anmeldung, LDAP- und OIDC-Abgleich und API-Clients wirken
  nur in ihm; die Zugehörigkeit steht in `account.tenant` (PL44). V25 (entschieden 2026-10-30,
  E138): Benutzernamen bleiben in der Installation eindeutig, die Anmeldung braucht keine
  Mandantenwahl.
- **Admins über Mandanten** sind Konten der Installation (ohne `account.tenant`) mit
  Zuweisungen an Mandantenknoten oder an der Wurzel. V27 (entschieden 2026-10-30, E138): Sie
  arbeiten immer in genau einem Mandanten und wechseln wie den Rechtekontext (PL10), im Kopf der
  Oberfläche oder über `POST /api/v1/auth/context` mit `tenant`; der Wechsel steht im
  Sicherheitsprotokoll, der zuletzt gewählte gilt bei der nächsten Anmeldung. Rollen, Typen und
  die Einstellungen der Wurzel ändert nur, wer das Recht aus einer Rolle an der Wurzel hat.
  Auswertungen über Mandanten kommen mit PL48.
- **Eindeutigkeit gilt je Mandant** (PL46, E140): eindeutige Felder und der externe Schlüssel
  (`source`, `external_key`) bekommen den Mandanten aus der Spalte `tenant` in den Index
  (`(tenant, feld)`, `(tenant, source, external_key)`); die Spalte setzt ein Trigger aus dem
  Knoten, `root` ohne Mandanten. Ein Wert in Mandant B sperrt ihn nicht für A und verrät ihn
  nicht; Datensätze an der Wurzel bilden den Bereich `root`.
- **Anlegen landet im richtigen Knoten** (PL46, E140): im Knoten aus dem Rechtekontext der
  Sitzung, sonst im Knoten der Person, wenn sie dort anlegen darf, sonst im einzigen Knoten mit
  Anlegerecht; bleibt eine Wahl, verlangt die API den Knoten (422) und das Formular fragt
  („Ablegen in“). Nie stillschweigend an der Wurzel oder im Mandanten. Ein Formular legt im
  Knoten des Formulars an.
- **Ereignisstrom, SSE, Webhooks und Benachrichtigungen** liefern nur Ereignisse des eigenen
  Mandanten (PL45). Der Valkey-Kanal `events` trägt alle Mandanten, gefiltert wird je Abonnent;
  die übrigen Schlüssel hängen an Sitzung, Konto oder Datensatz und mischen keine Mandanten.
- **Regeln, Webhooks, API-Clients, Verzeichnisse und Anmelderegeln** gehören einem Mandanten
  (PL45, E139). Regeln der Installation (ohne Mandant) wirken in jedem Mandanten und legt nur an,
  wer das Recht an der Wurzel hat. Ein Konto aus OIDC gehört zum Mandanten der ersten passenden
  Anmelderegel. Das Ereignisprotokoll trägt den Mandanten jedes Eintrags; Einträge der
  Installation sehen nur Admins mit dem Recht an der Wurzel.
- **Gemeinsame Stammdaten gibt es nicht** (V28, entschieden 2026-10-30, E139). Kalender,
  Feiertage, Auswahllisten, SLAs und Kataloge hat jeder Mandant selbst; die Vorgaben der Pakete
  kopiert der Kern beim Anlegen eines Mandanten und bei jeder Paket-Installation in jeden
  Mandanten, der Schlüssel eines Listeneintrags ist je Mandant eindeutig.
- **Wo die Daten liegen, ist je Installation einstellbar** (deine Entscheidung zu V24,
  2026-10-29, E137): entweder alle Mandanten in einer Datenbank, getrennt durch die Grenze im
  Rechtefilter, oder jeder Mandant in einer eigenen Datenbank mit eigener Sicherung und eigenen
  Zugangsdaten. Die Logik oben gilt in beiden Fällen gleich; der Kern wählt je Zugriff die
  Datenbank (E141): Jeder Dienst hält einen `db.Router` statt eines Pools und fragt `Control()`
  für die Installation oder `Tenant(ctx)` für den Mandanten der Anfrage; mit einer Datenbank sind
  beide dieselbe. Ein Test wacht darüber, dass kein Dienst einen Pool als Feld hält (PL47a).
  Jede Datenbank hat dasselbe Schema; eine Tabelle wird nur auf einer Seite benutzt. Bei eigenen
  Datenbanken kommt eine **Steuer-Datenbank** der Installation dazu: Mandantenverzeichnis (die
  Zugangsdaten je Mandant stehen nur in der Konfiguration), Konten der Installations-Admins mit
  allen ihren Zuweisungen (E142), die Zuordnung Benutzername → Mandant für die Anmeldung
  (`account_home`), Anmelderegeln und OIDC-Identitäten, Pakete und Typdefinitionen samt
  Kundenschicht, Rollen und Rechte, Regeln, API-Clients und Protokolleinträge der Installation,
  die Sicherheits- und Betriebseinstellungen der Wurzel. Jedes Konto hat genau eine
  Heimat-Datenbank; dort liegen seine Person, Gruppen, Zuweisungen und Einstellungen. Ein Admin
  bekommt bei jedem Betreten eines Mandanten dort ein Spiegelkonto ohne Kennwort mit Person
  unter denselben Kennungen, damit die Datensätze des Mandanten auf ihn verweisen können
  (PL47b). Eine Rolle in Gebrauch wird nicht gelöscht; die Nummern des Ereignisprotokolls sind
  zeitgeordnet, damit sich Einträge zweier Datenbanken mischen lassen, und je Datenbank monoton,
  damit ein Uhrsprung zurück die Reihenfolge nicht umdreht (PL47e). Migrationen, Paket-Installationen und
  Schemaänderungen laufen dann für jede Mandanten-Datenbank; River-Jobs, Verteiler und
  Zeitplaner laufen je Datenbank im selben Prozess: Ein Job liegt und läuft in der Datenbank
  der Änderung, die ihn einreiht, und arbeitet an sie gebunden; die Jobansicht zeigt die Jobs
  der Datenbank, in der man arbeitet; Zeitpläne gelten zentral für alle; nur der LDAP-Abgleich
  läuft einmal zentral (PL47c). Betrieb (PL47d, E144): `ITSM_TENANT_MODE=database` und je Mandant
  `ITSM_TENANT_<NAME>_DATABASE_URL`; der Betrieb legt die leere Datenbank an, `itsm tenant create`
  richtet sie ein; fehlt eine URL, startet itsm nicht; gesichert wird je Datenbank mit
  `pg_dump`/`pg_restore`, `itsm tenant check` prüft den Abgleich mit der Steuer-Datenbank; ein
  Webhook der Installation bekommt die Ereignisse aller Mandanten mit einer Position je Datenbank. Der Grenz-Rundgang läuft in jeder CI in beiden Betriebsarten (PL47e, E145). Abgesichert wird beides wie E34: die
  Linter-Regel gegen direktes SQL auf Typtabellen und ein Test, der jede Route mit zwei Mandanten
  in beiden Betriebsarten aufruft und keinen Treffer über die Grenze findet. Für VS-NfD beurteilt
  der IT-Sicherheitsbeauftragte, welche Betriebsart nötig ist; E16 bleibt der äußerste Ausweg.
- **Auswertungen über Mandanten** (E137): Admins mit einem eigenen Recht dafür dürfen Kennzahlen
  und Listen über mehrere Mandanten abrufen. Die Abfrage (JSON-Abfrage mit Gruppierung und
  Aggregation, E40) läuft je Mandant mit den Rechten, die der Admin dort hat, in dessen Datenbank;
  der Kern führt die Ergebnisse zusammen, mit dem Mandanten als Spalte. Es gibt keine Kopie der
  Daten in einer gemeinsamen Datenbank. Jede Auswertung über Mandanten steht im
  Ereignisprotokoll jedes betroffenen Mandanten. Schreiben über Mandanten hinweg gibt es nicht.
  Gebaut mit PL48 (E146): `POST /api/v1/tenants:query`; das Recht `report_tenants` an
  `core.tenant` zählt nur ausdrücklich, an der Wurzel, für Konten der Installation.

**Stand heute (2026-10-30, nach PL46), die Lücken zu diesem Bild:**

1. Behoben mit PL44: „alle“ endet an der Grenze des Mandanten; der Rechtefilter setzt sie vor
   jede Reichweite, und ein Rundgang über jede Route der API belegt das (Kern-Plan PL44).
2. Behoben mit PL44 und PL45: Konten, Ereignisprotokoll, Regeln, Webhooks, API-Clients,
   Verzeichnisse, Anmelderegeln, Jobs von Export und Massenlauf und die Fehler des Verteilers
   tragen ihren Mandanten; Dateien hängen an ihrem Datensatz.
3. Behoben mit PL44 und PL46: ein neuer Datensatz ohne Knoten landet im Knoten des Kontexts,
   der Person oder des einzigen Anlegerechts; bleibt eine Wahl, fragt die Oberfläche.
4. Formulare (UI11) wurden jedem angeboten, der den Zieltyp irgendwo anlegen darf; behoben am
   2026-10-29: angeboten wird ein Formular nur, wer im Knoten des Formulars anlegen darf, und es
   legt dort an.
5. Behoben mit PL46: eindeutige Felder und externe Schlüssel gelten je Mandant (Spalte
   `tenant`).
6. Behoben mit PL45: Ereignisstrom, SSE, Webhooks und Benachrichtigungen enden an der Grenze;
   auch das Recht, alle Ereignisse zu lesen, gilt nur im eigenen Mandanten.

Die Aufgaben dazu stehen im Kern-Plan: PL44 (Mandantengrenze), PL45 (Nebendaten je Mandant),
PL46 (Anlegen und Eindeutigkeit je Mandant), PL47 (Datenbank je Mandant als Betriebsart), PL48
(Auswertungen über Mandanten), nach PL43 und vor den Editoren.

---

## 6. API

### 6.1 Form

E11:

| Pfad | Zweck |
| --- | --- |
| `GET /api/v1/types`, `GET /api/v1/types/{typ}` | wirksame Definitionen |
| `GET /api/v1/records/{typ}` | Liste mit einfachen Parametern (`?state=open&sort=-created_at&fields=…&limit=50&after=…`) |
| `POST /api/v1/records/{typ}:query` | JSON-Abfrage (§6.3) |
| `GET`, `PATCH`, `DELETE /api/v1/records/{typ}/{id}` | ein Datensatz; `ETag` ist `version`, Ändern braucht `If-Match` (E29) |
| `POST /api/v1/records/{typ}` | anlegen |
| `POST /api/v1/records/{typ}:bulk` | Massenabgleich (§6.4) |
| `GET /api/v1/jobs/{id}` | Stand eines asynchronen Auftrags |
| `GET /api/v1/events?after={position}` | abrufbarer Ereignisstrom (§7) |
| `GET /api/v1/events/stream` | SSE für den Browser |
| `POST /api/v1/files`, `GET /api/v1/files/{id}` | Dateien |
| `POST /api/v1/workflow/{typ}/{id}/transitions/{übergang}` | Übergang auslösen |

- **OpenAPI** entsteht zur Laufzeit aus der wirksamen Definition. Feste Endpunkte kommen aus einer
  statischen Beschreibung. Der TypeScript-Client wird daraus erzeugt (§10).
- **Seiten per Schlüssel** (`after=`), nie per Versatz. So bleibt Blättern bei Millionen Zeilen
  gleich schnell (LZ2).
- **Anmeldung:** Der Browser nutzt ein Sitzungscookie nach OIDC. Maschinen nutzen OAuth 2 Client
  Credentials mit einem technischen Nutzer, der Rollen hat wie jeder andere.
- **Ratenbegrenzung** je Zugang in Valkey.

### 6.2 Schreiben

- **Ein Schreibvorgang ist eine Transaktion.** Sie umfasst Validierung, berechnete Felder (CEL,
  beim Schreiben gespeichert, E45), die Zeile, das Audit und das Ereignis in der Outbox, dazu
  die Summenfelder der Datensätze, auf die er zeigt (`rollup`, etwa die Zeit der Aufgaben am
  Vorgang, E118), und die SLA-Uhren (E114).
- **Regeln und Workflow** reagieren auf das Ereignis. Synchron laufen nur Regeln, die den
  Datensatz vor dem Speichern ändern, etwa beim Zuweisen. Alles andere läuft danach als Job. Das
  hält LZ4.

### 6.3 Abfragen

E40:

```json
{
  "where": { "and": [
    { "field": "state", "in": ["new", "assigned"] },
    { "field": "priority", "gte": 3 },
    { "rel": "affects", "to": "ci", "where": { "field": "location.name", "eq": "Rechenzentrum A" } }
  ]},
  "fields": ["number", "title", "priority", "assignee.name"],
  "sort": ["-priority", "created_at"],
  "limit": 50
}
```

- **Dieselbe Sprache** gilt für gespeicherte Suchen, Dashboards, Berichte und die Bedingungen der
  Rechte.
- **Aggregate** (`group`, `count`, `sum`) sind für Dashboards da (KF11).
- **Der Abfragebau** fügt immer die Rechtebedingung (§5) und den Ausschluss des Papierkorbs an.
  Er wählt Indexe nicht selbst; dafür sorgen Indexe, die das Paket je Typ deklariert.

### 6.4 Bulk

E38, E24, KF12:

- **Bis 1.000 Datensätze** schickt der Aufrufer ein JSON-Feld mit Zeilen und bekommt sofort eine
  Antwort je Zeile: angelegt, geändert, unverändert oder Fehler.
- **Mehr als 1.000:** Der Aufrufer schickt einen NDJSON-Strom. Der Server antwortet mit `202` und
  einer Job-ID. Der Strom liegt bis zur Verarbeitung im Dateispeicher.
- **Abgleich:** Erkannt wird über `source` + `external_key`, sonst über die Identifikationsregeln
  des Typs. Das sind CEL-Ausdrücke über Felder wie Seriennummer oder UUID der VM.
- **So schreibt der Worker:**
  1. Er liest die Stapel per `COPY` in eine temporäre Tabelle.
  2. Er rechnet `row_hash`.
  3. Mit `INSERT … ON CONFLICT (source, external_key) DO UPDATE … WHERE rec.row_hash <> excluded.row_hash` schreibt er nur Änderungen.
  4. Audit und Ereignisse entstehen nur für geänderte Zeilen.
  
  Das trägt LZ5 bis LZ8.
- **Vollstand:** Mit `mode=full` und Angabe der Quelle markiert der Abgleich alles als
  verschwunden, was diese Quelle früher geliefert hat und jetzt nicht mehr. Gelöscht wird es nicht.
- **Feldhoheit je Quelle:** Ein Feld, das eine Person von Hand geändert hat, überschreibt die
  Quelle nicht. Das ist der Ersatz für GLPIs gesperrte Felder (G30) und je Typ und Feld einstellbar.
- **Idempotent:** Wiederholt ein Aufrufer denselben Auftrag mit demselben `Idempotency-Key`,
  bekommt er dieselbe Antwort.
- **Unterobjekte** wie die Software eines Computers kommen in derselben Zeile als Liste. Der
  Abgleich behandelt sie als Beziehung mit eigenem Vollstand je Elternteil: 150 Installationen
  eines Computers sind ein Abgleich, nicht 150 Aufrufe.

---

## 7. Ereignisse

E39, E46:

- **Outbox:** Jede Änderung schreibt in derselben Transaktion ein Ereignis in `event` (seit PL24; Position aus einer Sequenz, Leser sehen nur Ereignisse unterhalb des Sichtbarkeitshorizonts, E87).
  Jedes Ereignis bekommt eine fortlaufende Position (`bigint`, Sequenz) und ist nach Tag
  partitioniert.
- **Verteiler im Worker (seit PL41, E112):** Jeder Abnehmer hat seine eigene Position in
  `event_consumer`: `live` (Valkey Pub/Sub für SSE an alle `api`-Instanzen), `webhooks` (signiert,
  Zustellung als River-Job), `rules` (Regeln danach als Job), `notify` (Mails als Jobs, nur wo
  ein Mailserver gesetzt ist, E120). Die SLA-Uhren sind kein Abnehmer: sie bewegen sich in der
  Transaktion des Schreibens (E114). Ein Durchgang sperrt die Zeile des Abnehmers (`FOR UPDATE SKIP
  LOCKED`, so arbeitet je Abnehmer eine Instanz), liest bis zu 1.000 Ereignisse und schreibt die
  Jobs des Stapels und die neue Position in einer Transaktion. Scheitert er, bleibt der Abnehmer
  stehen, behält den Fehler und versucht es wieder; die anderen laufen weiter. Die Aufbewahrung
  wartet auf den langsamsten dauerhaften Abnehmer. Was ein Job nach allen Versuchen nicht schafft,
  steht in `delivery_failure`. Die Outbox bleibt die einzige Quelle; Valkey ist nur der schnelle
  Weg zu Live-Anzeigen.
- **Abrufbarer Strom:** Ein Abonnent liest ab seiner Position, bis 7 Tage zurück. Nach einem
  Ausfall holt er so lückenlos nach. Ist seine Position älter, bekommt er `410` und muss über die
  Bulk-API oder eine Abfrage neu abgleichen.
- **Ereignisse tragen** Typ, ID, Version, geänderte Felder und den Auslöser. Sie tragen nicht den
  ganzen Datensatz; den liest der Abonnent mit seinen eigenen Rechten nach.

---

## 8. Workflow

E8, KF3 bis KF5, KF7:

- **Statusmodell je Typ:** Status, Übergänge, Bedingung je Übergang (CEL), Pflichtfelder und
  Feldzustände je Status und Variante (B5; gebaut als `field_states`, E110), Aktionen beim
  Übergang. Ein Übergang kann Felder im selben Aufruf setzen und eigene feste Werte (`set`,
  E163).
- **Rollen am Datensatz** (gebaut, E109): Anforderer, Beobachter, Bearbeiter und was ein Typ
  braucht, gehalten von Personen, Gruppen oder Lieferanten; Reichweite `involved` in den Rechten.
- **Regeln** (gebaut, E111): vor dem Schreiben in derselben Version oder danach als Job, nach
  Zeitplan oder als Eskalationsstufe einer SLA-Uhr (Auslöser `clock`, E115, nach Anteil oder
  Minuten); Aktionen setzen, zuweisen (auch nachgeschlagen in einem anderen Typ, `lookup`,
  E164), anlegen, benachrichtigen, Übergang (E116), Übergang an den verknüpften Datensätzen
  einer Beziehungsart (`related`, E163); Systemrechte, die Regel steht im Verlauf.
- **Genehmigungen** sind Unterdatensätze mit Stufen, in fester Reihenfolge oder parallel, mit
  Mindestanteil, Vertretung und Erinnerung.
- **Fristen:** Kalender mit Arbeitszeit und Feiertagen sind Typen des Kernpakets (gebaut,
  E113). Ein Service-Level ist ein Datensatz von `core.sla` mit Zieltyp, Ziel, Kalender und
  CEL-Bedingungen für gilt, Pause und erfüllt (gebaut, E114). Eine Uhr ist eine Zeile der
  Kerntabelle `clock` mit Start, gezählter Arbeitszeit, Frist und Zustand; sie bewegt sich in
  der Transaktion des Schreibens. Ein River-Job ist auf die Frist geplant. Bei einer Änderung
  wird er neu geplant (der alte erkennt sich an der Generation); es gibt keinen minütlichen
  Lauf über alle Tickets. Der Verstoß ist ein Ereignis `breach` am Datensatz. Eskalationsstufen
  sind Regeln mit Auslöser `clock` und Anteil am Ziel; der Job der Uhr wartet auf die nächste
  offene Stufe, das Erreichen ist ein Ereignis `escalation`, aus dem der Regel-Abnehmer den
  Job genau dieser Regel macht (E115). Wartegründe (`core.wait_reason`) sind SLAs, die ab
  Beginn des Wartens zählen, mit Stufen für Erinnerungen und Lösung (E116). Zeitregeln im
  Statusmodell (automatisch schließen, Warnung bei Laufzeit) sind ebenso SLAs mit Stufen; eine
  SLA zählt nach Kalender oder rund um die Uhr (E117).
- **Wiederkehrende Vorgänge** (gebaut, E119): Datensätze von `core.recurrence` mit Zeitplan
  (cron mit Zeitzone) und Vorlage; ein Job jede Minute legt je fälligem Termin genau einen
  Datensatz an (`internal/recurrence`), ein verpasster Termin wird einmal nachgeholt.
- **Workflow-Provider:** eine Schnittstelle mit `Start`, `Signal` und `Cancel`.
  - Provider 1 ist die eingebaute Engine.
  - Provider 2 ist Flowzilla, erreichbar über den Aktivitätstyp „externer Prozess". Der ruft
    Flowzilla per REST auf und wartet auf den Rückruf an `…/transitions/{übergang}`.
  - Weitere Provider sind gleich gebaut.

---

## 9. Sandbox

E17, E45:

- **CEL zuerst:** Bedingungen, Formeln, Identifikationsregeln und Rechtebedingungen sind
  CEL-Ausdrücke. Sie werden beim Speichern der Definition kompiliert und typgeprüft und laufen im
  Mikrosekundenbereich. „Jetzt“ heißt `today()` (Datum in Europe/Berlin) und `now()`
  (Zeitpunkt), mit Abstand `today(14)` oder `now("-24h")`; in SQL setzt der Übersetzer den Wert
  als Parameter ein (seit SD16d).
- **WASM für Logik,** die CEL nicht kann, etwa eine Berechnung über mehrere Datensätze oder eine
  Formatumwandlung. Module gehören zu einem Paket und werden beim Laden einmal kompiliert. Jeder
  Aufruf hat eine Speicher- und eine Zeitgrenze.
- **Host-Funktionen:** Datensätze lesen, mit den Rechten des Auslösers; Datensatz ändern; Ereignis
  auslösen; protokollieren. HTTP nach außen gibt es nur, wenn das Paket es deklariert und der
  Admin es freigibt.
- **SDK für Module:** `pkg/sdk` für Go-Module (TinyGo), weitere Sprachen später.

---

## 10. Oberfläche

E5, E41, E42:

```
web/src/
  app/          Rahmen, Navigation, Anmeldung, Einstellungen
  api/          aus OpenAPI erzeugter Client (keine Hand-Typen)
  render/       Listen, Formulare, Detailseiten, Zeitleiste aus der wirksamen Definition;
                Editor mit Textbausteinen (SD8)
  views/        Kalender, Gantt, Kanban, Graph, Matrix, Karte (KF10)
  editors/      Typ-, Formular-, Workflow- (React Flow 12 + dagre), Rollen- und Dashboard-Editor
  portal/       Oberfläche für Nutzer (eigene Vorgänge seit UI10; Band „Aktuell“ aus
                core.portal_notice seit SD5; Lesebereiche wie „Wissen“ im Kopf aus der
                Portal-Option read eines Typs seit SD16a; Katalog mit SD)
  dashboards/   Widgets (ECharts)
  i18n/         Deutsch und Englisch von Anfang an; Inhalte werden in der Sprache der Person
                gelesen, das Formular liest und schreibt Deutsch (SD16c)
  ui/           shadcn/ui-Bausteine nach dem Design-Kit „Klartext“ (Tokens hell und dunkel, IBM Plex, Marken für Status, Priorität, Frist), Theme, Barrierefreiheit
```

- **Fast alles wird gerendert.** Fachseiten entstehen aus der Definition. Eigener Code je Typ ist
  die Ausnahme und kommt dann als Paket-Ansicht.
- **Detailseite** (UI3, Hauptplan E122): Kopf mit Status und Übergängen, Felder mit der
  Zeitleiste darunter, Seitenleiste mit Rollen, SLA-Uhren und verweisenden Datensätzen samt
  Summen; alles aus der Definition (Merkmale, Rollen, Summenfelder).
- **Lange Listen** laufen mit TanStack Table und Virtual. Die Daten kommen seitenweise per
  Schlüssel, und Spalten werden nur gerendert, wenn sie sichtbar sind.
- **Live:** SSE aktualisiert offene Listen und Detailseiten. Arbeitet jemand anderes am selben
  Datensatz, zeigt die Seite das an (E29); die Anwesenheit liegt in Valkey (`internal/presence`,
  E130).
- **Barrierefreiheit:** BITV 2.0 / WCAG 2.1 AA. `axe` prüft in Vitest und Playwright, und ein
  Fehler bricht den Build.
- **Auslieferung:** statische Dateien aus dem `api`-Container. Es gibt keinen eigenen Webserver
  für das Frontend.

---

## 11. Leistung

Wie die Ziele aus Hauptplan §2.5 erreicht werden sollen; gemessen ab H11 bei jeder Änderung
(E32), in zwei Stufen (E63): schnell in jedem CI-Lauf im Wechsel gegen das freigegebene
Basis-Binary (E69), voll in Prod-Größe vor dem Fertigmelden datennaher Aufgaben und vor jedem
Meilenstein. Werkzeug: `test/load` (E51).

| Ziel | Maßnahme |
| --- | --- |
| LZ1 lesen ≤ 50 ms | Primärschlüssel auf UUIDv7; Definition und Rechte aus dem Cache im Prozess; ein SQL-Aufruf je Anfrage |
| LZ2 Liste ≤ 150 ms | echte Spalten und Indexe je Typ (E33); Rechte als SQL-Bedingung mit GiST auf `org_path`; Seiten per Schlüssel; nur angeforderte Spalten |
| LZ3 Volltext ≤ 300 ms | erzeugte `tsvector`-Spalte je Typ mit GIN; Trigram nur auf wenigen Namensfeldern |
| LZ4 schreiben ≤ 100 ms | eine Transaktion; Folgearbeit als Job über die Outbox |
| LZ5 Bulk ≥ 5.000/s | `COPY` und `INSERT … ON CONFLICT` in Stapeln; Prüfsumme statt Feldvergleich; Audit nur für Änderungen |
| LZ6, LZ7, LZ8 Massenläufe | wie LZ5; Unterobjekte als Vollstand je Elternteil; mehrere Worker parallel nach Quelle und Bereich |
| LZ9, LZ10 Oberfläche auch im Massenlauf | Bulk nur im Worker mit eigenem, begrenztem Pool; River-Warteschlangen mit Vorrang; `statement_timeout` für interaktive Anfragen |
| LZ11 2.000 Anfragen/s, LZ12 horizontal | `api` zustandslos; Sitzungen, Raten und Pub/Sub in Valkey; Verbindungen über einen Pooler (V22) |

- **Sichtbar machen:** OpenTelemetry-Traces für jede Anfrage, Prometheus-Metriken (Dauer je
  Endpunkt, Pool, Jobs, Outbox-Rückstand), `pprof` hinter einem Admin-Recht.
- **Wovon die Zahlen abhängen:** von der Hardware der Messung (V19) und von der Ticketmenge (V14).
  Beides ist offen.

---

## 12. Repository

```
cmd/itsm/            ein Binary (E35)
internal/<baustein>/ Bausteine aus §2
pkg/sdk/             SDK für WASM-Module
packages/            mitgelieferte Pakete (core, servicedesk, incident, …)
db/migrations/       goose-Migrationen der Kerntabellen
db/queries/          sqlc-Abfragen der Kerntabellen
web/                 Oberfläche (§10)
deploy/compose/      Docker Compose
deploy/helm/         Helm-Chart
test/load/           Lastwerkzeug und Datensatz in Prod-Größe (H11)
tools/               Werkzeug-Container (E52)
scripts/             CI und Prüfskripte
docs/                Pläne, diese Architektur, adr/
```

---

## 13. Offen

Die Fragen zu diesem Entwurf stehen im Hauptplan §6: V19 (Hardware der Messung), V20
(Lastwerkzeug), V21 (Grenzen der Sandbox), V22 (Pooler).
