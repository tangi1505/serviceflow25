# Plan: Plattform-Kern (PL)

**Stand:** 2026-10-20, erste Fassung aus Hauptplan H8. Teil des [Hauptplans](0_maintask.md),
Etappe 2 (Meilenstein 1) bis 5. Grundlage: [Architektur](architektur.md),
[Funktionsinventar](funktionsinventar-glpi.md), Kernfähigkeiten im Hauptplan §2.1.1,
Leistungsziele im Hauptplan §2.5. Nichts davon ist gebaut.

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

**Für jede Aufgabe gilt:** Sie ist erst fertig, wenn `scripts/ci.sh` grün ist und die
Leistungsmessung aus Hauptplan H11 keinen Zielwert verfehlt und nichts um mehr als 10 %
langsamer macht (E32). Der Prüfweg nennt nur, was dazukommt. Alle Befehle laufen in WSL in
`~/police/itsm`. „Inventar G…" verweist auf die Zeilen des Funktionsinventars, „trägt KF…" auf
die Kernfähigkeit, für die die Aufgabe die Verantwortung hat.

---

## Unterpläne

**Stand eines Plans:** ✅ abgeschlossen · 🔨 läuft · ⬜ nicht begonnen · 🔒 wartet · 💤 ruht

| Nr | Kürzel | Unterplan | Ergebnis | Etappe | Aufwand | Stand |
| --- | --- | --- | --- | --- | --- | --- |
| 011 | **AP** | [API und Agenten](01_plattform-kern/011_api-und-agenten.md) | die API vollständig beschrieben und getestet, gegen eine Flut geschützt (Limits in vier Schichten) und von einem Agenten ohne Wissen aus dem Code bedienbar; der MCP-Server selbst bleibt im Nebenplan 09 | 2 bis 4 | 27,5 d | 🔨 angelegt am 2026-11-16 auf deine Entscheidung, 25 Aufgaben; AP12 (Anforderungen an Agenten) ist 🟢, die Limits (AP6 bis AP9) sind freigegeben, neu sind Block E (OAuth im Namen eines Menschen, kein Aufruf ohne Konto) und das Leseprotokoll für Agenten und Dienstkonten (AP26, asynchron nach Hauptplan E191) |

---

## Aufgaben

### A — Durchstich (Meilenstein 1, zuerst)

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **PL1** | Meta-Modell, Grundform: Kerntabellen für Typ- und Felddefinitionen; eine Definition aus YAML laden, validieren und speichern; Feldtypen Text, Zahl, Ja/Nein, Datum, Zeitpunkt, Auswahl, Verweis (Architektur §2, §4; noch ohne Schichten) (trägt KF1; Inventar G16, G17) | ✅ | 2 d | `docker compose run --rm tools go run ./cmd/itsm package install packages/probe` meldet den Typ `probe` mit seinen Feldern; eine Definition mit unbekanntem Feldtyp wird mit Feldname und Grund abgelehnt; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL2** | Schema: je Typ eine Tabelle mit den Systemspalten aus Architektur §3.1 anlegen; ein neues Feld online als Spalte ergänzen; Eindeutigkeit je Feld als Index; jede Schemaänderung protokolliert und unter Advisory Lock (E33) (Inventar G182) | ✅ | 2 d | Nach PL1 zeigt `\d rec_probe` in `psql` die Systemspalten und die Felder; ein Feld mit `unique: true` hat einen eindeutigen Index; zwei parallele Installationen desselben Pakets enden ohne Fehler und mit genau einer Tabelle (Test); `docker compose run --rm tools scripts/ci.sh` grün |
| **PL3** | Datensätze: lesen, anlegen, ändern, in den Papierkorb legen und endgültig löschen über den eigenen SQL-Builder (E43); Validierung; `version` mit Konflikterkennung (E29); Verlauf je Feld in derselben Transaktion (Inventar G60, G235, G236) | ✅ | 2 d | Tests in `internal/records`: zwei Änderungen auf derselben Version → die zweite scheitert mit Konflikt; jede Änderung erzeugt je geändertem Feld genau eine Verlaufszeile; ein gelöschter Datensatz fehlt in Listen und ist wiederherstellbar; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL4** | REST-API je Typ: `/api/v1/types`, `/api/v1/records/{typ}` mit Lesen, Anlegen, Ändern, Löschen; OpenAPI zur Laufzeit aus dem Meta-Modell, die feste Beschreibung als YAML mit `oapi-codegen` (E60); Fehlerformat; Seiten per Schlüssel; einfache Filterparameter (Inventar G206, G207, G211) | ✅ | 2 d | `curl -s localhost:8092/api/v1/openapi.json` enthält den Pfad `/api/v1/records/probe.probe` (Typnamen sind qualifiziert, E67); anlegen, lesen, ändern mit `If-Match`, löschen per `curl` liefert 201, 200, 200, 204; ein Ändern ohne `If-Match` liefert 428; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL5** | Anmeldung mit lokalen Konten: Kennwort als Argon2id, Sitzung in Valkey mit Cookie, Abmelden, Kennwortablauf, Sitzungen räumen sich selbst ab (TTL) (Inventar G117, G241) | ✅ | 1,5 d | Anmelden per `curl` mit Konto aus `itsm user create` → Cookie; mit Cookie liefert `/api/v1/records/probe` 200, ohne 401; nach Abmelden 401; der Schlüssel der Sitzung in Valkey hat eine TTL; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL29** | Lizenzprüfung der Abhängigkeiten: Go-Module und pnpm-Pakete gegen eine Liste erlaubter Lizenzen (AGPL-verträglich, E15; Hauptplan E104) als Schritt in `scripts/ci.sh` | ✅ | 0,5 d | `docker compose run --rm tools scripts/ci.sh` grün mit dem neuen Schritt „licenses"; eine Abhängigkeit mit nicht erlaubter Lizenz in einem Probe-Zweig lässt ihn fehlschlagen |

### B — Verbreitern bis Meilenstein 1

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **PL6** | Organisationsbaum: Kerntabelle mit `ltree`, `org_path` an jeder Typtabelle mit GiST-Index, Einstellungen je Knoten mit Vererbung (E16) (Inventar G119) | ✅ | 1,5 d | Test: Einstellung an der Wurzel gesetzt, am Kind überschrieben → Enkel erbt vom Kind; `EXPLAIN` einer Unterbaum-Abfrage zeigt den GiST-Index; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL7** | Personen und Gruppen: Typen des Kernpakets (E79), mehrere Mailadressen (E80), Titel, Telefon, Vorgesetzter; Gruppen als Baum mit Verantwortlichen und Zweck (Inventar G116, G118) | ✅ | 1,5 d | Personen und Gruppen sind über `/api/v1/records/person` und `/group` anleg- und abfragbar; eine Gruppe mit Untergruppen liefert ihre Mitglieder auf Wunsch rekursiv; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL8** | Rollen und Rechte, Teil 1: Berechtigung aus Typ, Aktion und Reichweite (eigene, Gruppe, Organisationseinheit, Unterbaum, alle); Zuweisung an Person oder Gruppe je Knoten; Übersetzung in eine SQL-Bedingung (E34); Prüfung gegen SQL auf Typtabellen außerhalb von `query` und `records` (trägt KF9; Inventar G124) | ✅ | 2 d | Tests mit drei Personen und je anderer Reichweite: jede sieht genau ihre Datensätze, in Liste, Einzelabruf und Zählung; eine Abfrage auf `rec_` außerhalb der zwei Pakete bricht `golangci-lint`; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL9** | Rollen und Rechte, Teil 2: Bedingung in CEL, Rechte je Feld (Spaltenauswahl), Cache der übersetzten Bedingung im Prozess und in Valkey, Invalidierung über Pub/Sub | ✅ | 2 d | Test: Feld ohne Leserecht fehlt in API-Antwort und OpenAPI; nach Entzug einer Rolle gilt das neue Recht auf allen `api`-Instanzen binnen einer Sekunde (Test mit zwei Instanzen); `docker compose run --rm tools scripts/ci.sh` grün |
| **PL10** | Aktive Rolle und Organisationseinheit in der Sitzung wechseln (Inventar G240) | ✅ | 0,5 d | Nach dem Wechsel per API zeigt eine Liste nur noch die Datensätze der gewählten Einheit; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL11** | Anmeldung über OIDC (E61) mit `go-oidc`; Keycloak als Compose-Profil `auth` zum Testen; Regeln, die Rollen aus Gruppen des Identitätsanbieters zuweisen (Inventar G121) | ✅ | 2 d | `docker compose --profile auth up -d`, im Browser http://localhost:8092 → Weiterleitung zu Keycloak, Anmeldung mit dem Testnutzer, zurück im Werkzeug mit der Rolle aus der Keycloak-Gruppe; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL12** | LDAP-Sync (E61): mehrere Verzeichnisse, Personen und Gruppen importieren und abgleichen als Job; `lldap` als Compose-Profil `auth` (Inventar G184) | ✅ | 2 d | Nach `itsm ldap sync` stehen die Testpersonen und Gruppen aus `lldap` im Werkzeug; eine im Verzeichnis gelöschte Person ist danach deaktiviert, nicht gelöscht; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL13** | API-Zugänge: OAuth 2 Client Credentials für technische Nutzer, IP-Bereich je Zugang, Ratenbegrenzung in Valkey, Einstellungen der API (Inventar G179, G187, G226) | ✅ | 2 d | Token per `curl` mit Client-ID und Geheimnis; Aufruf aus nicht erlaubtem IP-Bereich → 403; über der Rate → 429 mit `Retry-After`; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL14** | JSON-Abfrage (E40): `and`, `or`, Vergleiche, über Beziehungen, Sortierung, Feldauswahl, Aggregate; immer mit Rechtebedingung und ohne Papierkorb (Inventar G228) | ✅ | 2 d | Die Beispielabfrage aus Architektur §6.3 liefert in einem Test genau die erwarteten Datensätze; dieselbe Abfrage als Person mit kleinerer Reichweite liefert weniger; LZ2 hält in der Messung aus H11; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL15** | Beziehungen: Kerntabelle `relation`, Beziehungsarten in der Definition, Graph-Abfrage mit Tiefengrenze, Auswirkungsanalyse über die API (trägt KF2; Inventar G48) | ✅ | 2 d | Test: Kette aus vier Datensätzen, Abfrage mit Tiefe 2 liefert genau zwei Ebenen; ein Kreis endet ohne Endlosschleife; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL16** | Auswahllisten als Typ mit Merkmal (B3), auch als Baum und mit Koordinaten; jedes Paket bringt seine Listen mit (Inventar G19, G138, G139, G140, G141, G142, G143, G144, G145, G146, G147, G148, G149, G150, G151, G152, G153, G154, G155, G156, G157, G158) | ✅ | 1,5 d | Ein Probe-Paket mit zwei Listen, eine davon ein Baum, legt keine eigene Tabelle je Liste an; ein Feld vom Typ Auswahl lehnt Werte außerhalb der Liste ab; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL17** | Merkmale (B2): Definitionsbausteine wie Verlauf, Dokumente, Notizen, reservierbar, per Name an einen Typ binden (Inventar G18) | ✅ | 1,5 d | Ein Typ mit `traits: [history, documents]` hat Verlauf und Dateien, ein Typ ohne nicht; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL18** | Dateien: Schnittstelle mit Umsetzung Dateisystem (E22), Hoch- und Herunterladen per API, Verknüpfung an jedem Typ, Metadaten in PostgreSQL; temporäre Dateien räumt ein Job ab (Inventar G90, G245) | ✅ | 2 d | Datei per `curl` hochladen, an einen Datensatz hängen, herunterladen: gleicher SHA-256; abgebrochene Uploads sind nach dem Aufräumjob weg; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL19** | Versionen von Datensätzen und Vergleich zweier Stände (trägt KF17; Inventar G107) | ✅ | 1,5 d | Test: drei Änderungen, Vergleich von Version 1 und 3 zeigt genau die geänderten Felder; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL20** | Kosten und Zeiten als Untersätze an jedem Typ, mit Summen am Datensatz (trägt KF18; Inventar G46, G80) | ✅ | 1,5 d | Drei Zeiteinträge an einem Datensatz → Summe am Datensatz stimmt, auch nach Löschen eines Eintrags; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL21** | Mehrsprachige Inhalte: Beschriftungen der Definitionen und übersetzbare Feldwerte (trägt KF19; Inventar G237) | ✅ | 1,5 d | Ein Datensatz mit deutschem und englischem Titel liefert je nach `Accept-Language` den passenden; fehlt eine Sprache, kommt Deutsch; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL22** | Berechnete Felder in CEL (E45), beim Schreiben ausgewertet und gespeichert, typgeprüft beim Speichern der Definition (trägt KF6) | ✅ | 2 d | Ein Feld `priority` aus `impact` und `urgency` wird beim Anlegen und Ändern gesetzt; ein CEL-Ausdruck mit Typfehler wird beim Installieren abgelehnt; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL23** | Jobs: River einbinden (E19), Warteschlangen mit Vorrang, Zeitpläne, Verwaltung und Protokoll per API; Benachrichtigungen und Webhooks laufen als Jobs (Inventar G126, G166, G183) | ✅ | 2 d | `/api/v1/jobs` zeigt Zeitpläne und letzte Läufe; ein Job in der Warteschlange `bulk` verzögert keinen in `interactive` (Test); `docker compose run --rm tools scripts/ci.sh` grün |
| **PL24** | Ereignisse (E39): Outbox in derselben Transaktion, fortlaufende Position, Verteiler im Worker, abrufbarer Strom `/api/v1/events?after=` für 7 Tage (E46), SSE über Valkey Pub/Sub (trägt KF13) | ✅ | 2 d | Zehn Änderungen → zehn Ereignisse in Reihenfolge; ein Abonnent, der bei Position 5 aussetzt, holt 6 bis 10 nach; eine zu alte Position liefert 410; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL25** | Webhooks: Abonnements je Ereignis, signierte Nutzlast, Wiederholung mit Rückstellung (Inventar G165) | ✅ | 1,5 d | Ein Testempfänger, der zweimal 500 liefert, bekommt das Ereignis beim dritten Versuch; die Signatur lässt sich mit dem Geheimnis prüfen; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL26** | Volltextsuche: `tsvector` je Typ aus den durchsuchbaren Feldern, deutsches Wörterbuch, Trigram für Namen, Endpunkt `/api/v1/search` (E21) | ✅ | 1,5 d | Suche nach „Drucker" findet „Druckern"; Suche nach „Drukcer" findet über Trigram den Namen; LZ3 hält in der Messung aus H11; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL27** | Ereignisprotokoll: Anmeldungen, Fehlschläge, Änderungen an Rechten abfragbar (Inventar G128) | ✅ | 1 d | Drei fehlgeschlagene Anmeldungen erscheinen mit Zeit und Quelle in `/api/v1/audit/auth`; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL28** | Sicherheitseinstellungen: Kennwortregeln, Sitzungsdauer, Sperre nach Fehlversuchen, lokale Anmeldung; Zwei-Faktor über Keycloak statt in itsm (Inventar G177; Hauptplan E103) | ✅ | 1,5 d | Ein Keycloak-Konto kommt ohne gültigen OTP-Code nicht durch (von Hand im Browser); ein zu kurzes Kennwort wird abgelehnt; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL30** | API-Abdeckung der Pakete von Meilenstein 1 prüfen: Personen, Gruppen, Organisationseinheiten, Auswahllisten, Vorgänge, Wissen, Benachrichtigungen, Einstellungen (Inventar G208, G209, G213, G214, G216, G219, G223; Hauptplan E105: allgemeiner Test über alle Pakete, die SD-Pakete prüft er, sobald es sie gibt) | ✅ | 1 d | Ein Test ruft für jeden Typ der Meilenstein-1-Pakete Liste und Einzelabruf auf und vergleicht mit der OpenAPI; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL41** | Verteiler in Stapeln: jeder Abnehmer (live, Webhooks, Regeln) mit eigener Position und Zustand, ein Durchgang je Stapel in einer Transaktion, ein Fehler hält den Abnehmer an statt zu überspringen, die Aufbewahrung wartet auf den langsamsten, endgültige Fehlschläge abfragbar (Hauptplan E112, LZ13) | 🟢 | 1,5 d | Nach einem Bulk-Lauf zeigt `/api/v1/events/consumers` den Rückstand sinken bis 0; LZ13 ≥ 5.000 Ereignisse/s; eine Regel, die zur Laufzeit abgelehnt wird, steht in `/api/v1/events/failures`; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL42** | Zeitleiste am Datensatz: Merkmal `timeline`, Einträge (Folgemeldungen) öffentlich oder privat in der Kerntabelle `entry`, unveränderlich; schreiben mit dem Recht `comment`, private lesen und schreiben zusätzlich mit `read_private`; `GET …/{id}/timeline` liefert Einträge und Verlauf (Feldänderungen, Übergänge, Verstoß, Eskalation, Benachrichtigung) in zeitlicher Reihenfolge; Ereignis `comment` für Benachrichtigungen, private nur an Berechtigte (Hauptplan E121, Inventar G40) | 🟢 | 1,5 d | Ein privater Eintrag erscheint in `…/timeline` nur für ein Konto mit `read_private`; ein öffentlicher Eintrag eines Anforderers ohne `update` gelingt mit `comment`; eine Benachrichtigung auf `comment` schickt einen privaten Eintrag nur an Berechtigte; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL43** | Kundenschicht und wirksame Definition: Änderungen aus der Oberfläche als Liste in der Schicht `customer`, die wirksame Definition aus Basis und Kunde (gecacht, Hash als Version), Konflikte beim Aktualisieren eines Pakets angezeigt, Export der Kundenschicht als Paket; die Erweiterungsschicht bleibt bei EX (Hauptplan E135, E37; Architektur §4) | 🟢 | 2 d | Ein Feld per API in der Kundenschicht ergänzen → erscheint im Typ und in der Tabelle; das Basispaket neu installieren → das Feld bleibt; ein Paket, das ein von der Kundenschicht geändertes Feld entfernt, meldet den Konflikt vor der Aktivierung; der Export enthält genau die Änderung; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL44** | Mandantengrenze: Knoten mit `tenant` direkt unter der Wurzel, Konto und Person mit Mandant, die Sitzung trägt einen Mandanten, der Rechtefilter setzt die Grenze vor jede Reichweite (auch `all`), Admins der Installation wechseln den Mandanten wie den Rechtekontext (Hauptplan E136; architektur.md §5.1; V24, V25, V27) | 🟢 | 2 d | Zwei Mandanten mit je einem Konto und der Rolle `all`: keiner sieht in Liste, Suche, Einzelabruf und Export einen Datensatz des anderen (404); ein Admin sieht nach dem Wechsel nur den gewählten; ein Test ruft jede Route mit beiden auf; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL45** | Nebendaten je Mandant: Ereignisstrom und SSE, Webhooks, Benachrichtigungen, Dateien, Jobs, Exporte, Ereignisprotokoll, Einstellungen, API-Clients, Verzeichnisabgleich und Valkey-Schlüssel tragen den Mandanten; Vorgaben der Pakete als Kopie für einen neuen Mandanten (Hauptplan E136; V28) | 🟢 | 2 d | Ein Ereignis in Mandant A erreicht weder SSE noch Webhook noch Benachrichtigung in B; ein API-Client von A bekommt in B nichts; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL46** | Anlegen und Eindeutigkeit je Mandant: ein neuer Datensatz landet im Knoten des Rechtekontexts oder im Heimatknoten der Person, die Oberfläche wählt den Knoten; eindeutige Felder und externer Schlüssel je Mandant (Hauptplan E136, E140) | 🟢 | 1,5 d | Dieselbe Seriennummer in A und B geht, zweimal in A nicht; eine Person mit Rolle nur im Unterbaum legt ohne Angabe in ihrem Knoten an; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL47a** | Datenbank je Mandant, Naht: `db.Router` mit `Control()`, `Tenant(ctx)`, `For(tenant)`; kein Dienst hält mehr einen Pool, jeder Zugriff wählt Steuer- oder Mandanten-Datenbank; bei einer Datenbank ändert sich nichts (Hauptplan E137, E141) | 🟢 | 1,5 d | Ein Test findet keinen Pool und keine Abfrage-Verbindung als Feld eines Dienstes; der Router wählt mit zwei Mandanten-Datenbanken richtig; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL47b** | Datenbank je Mandant, Steuer-Datenbank: Benutzername → Mandant und Anmeldung über sie; Konten, Regeln, API-Clients und Protokoll der Installation zentral; Rollen und Rechte zentral, Zuweisungen beim Mandanten, die der Installationskonten zentral; Anmelderegeln und Identitäten zentral; Spiegelkonto mit Person für Admins bei jedem Betreten; Baum geteilt (Wurzel und Mandantenknoten zentral, Teilbaum beim Mandanten) (Hauptplan E141, E142) | 🟢 | 2 d | Mit zwei Mandanten-Datenbanken: Anmeldung von Konten beider Mandanten und eines Admins; ein Admin ist Bearbeiter an einem Vorgang in A; eine Regel der Installation wirkt in A und B; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL47c** | Datenbank je Mandant, Hintergrund: je Mandanten-Datenbank ein River-Client, Verteiler und Zeitplaner (SLA, Wiederholung, Benachrichtigung), dazu einer für die Steuer-Datenbank (Hauptplan E141, E143) | 🟢 | 1 d | Ein Ereignis in A erreicht Webhook und Benachrichtigung von A über den Verteiler von A; ein Job von B läuft in B; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL47d** | Datenbank je Mandant, Betrieb: `ITSM_TENANT_MODE=shared\|database`, je Mandant `ITSM_TENANT_<NAME>_DATABASE_URL` (Zugangsdaten nur in der Konfiguration); `migrate`, `package install` und `tenant create` wirken auf jede Datenbank; Sicherung und Wiederherstellung einer Mandanten-Datenbank (Hauptplan E141, E144) | 🟢 | 1 d | Ein Paket-Update erreicht beide Mandanten-Datenbanken; eine Mandanten-Datenbank lässt sich allein sichern und wiederherstellen; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL47e** | Datenbank je Mandant, Tests: der Grenz-Rundgang aus PL44 bis PL46 läuft in beiden Betriebsarten mit eigenen Datenbanken (Hauptplan E141, E145; architektur.md §5.1) | 🟢 | 1 d | Der Rundgang findet in beiden Betriebsarten keinen Treffer über die Grenze; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL48** | Auswertungen über Mandanten: Sonderrecht für Admins der Installation, JSON-Abfrage mit Gruppierung und Aggregation sowie Listen je Mandant mit dessen Rechten, Zusammenführen mit dem Mandanten als Spalte, Eintrag im Ereignisprotokoll jedes Mandanten; nur lesen (Hauptplan E137, E146) | 🟢 | 1,5 d | Ein Admin mit dem Recht sieht offene Vorgänge je Status für A und B in einer Tabelle, in beiden Betriebsarten; ohne das Recht 403; beide Mandanten haben den Eintrag im Ereignisprotokoll; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL49** | Laufende Nummer je Typ: Feldtyp `sequence` mit Präfix (etwa `INC-24817`), vom Kern beim Anlegen vergeben, fortlaufend je Typ und Mandant in beiden Betriebsarten, danach nicht änderbar, in Liste, Suche und Betreff von Mails nutzbar; Lücken nach abgebrochenen Transaktionen sind erlaubt (aus dem [SD-Plan](05_service-desk.md) §2.3, N1; Hauptplan E137, E141; E63) | 🟢 | 1 d | Typ mit Feld `sequence` und Präfix `INC-` → neue Datensätze `INC-1`, `INC-2`; 100 Anlagen gleichzeitig ergeben 100 verschiedene Nummern; Mandant B beginnt wieder bei `INC-1`; PATCH auf das Feld → 422; `/api/v1/search?q=INC-2` findet den Datensatz, ebenso die Befehlszeile (Strg K, `INC-2` tippen, Enter öffnet ihn; UI-Plan E36); `docker compose run --rm tools scripts/ci.sh` grün |
| ~~**PL50**~~ | ~~Zwei CIs gleichzeitig (Spuren K und O, Hauptplan E147): die festen Namen der Test-Datenbanken aus `dbtest.Isolated` bekommen eine Endung aus einer Umgebungsvariable, ebenso die CI-Datenbank `itsm_ci`; Valkey getrennt je Spur (eigene Valkey-Datenbank oder Präfix der Schlüssel, Entscheidung vor dem Bau); Compose-Projekt und Ports der zweiten WSL-Kopie stören die erste nicht. Bis dahin läuft nie mehr als eine CI~~ — **entfällt am 2026-10-31:** nur noch ein Agent, nie zwei CIs gleichzeitig (Hauptplan E149) | — | 0,5 d | Zwei `scripts/ci.sh` mit verschiedener Endung laufen gleichzeitig und sind beide grün; keine löscht Datenbanken oder Schlüssel der anderen |
| **PL51** | CI mit eigenem Stand je Lauf: eigener PostgreSQL und Valkey je Lauf, frisch und danach entfernt; zeitkritische Pakete nacheinander; kein Abbruch beim ersten Fehler, alle Fehler gesammelt; Browser-Tests einzeln (Hauptplan E150) | 🟢 | 0,5 d | Zwei Läufe nacheinander finden dieselben Datenbanken vor (nur `itsm_ci` samt Vorlagen); nach dem Lauf gibt es keine Container `postgres-ci` und `valkey-ci` mehr; ein absichtlich roter Schritt hält die übrigen nicht auf und steht in der Zusammenfassung; `scripts/ci-run.sh` grün |
| **PL52** | Schnellere CI in zwei Stufen (Hauptplan E162): ein Skript für die Teil-CI während der Arbeit (Lint, Format, Typen immer; die geänderten Go-Pakete samt allen, die sie nutzen; Vitest nur für betroffene Dateien; die Browser-Tests des Bereichs); die ganze CI bleibt vor jedem Commit; die Lastmessung läuft nur, wenn sich Go-Code des Servers (`cmd/`, `internal/`) oder Migrationen ändern; die Basislinie der schnellen Stufe wieder messbar machen (heute „login as load: 404“, verglichen werden nur die festen Grenzen); `internal/httpapi` (210 s) in parallele Teile mit eigener Datenbank aufteilen; die wackeligen Browser-Tests (Formular-Baukasten per Tastatur, Zeitleiste in `records.spec`, LZ9-Seitenwechsel nach einem Neustart; in Vitest die Zeitgrenze von Formular-Baukasten und Workflow-Editor unter Last) stabil machen | 🟢 | 1 d | Eine Änderung nur in `web/src` läuft in der Teil-CI ohne Go-Tests und ohne Lastmessung durch und nennt die Tests, die sie gewählt hat; die ganze CI misst LZ1 bis LZ4 wieder gegen die Basislinie; die ganze CI ohne Lastmessung ist mindestens zwei Minuten schneller als 10:27 min (deine Entscheidung 2026-11-03); drei Läufe nacheinander ohne wackelnden Test; `scripts/ci-run.sh` grün |
| **PL53** | Typen und Kalender im Speicher (gefunden mit H14, E64): die Typdefinitionen werden nicht mehr bei jedem Aufruf aus der Datenbank gelesen und dekodiert, sondern im Speicher gehalten und bei einer Änderung neu geladen (sofort im eigenen Prozess, über `LISTEN itsm_definitions` für andere Instanzen und die Kommandozeile, spätestens nach einer Minute); die SLA-Uhren halten ihren Kalender zehn Sekunden | 🟢 | 1 d | Massenimport von Störungen mindestens dreimal so schnell; ein Feld aus dem Typ-Editor ist sofort da, eine Änderung an der API vorbei binnen Sekunden; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL54** | Indizes je Typ (gefunden mit H14, E65): der Kern legt für jeden Typ einen Index auf dem Anlagedatum an; ein Paket erklärt in der Typdatei weitere unter `indexes:` (bis vier Spalten aus Feldern und Systemspalten wie `state`), der Kern legt sie beim Installieren an und entfernt erklärte, die das Paket nicht mehr nennt; Störung und Anfrage erklären Status mit Anlagedatum | 🟢 | 1 d | die Warteschlange „Störungen meines Teams“ auf 18,25 Mio. Störungen liest nicht mehr die ganze Tabelle; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL55** | Suche über große Mengen (gefunden mit H14, deine Entscheidung 2026-11-14): die Volltextsuche bewertet je Typ die neuesten bis zu 1.000 Treffer der letzten sieben Tage und sucht nur, wenn das zu wenige sind, über alles | 🟢 | 1 d | LZ3 auf 18,25 Mio. Störungen p95 ≤ 300 ms; CI grün |
| **PL56** | Zeitgrenze je API-Anfrage (gefunden mit H14, deine Entscheidung 2026-11-14): PostgreSQL bricht jede Abfrage einer API-Anfrage nach 10 s ab, einstellbar; Export und Bulk-Jobs ausgenommen | 🟢 | 0,5 d | eine zu lange Abfrage blockiert nicht mehr den Server; CI grün |
| **PL57** | Rechtefilter zusammenfassen (gefunden mit H14, deine Entscheidung 2026-11-15): gleiche Rechte werden eine Bedingung, jede Gruppe zählt einmal; dieselbe Rolle am selben Knoten und dieselbe Mitgliedschaft lassen sich nicht doppelt anlegen | 🟢 | 1 d | LZ2 auf 18,25 Mio. Störungen p95 ≤ 150 ms; CI grün |
| **PL58** | SLA-Speicher im Hintergrund (gefunden mit H14, deine Entscheidung 2026-11-15): Anfragen nehmen den letzten Stand der SLAs, nachgeladen wird beim Start und im Hintergrund, nie in einer Anfrage; andere Zwischenspeicher mit demselben Muster ebenso | 🟢 | 0,5 d | LZ4 mit 32 gleichzeitigen Anfragen ohne Fehler; CI grün |
| **PL59** | Übernahme alter Daten ohne Folgejobs (Befund aus H14): Der Bulk-Import alter, längst erledigter Störungen legt je Zeile Fristen, Benachrichtigungen und Regeln als Jobs an; 18,25 Mio. Störungen ergaben rund 70 Mio. Jobs, `river_job` hielt 35 GB für 7 Tage (E86). Vorschlag: ein Kennzeichen am Bulk-Lauf für eine Übernahme, die nur speichert | ⬜ deine Entscheidung offen | 1 d | 100.000 erledigte Störungen als Übernahme → keine Jobs außer dem Bulk-Job; ohne Kennzeichen wie bisher |
| **PL60** | Bulk mit Fristen schneller (Befund aus H14): Störungen mit SLA-Uhren laden mit rund 920 Zeilen/s, LZ5 (ein Typ ohne Fristen) mit 8.850. Vorschlag: Uhren und Kalender je Stapel statt je Zeile rechnen | ⬜ deine Entscheidung offen | 1 d | Lastwerkzeug: Bulk von Störungen mit Fristen ≥ 5.000 Zeilen/s |
| **PL61** | Große Bulk-Ströme ohne Abbruch (Befund aus H14): Rivers Grenze von einer Minute je Job bricht einen Strom von 2.000 Störungen mit Fristen ab; auf `ki-lab` umgangen mit 500 Zeilen je Strom. Vorschlag: eigene Grenze für Bulk-Jobs, einstellbar | ⬜ deine Entscheidung offen | 0,5 d | ein Strom von 50.000 Störungen läuft durch |
| **PL62** | Rechte verschiedener Art zugleich (Befund aus H14, Rest aus PL57): „beteiligt“ ODER „eigene Einheit“ bleibt eine ODER-Bedingung und kostet bei Teams mit Hunderttausenden Vorgängen rund 0,5 s | ⬜ deine Entscheidung offen | 1 d | Lastwerkzeug: LZ2 mit einer Rolle aus beiden Arten p95 ≤ 150 ms auf `ki-lab` |
| **PL63** | Index ohne Schreibsperre (Befund aus H14, Rest aus PL54): Ein neuer Index auf einer großen Tabelle hält das Schreiben für die Dauer des Baus an (`CREATE INDEX` statt `CONCURRENTLY`); Kundenschicht und Typ-Editor kennen `indexes` noch nicht | ⬜ deine Entscheidung offen | 1 d | Paket mit neuem Index auf 18,25 Mio. Störungen installieren, währenddessen LZ4 ohne Fehler |
| **PL64** | Suche nach früher häufigen Wörtern (Befund aus H14, Rest aus PL55): Ein Wort mit wenigen Treffern in den letzten sieben Tagen, aber sehr vielen davor, durchsucht alles und endet an der Zeitgrenze von 10 s. Vorschlag: weitere Fenster (30 Tage, 1 Jahr) vor dem ganzen Bestand | ⬜ deine Entscheidung offen | 0,5 d | Lastwerkzeug: Suche nach einem solchen Wort p95 ≤ 300 ms auf `ki-lab` |

### C — Meilenstein 2

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **PL31** | Bulk synchron bis 1.000 (E38): Abgleich über Quelle und Schlüssel, `row_hash`, Antwort je Zeile, `Idempotency-Key` (trägt KF12; Inventar G35; Hauptplan E106) | ✅ | 2 d | 1.000 Zeilen einmal → 1.000 angelegt; dieselben noch einmal → 1.000 unverändert, kein Verlauf, keine Ereignisse; mit gleichem `Idempotency-Key` gleiche Antwort; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL32** | Bulk asynchron: NDJSON-Strom in den Dateispeicher, Job mit `COPY` in eine temporäre Tabelle und `INSERT … ON CONFLICT`, Stand per `/api/v1/jobs/{id}` (Hauptplan E107) | ✅ | 2 d | 100.000 Zeilen als NDJSON → `202` mit Job-ID, Stand bis „fertig", Zahlen stimmen; LZ5 hält in der Messung aus H11; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL33** | Vollstand je Quelle (`mode=full`) und Unterobjekte als Vollstand je Elternteil | ⬜ | 2 d | Lauf 2 ohne 10 Geräte aus Lauf 1 → genau diese 10 als verschwunden markiert, nicht gelöscht; ein Computer mit 150 Programmen ist ein Abgleich; LZ6 bis LZ8 halten in H11; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL34** | Feldhoheit je Quelle und Identifikationsregeln in CEL (Inventar G30) | ⬜ | 1,5 d | Ein von Hand geändertes Feld bleibt nach dem nächsten Bulk-Lauf erhalten; ein Gerät ohne Schlüssel wird über die Seriennummer erkannt; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL35** | Vorlagen je Typ und Notizen an jedem Typ (Inventar G31, G115) | ⬜ | 1 d | Anlegen aus einer Vorlage übernimmt deren Werte; Notizen erscheinen nur bei Typen mit Merkmal `notes`; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL36** | Datensatz im Organisationsbaum verschieben; verwaiste Dateien aufräumen (Inventar G91, G120) | ⬜ | 1 d | Nach dem Verschieben sieht die alte Einheit den Datensatz nicht mehr, die neue schon; eine Datei ohne Verknüpfung ist nach dem Job weg; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL37** | API-Abdeckung der Pakete von Meilenstein 2 und 3 prüfen: Komponenten, Dashboards, Verwaltung, Notizen, Regeln, Projekte, Berichte, Werkzeuge (Inventar G210, G212, G217, G218, G220, G221, G222, G224) | ⬜ | 1 d | wie PL30 für diese Typen; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL40** | Formeln über Beziehungen: Summe, Anzahl, Minimum und Maximum über verknüpfte Datensätze, neu berechnet, wenn sich ein verknüpfter Datensatz oder die Beziehung ändert (trägt KF6; aus PL22 abgetrennt, Hauptplan E99) | ⬜ | 2 d | Ein Datensatz summiert ein Feld seiner verknüpften Datensätze; ein neuer verknüpfter Datensatz, eine entfernte Beziehung und ein geänderter Wert ändern die Summe; `docker compose run --rm tools scripts/ci.sh` grün |

### D — Meilenstein 3 und 4

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **PL38** | CalDAV-Server für Planung und Notizen (E26) (Inventar G227) | ⬜ | 2 d | Thunderbird abonniert den Kalender einer Person und zeigt ihre Aufgaben mit Termin; `docker compose run --rm tools scripts/ci.sh` grün |
| **PL39** | Aufbewahrung je Typ: Partitionen im Voraus anlegen, abgelaufene löschen oder auslagern (E47); Verlauf und Vorgänge nach Frist (Inventar G175, G244) | ⬜ | 2 d | Ein Typ mit Monatspartitionen hat nach dem Wartungsjob die nächsten drei Monate angelegt; eine abgelaufene Partition ist gelöscht; `docker compose run --rm tools scripts/ci.sh` grün |

**Summe:** 80,5 d; 64 d geschätzt vor dem Bau (Block A: 10 d), PL40 am 2026-10-27 aus PL22 abgetrennt, PL41 am 2026-10-28 aus dem Befund von WF4 dazu, PL42 am 2026-10-29 vor UI3 eingeschoben (Hauptplan E121), PL43 am 2026-10-29 vor UI25 eingeschoben (Hauptplan E135), PL44 bis PL48 am 2026-10-29 für die Mandanten (Hauptplan E136, E137), PL47 am 2026-10-30 in PL47a bis PL47e geteilt und von 2,5 d auf 6,5 d geschätzt (Hauptplan E141). Mit PL49 am 2026-10-31 aus dem SD-Plan (§2.3, N1) 81,5 d. PL51 am 2026-11-02 für die CI (Hauptplan E150): 82 d. PL52 am 2026-11-03 für eine schnellere CI (Hauptplan E162): 83 d.

**Stand der Reihenfolge (2026-10-28, Hauptplan E108):** Block B ist fertig; PL31 und PL32 aus Block C sind vorgezogen gebaut. PL41 (Verteiler in Stapeln, Hauptplan E112) ist auf deine Entscheidung nach dem Befund von WF4 in Block B dazugekommen und vor WF8 gebaut. PL42 (Zeitleiste, Hauptplan E121) ist am 2026-10-29 auf deine Entscheidung vor UI3 eingeschoben, weil die Detailseite Folgemeldungen braucht, die es noch nicht gab. Der Rest von Block C und Block D wartet bis nach Meilenstein 1; vorher kommen WF und UI. PL49 (laufende Nummer) ist am 2026-10-31 aus dem SD-Plan dazugekommen und steht vor SD2. PL50 (zwei CIs gleichzeitig) ist am 2026-10-31 dazugekommen und am selben Tag wieder entfallen, weil nur noch ein Agent arbeitet (Hauptplan E149). PL52 (schnellere CI) ist am 2026-11-03 auf deine Entscheidung dazugekommen und kommt direkt nach SD4 (Hauptplan E162).

**Reihenfolge:** Block A ist der Durchstich (Hauptplan E59): PL1 bis PL5 in dieser Reihenfolge, zusammen mit UI1, UI2 und WF1; PL29 läuft daneben. Davor muss Hauptplan H11 stehen. Abgenommen wird der Durchstich als Ganzes mit Hauptplan H13. Danach Block B: zuerst PL6 bis PL10 (Organisationsbaum, Personen, Rechte), weil fast alles danach Rechte braucht; dann PL23 und PL24 (Jobs, Ereignisse), weil WF darauf aufsetzt; der Rest in der Reihenfolge der Tabelle. Block C nach Meilenstein 1, Block D mit Meilenstein 3 und 4.

---

## 1. Entscheidungen

Die übergreifenden Entscheidungen stehen im Hauptplan §1 und in [`adr/`](adr/).

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | LDAP-Testserver ist `lldap` im Compose-Profil `auth`; er wird nicht ausgeliefert (V1) | 2026-10-20 | Pascal |
| **E2** | YAML über `goccy/go-yaml` (MIT), weil `gopkg.in/yaml.v3` seit 2025 nicht mehr gepflegt wird; UUIDv7 über `google/uuid` (BSD-3); `sqlc` bildet `uuid` auf `uuid.UUID` ab | 2026-10-20 | Umsetzung, in PL1 |
| **E3** | Das Probe-Paket für den Durchstich liegt unter `packages/probe` und hat je Feldtyp ein Feld; nach dem Durchstich wird es durch die echten Pakete abgelöst, bleibt aber als Testpaket | 2026-10-20 | Umsetzung, in PL1 |
| **E4** | Eindeutige Felder bekommen einen partiellen eindeutigen Index, der Datensätze im Papierkorb ausnimmt (`WHERE deleted_at IS NULL`); ebenso `(source, external_key)`, eindeutig wo `source` gesetzt ist | 2026-10-20 | Umsetzung, in PL2 |
| **E5** | Beim Anlegen einer Typtabelle entstehen die Indexe in derselben Transaktion (die Tabelle ist leer); Indexe auf bestehende, gefüllte Tabellen kommen später als Job mit `CREATE INDEX CONCURRENTLY` (Architektur §3.2) | 2026-10-20 | Umsetzung, in PL2 |
| **E6** | Ein geänderter Feldtyp wird abgelehnt und benennt das Feld; der Umbau über eine neue Spalte und einen Füll-Job (Architektur §3.2) ist eine eigene Aufgabe in Block B oder C, sobald ein Paket ihn braucht | 2026-10-20 | Umsetzung, in PL2 |
| **E7** | Der Verlauf (`audit`) ist von Anfang an nach Monat partitioniert (E47) mit einer Standardpartition, die alles aufnimmt, bis PL39 die Monatspartitionen anlegt; je Schreibvorgang eine Zeile „create", „trash", „restore" oder „purge" und je geändertem Feld eine Zeile „update" mit altem und neuem Wert als JSON | 2026-10-20 | Umsetzung, in PL3 |
| **E8** | Zahlen (`numeric`) laufen durch die API als JSON-Zahl mit `float64`-Genauigkeit; Geldbeträge und Werte mit mehr als 15 Stellen brauchen später einen eigenen Feldtyp | 2026-10-20 | Umsetzung, in PL3 |
| **E10** | `oapi-codegen` 2.8.0 läuft als Go-Tool aus `go.mod` (`go tool oapi-codegen`), kein Eintrag im Werkzeug-Container; die CI erzeugt den Server neu und bricht ab, wenn `internal/httpapi/gen/api.gen.go` nicht aktuell ist. Die wirksame OpenAPI-Beschreibung entsteht zur Laufzeit aus der eingebetteten Spec plus je Typ einem Schema und den konkreten Pfaden `/api/v1/records/<paket>.<typ>…`; die generischen Pfade mit `{typeName}` bleiben für Werkzeuge erhalten | 2026-10-20 | Umsetzung, in PL4 |
| **E11** | HTTP-Abbildung der Fehler: 400 `bad_request` (Pfad, Parameter, Body), 404 `not_found`, 409 `conflict`, 422 `validation` mit `errors`, 428 `precondition_required` (fehlendes `If-Match`), 500 `internal` ohne Details; `type` ist `/problems/<code>`, der Titel folgt `Accept-Language` (Deutsch, Englisch) | 2026-10-20 | Umsetzung, in PL4 |
| **E12** | Listen: `limit` bis 500, Sortierung vorerst nur nach `id`, `created_at`, `updated_at`, `version` (NOT NULL, darum ein sauberer Schlüssel-Cursor aus Sortierwert und `id`); Gleichheitsfilter `filter[feld]=wert` auf Felder sowie `state`, `source`, `external_key`; `fields` wählt Spalten, `id` und `version` kommen immer. Alles Weitere bringt die JSON-Abfrage (PL14) | 2026-10-20 | Umsetzung, in PL4 |
| **E13** | Eine Sitzung ist ein JSON-Dokument unter `session:<id>` in Valkey (Konto, Name, `password_expired`, Anlage- und Auffrischzeit); die TTL des Schlüssels ist die Lebensdauer (Hauptplan E72). Aufgefrischt wird höchstens einmal je Minute, damit ein Lesen (LZ1) einen GET und keinen SET kostet | Sitzung in PostgreSQL; bei jeder Anfrage schreiben | 2026-10-20 |
| **E15** | Knoten des Organisationsbaums sind `ltree`-Pfade unter `root` (`root.it.support`), die Namen snake_case; die Tabelle `org_node` hält Pfad, Name und Label, ein Knoten ohne Kinder kann gelöscht werden, die Wurzel nie | 2026-10-20 | Umsetzung |
| **E16** | Einstellungen je Knoten liegen in `org_setting` (Pfad, Schlüssel in gepunktetem snake_case wie `ticket.default_priority`, Wert als JSON); wirksam ist je Schlüssel der nächste Knoten auf dem Weg nach oben (`path @> node`, nach Tiefe sortiert); die API nennt zu jedem wirksamen Wert seine Quelle | 2026-10-20 | Umsetzung |
| **E17** | `org_path` ist das eine Systemfeld, das ein Aufrufer setzen darf: beim Anlegen und Ändern eines Datensatzes als bestehender Knoten (sonst 422), Verschieben steht im Audit; `filter[org_path]=root.it` filtert den Unterbaum (`<@`, GiST-Index) | 2026-10-20 | Umsetzung |
| **E18** | Das Kernpaket `packages/core` wird wie jedes Paket installiert (`itsm package install packages/core`); die CI tut es vor der Lastmessung. `core.person` hat `last_name` als Titelfeld, Sprache und Aktiv-Kennzeichen; `core.group` hat `parent`, `responsible` und `purpose`; Mitgliedschaft ist `core.group_member` | 2026-10-22 | Umsetzung |
| **E19** | Abfragen über mehrere Typtabellen liegen im Paket `internal/query` (Architektur §3.1, E34); die erste ist `GET /api/v1/groups/{id}/members?recursive=` mit rekursivem CTE über `parent`, Papierkorb ausgenommen, höchstens 50 Ebenen | 2026-10-22 | Umsetzung |
| **E20** | Kerntabellen `role`, `permission` (Rolle, Typ oder `*`, Aktion oder `*`, Reichweite, später Bedingung und Feldrechte), `role_assignment` (Rolle, Person oder Gruppe, Knoten); `internal/authz` lädt je Anfrage die Berechtigungen der Person des Kontos samt Gruppen (`GrantsForPerson`, Gruppen über `internal/query`) und baut daraus eine SQL-Bedingung für Listen und Zählung (`Filter`) sowie eine Prüfung in Go für Einzelabruf und Schreiben (`Permits`); ohne Berechtigung `FALSE` | 2026-10-22 | Umsetzung |
| **E21** | Der Record-Dienst wendet die Rechte selbst an (`checkAccess`, `checkCreate`): Lesen filtert, unsichtbar → `ErrNotFound`, sichtbar ohne Recht → `ErrForbidden` (403); Anlegen prüft am Zielknoten, Verschieben am alten und neuen; Ändern und Übergang brauchen `update`, Papierkorb, Wiederherstellen und endgültig Löschen `delete`; der Verlauf ist so sichtbar wie der Datensatz. Ein Aufruf ohne Zugriffsobjekt im Kontext ist das System selbst (CLI, Tests) | 2026-10-22 | Umsetzung |
| **E22** | Rollen verwalten ist das Sonderrecht `manage_rights` am Typ `core.role` (in `admin` enthalten); Rollen lesen darf jede angemeldete Person. Listen zählen auf Wunsch (`count=true` → `total`) mit derselben Bedingung. Die Regel von E34 prüft `scripts/check-type-tables.sh` als eigener CI-Schritt, weil golangci-lint keinen Linter für Zeichenkettenliterale hat | 2026-10-22 | Umsetzung |
| **E23** | Feldrechte stehen je Berechtigung als `fields: {read: [...], write: [...]}`; ohne Angabe gelten alle Felder. Lesbar ist die Vereinigung über alle greifenden Berechtigungen, Systemfelder immer; nicht lesbare Felder fehlen in Datensatz, Liste, Typdefinition und OpenAPI (`records.VisibleType`), nicht schreibbare Felder lehnt der Record-Dienst mit 403 ab | 2026-10-22 | Umsetzung |
| **E24** | Der Record-Dienst meldet jeden erfolgreichen Schreibvorgang über `AfterWrite(typ)`; das Binary lässt damit bei `core.group_member` den Rechte-Cache verfallen. Derselbe Haken trägt später die Outbox-Verteilung (PL24) | 2026-10-22 | Umsetzung |
| **E25** | Rivers Tabellen (`river_job`, `river_queue`, …) legt `itsm migrate` nach den goose-Migrationen über Rivers eigenen Migrator an; River führt sie in `river_migration`, getrennt von den goose-Versionen. sqlc sieht sie nicht, der Zugriff läuft nur über Rivers Client in `internal/jobs` | 2026-10-26 | Umsetzung |
| **E26** | Die Rolle `api` hält einen River-Client ohne Worker (nur einfügen und lesen), die Rolle `worker` arbeitet die Warteschlangen ab; Zeitplan-Jobs setzt nur der von River gewählte Leader ab, also je Datenbank genau einmal. Eine Änderung an `job_schedule` erreicht jede Instanz über Valkey Pub/Sub (Kanal `jobs.schedule`) | 2026-10-26 | Umsetzung |
| **E27** | Erster Zeitplan-Job ist `event.retention` (täglich 02:30, Warteschlange `default`): löscht Ereignisse, die älter sind als `ITSM_EVENT_RETENTION` (Vorgabe 7 Tage, Hauptplan E46). Benachrichtigungen (WF15) und Webhooks (PL25) bringen ihre Job-Arten mit und fügen sie über `EnqueueTx` in der Transaktion der auslösenden Änderung ein (E19) | 2026-10-26 | Umsetzung |
| **E28** | Jede Änderung eines Datensatzes (`create`, `update`, `trash`, `restore`, `purge`, `transition`) schreibt in derselben Transaktion genau ein Ereignis in `event`; die Tabelle heißt so, nicht `event_outbox`, und ist nicht nach Tag partitioniert (der Aufräumjob `event.retention` löscht nach Alter, E27). Kein `NOTIFY` in der Transaktion: PostgreSQL serialisiert Commits mit Benachrichtigung, das kostete LZ4 ein Vierfaches; stattdessen weckt `AfterWrite` nach dem Commit den Verteiler über Valkey (`events.wake`). Die Position ist eine Sequenz, jede lesende Abfrage (`ListEvents`, `LatestEventPosition`) filtert mit `age(xmin) > age(pg_snapshot_xmin(pg_current_snapshot())::text::xid)` auf den Horizont (E87) | 2026-10-26 | Umsetzung |
| **E29** | Der Verteiler läuft in der Rolle `worker` (`events.RunDispatcher`): geweckt über den Valkey-Kanal `events.wake` plus Abfrage jede Sekunde, liest ab seiner Position in `event_consumer`, veröffentlicht jedes Ereignis als JSON auf dem Valkey-Kanal `events` und ruft registrierte Abnehmer (`events.Handler`, für Regeln, Webhooks); Zustellung mindestens einmal, die Position wird je Stapel gespeichert, ein fehlgeschlagener Abnehmer wird protokolliert und hält den Strom nicht auf | 2026-10-26 | Umsetzung |
| **E30** | `GET /api/v1/events?after=` liefert gefilterte Ereignisse und im Kopf `ITSM-Last-Position` die zuletzt gelesene Position (auch wenn alles verborgen war); liegt `after` vor dem ältesten gehaltenen Ereignis, kommt 410. `GET /api/v1/events/position` nennt die neueste Position. SSE unter `/api/v1/events/stream`: mit `after` oder `Last-Event-ID` zuerst das Versäumte, dann live; ohne beides ab jetzt; je Nachricht `id` = Position, `event` = Art, `data` = Ereignis; Herzschlag alle 15 s; bei Überlauf des Puffers `event: overflow`, der Browser verbindet neu mit `Last-Event-ID` | 2026-10-26 | Umsetzung |
| **E31** | Paket `internal/authn` ist die Anmeldung über Anbieter (OIDC jetzt, LDAP-Sync PL12, API-Zugänge PL13); `internal/auth` bleibt Konten und Sitzungen. Ein Konto vom Anbieter hat den leeren Kennwort-Hash; `auth.Login` und `ChangePassword` lehnen es ab, ohne zu verraten, dass es existiert. Die Entdeckung des Anbieters läuft beim ersten Gebrauch und wird wiederholt, damit ein noch startendes Keycloak den Server nicht stoppt | 2026-10-26 | Umsetzung |
| **E32** | Der begonnene Anmeldevorgang (Nonce, PKCE-Verifier, Zielpfad) liegt 10 min in Valkey unter seinem `state`, nicht im Cookie; der Rückweg kennt nur Pfade dieser Seite (kein offener Redirect). Fehler enden auf `/login?error=state|refused|disabled|provider|sso`, nie als JSON, weil der Browser in einer Weiterleitung steckt | 2026-10-26 | Umsetzung |
| **E33** | Login-Regeln in `login_rule` (Name, Reihenfolge, CEL über `claims`, Rolle, Knoten, aktiv) mit API `/api/v1/auth/rules` (Recht `manage_rights`); `role_assignment.rule_id` kennzeichnet regelverwaltete Zuweisungen, Löschen einer Regel nimmt sie mit (`ON DELETE CASCADE`), Entzug und Vergabe verwerfen den Rechte-Cache. Eine Bedingung, die einen fehlenden Claim anspricht, greift nicht (kein Fehler) | 2026-10-26 | Umsetzung |
| **E34** | Der LDAP-Sync liest nur: Personen, Gruppen und Mitgliedschaften entstehen als `core.person`, `core.group`, `core.group_member` über den Record-Dienst (Audit und Ereignisse wie bei jeder Änderung), am Knoten des Verzeichnisses. Unverändertes schreibt er nicht (der Record-Dienst lässt leere Änderungen aus), so bleibt der Verlauf sauber. Attributvorgaben für OpenLDAP und lldap (`entryUUID`, `uid`, `givenName`, `sn`, `mail`, `telephoneNumber`, `mobile`, `cn`, `member`); Active Directory braucht `objectGUID` (binär, wird als UUID-Text geführt), `sAMAccountName`, `member` | 2026-10-26 | Umsetzung |
| **E35** | Verzeichnisse und Login-Regeln verwaltet, wer Rechte verwaltet (`manage_rights` an `core.role`): Wer Rollen vergibt, entscheidet auch, woher Personen und Gruppen kommen. Löschen eines Verzeichnisses löscht nur die Zuordnung, Personen und Gruppen bleiben | 2026-10-26 | Umsetzung |
| **E36** | Für die CI gibt es in `internal/authn/authntest` neben dem OIDC-Anbieter einen kleinen LDAP-Server (`gldap`): Bind mit festem DN, Suche beantwortet den Personen- und den Gruppenfilter mit den gesetzten Einträgen. lldap selbst prüft dein Prüfweg; seine Bootstrap-Daten liegen unter `compose/lldap` | 2026-10-26 | Umsetzung |
| **E37** | Ein Bearer-Token ergibt je Anfrage eine flüchtige Sitzung (`auth.Session` ohne Eintrag in Valkey): Rechte, Akteur im Audit und `auth/me` laufen wie bei einer Browser-Sitzung. Der CSRF-Kopf entfällt bei Bearer-Aufrufen und am Token-Endpunkt, weil kein Cookie im Spiel ist. Die Tokens eines Zugangs sind unter ihm gelistet, damit Sperren und Löschen sie widerrufen; Drehen des Geheimnisses lässt laufende Tokens gelten | 2026-10-26 | Umsetzung |
| **E38** | Die Rate zählt je Zugang und Minute in Valkey (`INCR` mit Ablauf, kein gleitendes Fenster); `Retry-After` nennt die Sekunden bis zur nächsten Minute. Die Adresse ist der Peer, hinter einem vertrauten Proxy der erste Eintrag von `X-Forwarded-For`; IPv4-in-IPv6 wird entpackt | 2026-10-26 | Umsetzung |
| **E39** | Pfade über Referenzen werden als korrelierte skalare Unterabfragen gebaut (`(SELECT h0.title FROM rec_… h0 WHERE h0.id = rec_….parent AND h0.deleted_at IS NULL)`), nicht als JOIN: so bleibt die Basistabelle ohne Alias, und die Rechtebedingung aus `authz.Filter` (nackte Spaltennamen) gilt unverändert; die äußere Spalte ist qualifiziert, weil der Zieltyp dieselbe Spalte haben kann (Elternteil vom gleichen Typ). Sortieren nach Pfaden geht nicht (kein Schlüsselsatz-Cursor), Werte werden mit `coerce` auf den Feldtyp geprüft, Muster für `contains` mit `\`, `%`, `_` maskiert | 2026-10-26 | Umsetzung |
| **E40** | Paket `internal/relations` nutzt den Record-Dienst für Rechte und Spuren: `records.Check` (Datensatz lesen und Aktion prüfen) und `records.Trace` (Audit und Ereignis ohne Zeilenänderung, Version bleibt). Der Graph wird je Richtung als rekursive CTE geholt und in Go zusammengesetzt (`both` = Vereinigung, kleinste Tiefe gewinnt); Titel der Knoten aus `title_field`. Die Sichtbarkeit je Knoten läuft über `records.Get` mit Zwischenspeicher je Aufruf, was bei Graphen mit hunderten Knoten reicht und für tausende später eine SQL-Lösung braucht | 2026-10-26 | Umsetzung |
| **E68** | **Suche neueste zuerst (PL55, deine Entscheidung 2026-11-14 nach H14):** Die Volltextsuche bewertete jeden Treffer; „Drucker defekt“ traf auf 18,25 Mio. Störungen rund eine Million und brauchte mehr als 10 s. Ein Zeitfenster mit den Indizes allein half wenig (2,6 s), weil schon die Trefferlisten der Indizes groß sind (Titel-Ähnlichkeit 3,9 Mio.). Jetzt geht die Suche je Typ rückwärts am Index auf `created_at` durch die letzten sieben Tage, nimmt die neuesten bis zu 1.000 Treffer und bewertet nur diese (auf `ki-lab` 3 ms); bringt das weniger Treffer als verlangt, sucht sie über alle Datensätze über den Suchindex, wo seltene Wörter und alte Nummern billig sind (6 ms). Ein Wort, das nur vor langer Zeit häufig war, bleibt teuer; dagegen steht die Zeitgrenze (E69) | über die Indizes in Zeitfenstern; alle Wörter müssen vorkommen; eigene Suchmaschine; erst als Befund | 2026-11-14 | Pascal |
| **E69** | **Zeitgrenze je API-Anfrage (PL56, deine Entscheidung 2026-11-14 nach H14):** Suchen von zehn Sekunden hielten alle Verbindungen des Pools, und Schreiben und Jobs warteten dahinter. Jetzt endet jede API-Anfrage nach `ITSM_REQUEST_TIMEOUT` (10 s, `0` für keine): ihr Kontext läuft ab, pgx bricht die laufende Abfrage in PostgreSQL ab, und die Anfrage antwortet 503 mit dem Problem `timeout`. Ausgenommen sind Export, Bulk-Strom, Ereignisstrom und Hochladen; Hintergrund-Jobs laufen ohnehin außerhalb von Anfragen | nur für Suche und Listen; keine Grenze | 2026-11-14 | Pascal |
| **E67** | **Rechtefilter zusammenfassen (PL57, deine Entscheidung 2026-11-15 nach H14):** Die Warteschlange eines Teams brauchte auf 18,25 Mio. Störungen 3 bis 8 s: die Rolle war viermal zugewiesen und die Mitgliedschaft im Team viermal eingetragen, jede Zuweisung wurde eine eigene ODER-Bedingung `id IN (SELECT … FROM actor …)`, und PostgreSQL hashte für jede alle 365.000 Rollen-Einträge des Teams. Jetzt zählt jedes Recht einmal, jede Gruppe einmal, und alle Rechte „beteiligt“ ohne Bedingung werden eine Bedingung über ihre Knoten. Dieselbe Rolle für denselben Inhaber am selben Knoten lässt sich nicht mehr doppelt zuweisen (Migration 00048, bestehende Doppelungen gehen, die älteste bleibt), und ein Paket kann einen Index als eindeutig erklären (`unique: true`, je Mandant außerhalb des Papierkorbs); das Kernpaket erklärt damit die Mitgliedschaft (Gruppe, Person) eindeutig. Stehen beim Installieren schon Doppelungen in der Tabelle, bricht die Installation mit den Spalten ab. Listen werden jedes Mal mit ihren Werten geplant (Nachtrag: der allgemeine Plan vorbereiteter Abfragen brauchte 16 s; die Suche behält den Plan-Speicher) | nur den Filter zusammenfassen; erst als Befund | 2026-11-15 | Pascal |
| **E66** | **Zwischenspeicher laden im Hintergrund (PL58, deine Entscheidung 2026-11-15 nach H14):** Die Speicher des Kerns für SLAs, Kalender der Uhren, Regeln, Einstellungen und Typen luden nach ihrer Frist in der Anfrage nach, die sie fragte. Ein Schreiben hält dabei seine Transaktion und damit eine Verbindung, das Nachladen braucht eine zweite; mit so vielen gleichzeitigen Schreibern wie der Pool Verbindungen hat (auf `ki-lab` 32 Anfragen, 16 Verbindungen) warteten alle aufeinander, bis die Anfragen nach 10 s aufgaben, und jedes Anlegen einer Störung scheiterte. Jetzt gibt ein Speicher immer den letzten Stand sofort heraus; ist er zu alt oder als veraltet gemeldet, liest genau ein Ablauf ihn im Hintergrund neu (`internal/memo`). Eine eigene Änderung (SLA, Kalender, Regel, Einstellung, Typ) liest der schreibende Prozess nach dem Commit gleich selbst, damit sie für seine nächste Anfrage gilt; die Meldung einer anderen Instanz markiert nur als veraltet. Beim Start lädt der Server die Speicher vor der ersten Anfrage | nur mehr Verbindungen; erst als Befund | 2026-11-15 | Pascal |
| **E65** | **Indizes je Typ (PL54, deine Entscheidung 2026-11-14 nach H14):** Die Tabellen der Typen hatten nur die Indizes des Kerns (Baum, Quelle, Änderung, Frist, Suche, eindeutige Felder). Die Liste „offene Störungen meines Teams, neueste zuerst“ las auf 18,25 Mio. Störungen die ganze Tabelle (13 GB, 6,2 s). Jetzt indiziert der Kern das Anlagedatum jedes Typs, und ein Paket erklärt in der Typdatei weitere Indizes für seine häufigen Listen (`indexes: - fields: [state, created_at]`): bis vier Spalten, Felder des Typs oder die Systemspalten `state`, `created_at`, `created_by`, `updated_at`, `updated_by`, `sla_due_at`, `source`, `external_key`; Textfelder nur mit `max_length` bis 500 (Grenze eines B-Baums). Erklärte Indizes tragen einen Kommentar und gehen wieder, wenn das Paket sie nicht mehr nennt. Der Bau eines Index auf einer großen Tabelle hält deren Schreiben für seine Dauer an, innerhalb der Installation (18,25 Mio. Störungen: zwei Indizes in rund einer Minute). Spalten ohne leere Werte sortieren ohne `NULLS LAST`, sonst kann der Index nicht rückwärts gelesen werden | nur feste Indizes im Kern; erst als Befund | 2026-11-14 | Pascal |
| **E64** | **Typen und Kalender im Speicher (PL53, deine Entscheidung 2026-11-13 nach dem Profil aus H14):** `records.Service.Type` las bei jedem Aufruf alle Definitionen aus der Steuer-Datenbank und dekodierte sie; beim Schreiben einer Störung war das mehr als die Hälfte der Zeit (Massenimport 55 Zeilen/s). Jetzt hält ein Speicher die Typen und lädt sie neu, wenn sich die Definitionen ändern: sofort nach einem Schreiben des eigenen Prozesses (`meta.DefinitionsChanged`), über einen Trigger mit `pg_notify` auf `itsm_definitions` für andere Instanzen und `itsm package install`, und spätestens nach einer Minute, falls eine Nachricht verloren ging. Die ausgegebenen Typen sind geteilt und werden nicht verändert. Die SLA-Uhren halten ihren Kalender zehn Sekunden (wie die Einstellungen), eine Änderung eines Kalenders oder SLA wirft ihn hier und über Valkey auf den anderen Instanzen sofort weg; der Kalender ordnet seine Zeiten einmal und ist damit zwischen Abläufen teilbar. Eigene Aufgabe vor H14 eingeschoben | jede Definition bei jedem Aufruf lesen; nur Ablaufzeit ohne Nachricht | 2026-11-13 | Pascal |
| **E63** | Laufende Nummer (PL49, deine Entscheidungen 2026-11-02, Hauptplan E157): **Zähler** ist eine Postgres-Sequenz je Typ, Feld und Mandant, ohne Sperre; Lücken nach abgebrochenen Transaktionen sind erlaubt. **Gespeichert** wird die Zahl (`bigint`), das Präfix kommt aus der Definition und gilt für alle Datensätze; sortiert numerisch, die Suche findet „INC-24817“ und „24817“. **Bestand:** bekommt ein Typ das Feld später, nummeriert ein Job die vorhandenen Datensätze nach Anlagezeit. **Import:** nur Bulk mit `source` darf eine Nummer mitbringen, die Sequenz springt danach über die höchste; Anlegen und PATCH setzen sie nie. Verworfen: Zählertabelle mit Zeilensperre; fertiger Text; nur neue Datensätze; Altnummern nur als Textfeld | 2026-11-02 | Pascal |
| **E62** | Nebendaten finden ihren Mandanten auf drei Wegen (PL45): eine eigene Spalte (`rule.tenant`, `webhook.tenant`, `security_event.tenant`), der Knoten des Datensatzes im Ereignis (`payload.org_path`, `delivery_failure.org_path`) oder das Konto (`account.tenant` für API-Clients). Wo das System im Auftrag arbeitet (Regeln, Benachrichtigungen, OIDC), bestimmt `org.TenantOf` den Mandanten aus dem Knoten, und `authz.SystemIn` hält Schreiben und Suchen darin. Jobs tragen den Mandanten als `tenant` in den Argumenten; `/jobs` filtert darüber mit `JobListParams.Where` | 2026-10-30 | Umsetzung |
| **E61** | Die Grenze des Mandanten (PL44) sitzt in `authz.Access`: Mandant der Anfrage ist der mit `Enter` gewählte, sonst der erste erlaubte, damit Arbeit im Auftrag eines Kontos (Jobs, Webhooks) seine Mandanten nie verlässt. `Filter` hängt `org_path <@ Mandant` an die ganze Bedingung, `Permits` prüft dasselbe; eine Rolle an einem Knoten außerhalb zählt nicht, auch nicht mit `all`. `Tenants` und `Home` stehen im Rechte-Cache, der gewählte Mandant nur in der Sitzung. Ein Konto der Installation ohne Rolle in einem Mandanten arbeitet in `root.NONE`, einem Pfad, den kein Knoten haben kann. Was zur Installation gehört (Rollen, Kundenschicht, Einstellungen der Wurzel), prüft `Installation`: das Recht aus einer Rolle an der Wurzel mit Reichweite `all` oder `subtree` | 2026-10-30 | Umsetzung |
| **E60** | Die Kundenschicht (PL43) liegt je Typ als ein Dokument mit der Liste der Änderungen in `definition` (Schicht `customer`); `meta.Types` führt sie bei jedem Lesen mit der Basis zusammen, damit Rechte, Datensätze und Oberfläche ohne Neustart die wirksame Definition sehen. `revision` ist der Hash der gespeicherten Schichten, nicht der zusammengeführten Definition, damit das Lesen nichts serialisiert | 2026-10-29 | Umsetzung |
| **E59** | Rechte am Organisationsbaum (Befund vor UI9, deine Entscheidung 2026-10-29): Pseudotyp `core.org` mit den Sonderrechten `manage_org` (Knoten anlegen und löschen; beim Anlegen zählt der Elternknoten) und `manage_settings` (Einstellungen setzen und entfernen), geprüft mit `Access.Permits` und dem Knoten als `org_path`, also mit der Reichweite der Zuweisung; eine Einheit verwaltet ihren Unterbaum, die Wurzel und ihre Einstellungen (darunter die Sicherheit aus PL28) nur, wer an der Wurzel zugewiesen ist. Lesen bleibt für jede Sitzung offen. Die Rolle `admin` hat beide über `*` | 2026-10-29 | Umsetzung |
| **E58** | Mehrere Reichweiten je Rolle (UI4, Hauptplan E124, deine Entscheidung 2026-10-29): Eine Rolle hält je Typ, Aktion und Reichweite eine Zeile (`UNIQUE (role_id, type_name, action, scope)`, Migration 00031), etwa `read` mit `own` und mit `involved`; die Zeilen gelten mit ODER wie die Zeilen mehrerer Rollen schon vorher. `PUT …/permissions` ersetzt nur die Zeile derselben Reichweite (Bedingung, Feldrechte); `DELETE …/permissions/{typ}/{aktion}` entfernt alle Zeilen, mit `?scope=` nur die eine. Das Ereignisprotokoll nennt als „vorher“ die Zeile derselben Reichweite, beim Entfernen ohne Reichweite die Liste | 2026-10-29 | Umsetzung |
| **E57** | Die Zeitleiste (`internal/records/timeline.go`): Einträge stehen in der Kerntabelle `entry` (Typ, Datensatz, Text bis 100.000 Zeichen, privat, Zeit, Konto) und werden nie geändert. `AddEntry` liest den Datensatz in der Transaktion, prüft `read` und `comment` (`checkAccess`), für einen privaten Eintrag `read_private` (`Access.Permits` je Datensatz, Bereich und Bedingung gelten wie bei jedem Recht), schreibt den Eintrag und das Ereignis `comment` mit Eintrag und `private` (nie den Text) zur aktuellen Version; Version und `updated_at` bleiben. `Timeline` liest Einträge (private nur mit `read_private`) und die Verlaufszeilen (Merkmal `history`), lässt Feldänderungen ohne Leserecht weg (`readableSet` wie die Versionen), sortiert nach Zeit und Kennung und holt die Namen der Konten (`account.display_name`) mit einer Abfrage. `notify` prüft bei einem privaten Eintrag je Person die aktiven Konten (`AccountsOfPerson`, `Load`) auf `read_private`. Das endgültige Löschen nimmt die Einträge mit, auch bei einem Typ, der das Merkmal nicht mehr hat | 2026-10-29 | Umsetzung |
| **E56** | Der Verteiler (`internal/events`): `Consume(name, BatchHandler)` meldet einen Abnehmer an; `live` ist eingebaut, sobald Valkey da ist, `webhooks` (`webhooks.HandleBatch`) und `rules` (`engine.HandleBatch`) meldet `main.go` an. Je Abnehmer läuft eine Schleife: Durchgang (Zeile sperren, bis 1.000 Ereignisse ab der Position lesen, Abnehmer, neue Position, Commit), bei vollem Stapel sofort weiter, sonst Wecken über Valkey oder Abfrage jede Sekunde. Eine neue Zeile beginnt am Ende des Stroms; Zeilen von Tests (`Prefix`) sind nicht dauerhaft. Webhooks lesen die aktiven Abos einmal je Stapel, filtern im Speicher, prüfen die Sichtbarkeit nur bei Treffern und rücken jedes Abo ans Stapelende vor; Regeln stellen je Ereignis mit Regeln danach einen Job ein (die Eindeutigkeit nach Argumenten entfällt, die Transaktion sorgt für genau einmal). `jobs.EnqueueManyTx` nutzt River `InsertManyFastTx`. Die Worker von Webhooks und Regeln schreiben beim letzten Versuch oder bei Abbruch nach `delivery_failure` (`events.RecordFailure`). Die Migration 00025 übernimmt die Position des alten `dispatcher` für die drei neuen Abnehmer | 2026-10-28 | Umsetzung |
| **E55** | Der Mengenweg ist `records.SyncBatch` (`internal/records/bulkset.go`): Er liest die schon gelieferten Datensätze des Stapels mit einer Abfrage, prüft jede Zeile in Go mit `validate`, `checkCreate`, `checkAccess`, `checkFields` und `calculate` wie der Zeilenweg (Organisationsknoten und Verweise werden je Stapel und Wert nur einmal nachgeschlagen), legt eine temporäre Tabelle mit allen Werten als Text an, füllt sie per `COPY` und schreibt mit einem `INSERT … SELECT … ON CONFLICT DO NOTHING RETURNING id` (Suchspalte aus den Werten), einem `UPDATE … FROM`, das nur die geänderten Felder setzt (`CASE WHEN feld = ANY(_changed)`) und die Version prüft, und je einem `COPY` in `audit` und `event`. Was die Anweisungen auslassen (ein Schlüssel oder eindeutiger Wert inzwischen vergeben, eine Version inzwischen geändert), geht danach über `records.Sync`; scheitert der Stapel an einer Bedingung der Datenbank, geht er ganz über `Sync`, damit die fehlerhafte Zeile ihren Fehler bekommt. Der Job (`internal/bulk/async.go`, Art `bulk.apply`, Queue `bulk`) lädt die Rechte des startenden Kontos, liest den Strom zeilenweise (eine Zeile über 1 MiB ist ein Fehler), meldet den Fortschritt nach jedem Stapel über `JobUpdate` und das Ergebnis über `RecordOutput`, und löscht den Strom am Ende oder nach dem letzten Versuch. `jobs.Job` hat dafür `output`. Im Lastwerkzeug ist LZ5 die neue Messart „Durchsatz eines Laufs“ (`Scenario.Throughput`); die CI setzt `ITSM_FILES_DIR` für den gemessenen Server | 2026-10-28 | Umsetzung |
| **E54** | Der Abgleich liegt in zwei Bausteinen: `internal/bulk` prüft den Aufruf (Quelle, Zahl der Zeilen), nimmt `external_key` aus der Zeile, rechnet `row_hash` (SHA-256 über das JSON der gelieferten Felder, Schlüssel sortiert) und sammelt die Ergebnisse; `records.Sync` schreibt eine Zeile, weil nur `records` die Typtabellen anfasst (E34). Sync sucht den Datensatz über `source` und `external_key` (bei Listen innerhalb der Liste), legt sonst mit `create` an, das Quelle, Schlüssel und Prüfsumme mitschreibt, oder ändert mit `update`, das die Prüfsumme setzt, auch wenn kein Feld sich ändert (dann ohne Version, Verlauf und Ereignis). Jede Zeile ist eine eigene Transaktion; ein Fehler der Datenbank selbst bricht den Aufruf ab, ein Fehler der Zeile nicht. Eine Änderung zwischen Lesen und Schreiben ist ein Konflikt der Zeile. Nebenbei behoben: Die wirksame Beschreibung setzte jeden Anfragekörper eines Typpfads auf das Schema des Typs, auch bei `:query`, Kosten und Dokumenten; jetzt nur noch dort, wo die statische Beschreibung `Record` sagt | 2026-10-28 | Umsetzung |
| **E53** | `TestAPICoverage` liegt in `internal/httpapi/coverage_test.go`. Er installiert jedes Paket aus `packages/` (Abhängigkeiten zuerst) und erzeugt je Typ einen kleinsten Datensatz: jedes beschreibbare Feld bekommt einen gültigen Wert (Text eindeutig und auf die Höchstlänge gekürzt, Zahl an der Untergrenze, erste Option einer Auswahl), Pflichtverweise zeigen auf den Datensatz des Zieltyps, freiwillige Verweise bleiben leer, weil sie auf den eigenen Typ zeigen können (Elternknoten eines Baums). Ein Ring aus Pflichtverweisen oder ein Verweis auf einen Typ ohne Paket lässt den Test scheitern. Die Beschreibung kommt vom Server, wird mit `openapi3.Loader` gelesen und mit `Validate` geprüft; die Objekt-Schemas der Komponenten bekommen `additionalProperties: false`. Am Ende landen die Datensätze im Papierkorb und werden endgültig gelöscht; bricht der Test vorher ab, löscht die Aufräumfunktion sie direkt. Typen, die nur in der gemeinsamen Datenbank stehen (von anderen Tests), prüft er nicht | 2026-10-28 | Umsetzung |
| **E52** | Die Lizenzprüfung ist ein eigenes kleines Programm, `test/licenses`, kein fremdes Werkzeug im Image. Die Go-Module kommen aus `go list -deps ./cmd/itsm` (ohne Standardbibliothek); ihre Lizenz erkennt `github.com/google/licensecheck` (BSD-3-Clause, vom Go-Team, auch hinter pkg.go.dev) aus den Dateien `LICENSE*`, `LICENCE*` und `COPYING*` im Wurzelverzeichnis des Moduls. Eine Datei zählt nur, wenn mindestens 70 % ihres Textes eine bekannte Lizenz sind (am 2026-10-28 lagen alle bei mindestens 85 %); mehrere Lizenzen in einer oder mehreren Dateien gelten zusammen („A AND B“). Die Web-Pakete liefert `pnpm licenses list --prod --json` mit dem SPDX-Ausdruck aus ihrer `package.json`; `OR` braucht eine erlaubte Seite, `AND` beide, `WITH` zählt wie die Lizenz davor. Erlaubte Lizenzen und Ausnahmen stehen in `test/licenses/licenses.txt`, die ins Programm eingebettet ist. `licensecheck` ist nur eine Abhängigkeit des Prüfprogramms, nicht des Binarys | 2026-10-28 | Umsetzung |
| **E51** | Die Regeln liegen in `internal/auth/policy.go` (Laden der Einstellungen mit Rückfall auf die Vorgaben, 30 Sekunden zwischengespeichert, nach einer Änderung über die API auf dieser Instanz sofort verworfen; `CheckPasswordWith` mit `WeakPasswordError`, der die gebrochenen Regeln nennt) und `internal/auth/lockout.go` (Zähler und Sperrschlüssel in Valkey, je Benutzername kleingeschrieben und je Adresse; ohne Valkey, also auf der Kommandozeile, keine Sperre). Die Sitzung trägt ihre Lebensdauern; `Sessions.Lifetimes` setzt sie beim Anlegen und gleicht sie bei jedem Zugriff an die Einstellungen an. `Login` bekommt die Adresse; die Prüfung der lokalen Anmeldung für `admins` macht `auth.Service.AllowLocal` mit den Rechten aus `authz`. Die Sperrliste ist eine mitgelieferte Datei (`common_passwords.txt`, gut 100 Grundwörter, deutsch und englisch, mit „polizei“), keine heruntergeladene Liste. Das Lastkonto `load` hat jetzt das Kennwort `Messlauf-Pw7!`, weil das alte den Benutzernamen enthielt; `ci.sh` setzt es auch bei einem bestehenden Konto | 2026-10-27 | Umsetzung |
| **E50** | Das Ereignisprotokoll liegt in `internal/seclog`; die HTTP-Schicht schreibt es in den Handlern (`signIn`, `changed` in `internal/httpapi/seclog.go`), weil nur sie Adresse und User-Agent kennt, die Kommandozeile mit `cliChanged` (User-Agent „itsm cli“, ohne Konto und Adresse). Den Grund eines Fehlschlags liefert `auth.LoginError`, der nach außen `ErrInvalidCredentials` bleibt. Den alten Stand liest der Handler vor der Änderung (Rolle, Berechtigung, Zuweisung, Regel, Zugang, Verzeichnis, Knoten, eigene Einstellung des Knotens). Ein Schreibfehler des Protokolls wird geloggt, bricht die Anmeldung aber nicht ab; geschrieben wird ohne den Abbruch der Anfrage. Nicht protokolliert werden einzelne Anfragen mit ungültigem Bearer-Token und abgelehnte Änderungsversuche (403) | 2026-10-27 | Umsetzung |
| **E49** | Die Suche liegt in `internal/records/search.go`, weil sie Typtabellen liest (E34); `internal/search` ist nur die Schnittstelle mit der PostgreSQL-Umsetzung. Die Suchspalte schreibt nach dem Zeilenschreiben ein zweites `UPDATE … SET search = <Ausdruck über die Spalten>` derselben Transaktion; englische Übersetzungen kommen per Unterabfrage aus `translation`. Die Suche baut je lesbarem Typ eine Unterabfrage mit Rechtebedingung und eigenem Limit, verbindet sie mit `UNION ALL` und berechnet `ts_headline` nur für die besten Treffer. Wer ein gesuchtes Feld nicht lesen darf, sucht in diesem Typ ohne Index über den Text der lesbaren Felder, damit kein verborgenes Feld einen Treffer verrät. Die Indizes legt `schema.Apply` an, für `list_entry` die Migration `00021_search.sql`. Lastszenario LZ3: 60 Probe-Datensätze, Suche nach „Drucker defekt“ | 2026-10-27 | Umsetzung |
| **E48** | Webhooks liegen in `internal/webhooks`. Der Verteiler ruft `Handle` je Ereignis; `DueWebhooks` liefert die aktiven Abonnements mit kleinerer Position und passenden Filtern, je Abonnement laufen Positionswechsel (`AdvanceWebhook`, nur wenn kleiner) und `EnqueueTx` in einer Transaktion. Die Rechte prüft `records.VisibleEvent` mit der Access des Besitzers aus `authz.Service.Load`, beim Einreihen und noch einmal beim Zustellen. Der Worker `webhooks.deliver` setzt `NextRetry` aus der eigenen Rückstellung; ein gelöschtes oder abgeschaltetes Abonnement und ein durch Aufbewahrung verschwundenes Ereignis beenden den Job ohne Versand, ein nicht mehr erlaubtes Ziel bricht ihn ab (`JobCancel`). Die Zustellungen liest `/deliveries` aus `river_job` (Argument `webhook`). Das Geheimnis liegt im Klartext in der Datenbank, weil HMAC es braucht | 2026-10-27 | Umsetzung |
| **E47** | Formeln: `internal/rules/formula.go` baut je Formel eine CEL-Umgebung mit einer Variablen je Feld (ohne das Feld selbst und ohne die Kostensummen) und liest die benutzten Felder aus der Referenztabelle des geprüften Ausdrucks. `meta.CompileFormulas` ordnet die Formeln nach Abhängigkeiten (Tiefensuche, Zyklus mit Pfad in der Meldung) und wird beim Installieren und in `records` benutzt; `records` hält die übersetzten Programme je Typ und Formelstand im Speicher. Eine ganze Zahl passt in ein Zahlenfeld; ein Ergebnis vom Typ `dyn` (Nachschlagen in einer Map wie bei der ITIL-Matrix) wird beim Rechnen geprüft. Der Neuberechnungs-Job liest je Transaktion 500 Datensätze nach ID mit `FOR UPDATE` und schreibt nur geänderte Felder; ein Datensatz, dessen Formel scheitert, behält seine Werte und wird gezählt. `Field.Calculated()` fasst Kostensummen und Formeln zusammen (nicht schreibbar, im Formular schreibgeschützt) | 2026-10-27 | Umsetzung |
| **E46** | Die Übersetzungen liegen in `internal/records/translations.go`. `splitTranslations` nimmt vor `validate` die Werte anderer Sprachen aus der Eingabe; was bleibt, ist die deutsche Eingabe mit den bekannten Regeln. Fehlt bei einem Pflichtfeld der deutsche Wert, meldet `validate` ihn wie bisher. Beim Ändern werden die gespeicherten Übersetzungen gelesen und nur Abweichungen zu Änderungen; sie laufen durch dieselbe Versions-, Verlaufs- und Ereignislogik wie Feldänderungen. Lesen bleibt in `records` deutsch; die HTTP-Schicht ruft nach Get, List, Query und den Schreibvorgängen `Localize` auf, das die Übersetzungen aller Datensätze einer Seite mit einer Abfrage lädt. Die Sprache trägt der Kontext (`records.WithLang`), gesetzt von der Middleware `withLanguage`, die auch die Titel der Fehlermeldungen nutzen. Die Oberfläche schickt bis UI3 fest `Accept-Language: de` und `Content-Language: de`, damit das Formular aus einem englischen Browser keinen englischen Wert in die deutsche Spalte schreibt | 2026-10-27 | Umsetzung |
| **E45** | Die Kosten liegen in `internal/records/costs.go`, weil die Summen Spalten der Typtabelle sind und nur `records` sie schreiben darf (E34). Ein Kern-Merkmal darf jetzt Felder mitbringen; `Field.Computed` markiert sie, `validate` weist sie mit `read_only` ab, das Schema legt sie als `numeric NOT NULL DEFAULT 0` an, Versionen und Zurücksetzen lassen sie aus. Jeder Schreibvorgang an einem Eintrag sperrt erst die Zeile des Datensatzes (`FOR UPDATE`) und rechnet dann beide Summen aus der Tabelle `cost` neu; so zählt jede Summe alle vorher bestätigten Einträge, auch bei parallelen Buchungen. Die Einstellungen liest `records` über `Service.Settings` (`org.Service.Setting`) | 2026-10-27 | Umsetzung |
| **E44** | Die Rekonstruktion liegt in `internal/records/versions.go`: `stateAt` nimmt den heutigen Datensatz und läuft die Verlaufszeilen (nach Zeit und ID sortiert, also nach Version aufsteigend) von hinten bis zur ersten Zeile mit Version ≤ N und setzt je Feld den alten Wert. Rückwärts statt vorwärts, weil der Verlauf beim Anlegen nur gesetzte Felder schreibt und `org_path` dort gar nicht steht; rückwärts stimmen auch Felder, die erst nach dem Anlegen zum Typ kamen. Zurücksetzen baut einen Patch aus den abweichenden, schreibbaren Feldern und ruft `Update`, damit Prüfung, Verlauf, Ereignis und Konflikterkennung dieselben bleiben. Die Methode heißt `RestoreVersion`, weil `Restore` schon den Papierkorb meint | 2026-10-27 | Umsetzung |
| **E43** | `internal/files` hat zwei Schnittstellen: `Store` (Put, Open, Delete; heute `FS`, das in `<schlüssel>.part` schreibt und am Ende umbenennt, so dass nie eine halbe Datei sichtbar ist) und `Scanner` (heute `NoScan`). Der Dienst legt die Zeile `uploading` vor dem Strom an, damit ein Absturz eine Spur für den Aufräumjob hinterlässt; jeder Fehler danach löscht Zeile und Inhalt sofort, mit einem eigenen Kontext, weil die Anfrage schon weg sein kann. Rechte laufen über den Record-Dienst (`Get`, `Check`, `Trace`); `Trace` schreibt bei `attach`/`detach` das Feld `documents`. Das Image legt `/data/files` mit dem Nutzer 65532 an und setzt `ITSM_FILES_DIR`, damit ein benanntes Volume beschreibbar startet; ohne `ITSM_FILES_DIR` antworten die Endpunkte 503 | 2026-10-27 | Umsetzung |
| **E42** | Merkmale bleiben Definitionssache: `meta.Trait` mit `CoreTraits` (Name, Beschriftung, Unterressourcen) und `Package.Traits` aus `traits/*.yaml`; `expandTraits` hängt die Felder beim Laden an, so dass Definition, Schema, OpenAPI und Oberfläche sie wie deklarierte Felder sehen, nur mit `trait` als Herkunft. Der Record-Dienst prüft `HasHistory` an einer Stelle (`audit`), die Unterressourcen hängen über `Type.SubResources` an der OpenAPI je Typ; Merkmale anderer Pakete sind noch nicht erlaubt (nur Kern und eigenes Paket), weil der Lader ohne Datenbank arbeitet | 2026-10-27 | Umsetzung |
| **E41** | Listen laufen durch den Record-Dienst wie jeder Typ: `schema.TableName` liefert für sie `list_entry`, `schema.Apply` überspringt sie, `records.listScope` hängt `list = 'paket.typ'` als Literal an (der Name ist ein geprüfter Bezeichner, so bleiben die Platzhalter-Nummern unberührt). Die Felder einer Liste setzt der Lader (`meta.expandList`), so dass Definition, OpenAPI und Oberfläche sie wie deklarierte Felder sehen; `full_name` ist keine Feld-, sondern eine abgeleitete Spalte (`extraColumns`), damit das Formular sie nicht schreibt. Das implizite Leserecht steht als `read_all` im geladenen Zugriff (`authz.Access`), gefüllt aus den installierten Listentypen; es greift in `Allowed`, `Filter`, `Permits` und den Feldrechten vor den Rollen und ignoriert die Einengung auf Knoten (PL10). Die Startwerte legt `records.SeedLists` mit Systemzugriff und ohne Akteur an, `itsm package install` ruft es nach dem Schema auf | 2026-10-27 | Umsetzung |
| **E14** | `itsm user create|passwd` lesen das Kennwort von stdin, nie aus einem Argument (Prozessliste, Shell-Verlauf); Kontobefehle brauchen kein Valkey | Argument `--password`; interaktive Abfrage | 2026-10-20 |
| **E9** | Löschen ist dreistufig: Papierkorb (`deleted_at`), Wiederherstellen, endgültig löschen nur aus dem Papierkorb. Ein Datensatz im Papierkorb ist in Listen und Einzelabruf unsichtbar; eine Wiederherstellung, die auf einen inzwischen eindeutigen Wert trifft, ist ein Konflikt mit Feldname | 2026-10-20 | Umsetzung, in PL3 |

## 2. Begründung

- **Was PL baut,** steht in der [Architektur](architektur.md) §2 bis §7 und §11: Meta-Modell,
  Schema, Rechte, Datensätze, Abfragen, Beziehungen, Bulk, Audit, Ereignisse, Jobs, Suche,
  Dateien, Cache.
- **Abgedeckt:** alle 82 Zeilen des Funktionsinventars mit Ziel PL, je in genau einer Aufgabe,
  und die Kernfähigkeiten KF1, KF2, KF6, KF9, KF12, KF13, KF17, KF18, KF19 als Träger.
  KF11 trägt UI13; die JSON-Abfrage in PL14 ist ihre Grundlage.
- **Nicht in PL:** Merkmale und Pakete als Format zum Austausch (EX), Discovery-Quellen (CM),
  SAML (Hauptplan E61, Meilenstein 3).

## 3. Offene Fragen

| # | Frage | Hängt daran |
| --- | --- | --- |
Keine offen.

**Beantwortet am 2026-10-20:** V1 (E1).

Die Fragen an die Fachseite stehen im Hauptplan.

## 4. Umsetzungsstand

### PL1 — Meta-Modell, Grundform, 2026-10-20

- **Grundlage:** Hauptplan E66 bis E68 (Paketordner mit `package.yaml` und `types/*.yaml`,
  snake_case mit übersetzten Labels, ein JSONB-Dokument je Typ und Schicht); Architektur §3.1
  (Systemspalten sind für Felder gesperrt) und §4.
- **Gebaut:** `internal/meta` mit Modell (Paket, Typ, Feld; sieben Feldtypen: text, number,
  boolean, date, datetime, choice, reference), strengem Laden (unbekannte Schlüssel sind Fehler,
  Typname gleich Dateiname), Validierung, die alle Probleme eines Pakets auf einmal nennt, je mit
  Ort und Grund, und Speichern in einer Transaktion. Migration `00002_definitions.sql` mit den
  Kerntabellen `package` und `definition` (Schicht `base`, `extension`, `customer`; eindeutig je
  Paket, Art, Name und Schicht). Befehle `itsm package install <dir>` und `itsm package list`.
  Das Paket `packages/probe` mit dem Typ `probe` und sieben Feldern. Eine Neuinstallation
  entfernt Basisdefinitionen, die das Paket nicht mehr mitbringt, und lässt andere Schichten
  unberührt.
- **Belegt, gemessen am 2026-10-20 im Werkzeug-Container:**
  - `itsm package install packages/probe` meldet „Paket probe 0.1.0 installiert" und den Typ
    `probe` mit seinen sieben Feldern, je mit Typ, Label und Eigenschaften.
  - Dasselbe Paket mit `type: colour` statt `choice` wird abgelehnt: „type probe, field kind:
    unknown field type "colour", allowed are text, number, …", Rückgabe 1.
  - Tests: Laden des Probe-Pakets; Ablehnen unbekannter Schlüssel und falscher Dateinamen; zwölf
    Validierungsfälle mit erwartetem Text (unbekannter Typ, Systemspalte, kein snake_case,
    doppeltes Feld, Option ohne Sprache, doppelte Option, Vorgabe außerhalb der Optionen, Verweis
    auf unbekannten Typ, `max_length` am Zahlfeld, min > max, falscher Vorgabetyp, Label ohne
    Englisch); Verweis auf installierten Typ; Rundlauf gegen PostgreSQL mit Neuinstallation,
    Bereinigung und Versionszähler. Linter ohne Befund.
- **Nicht geprüft:** Es entsteht noch keine Tabelle je Typ, das ist PL2. Schichten `extension`
  und `customer` sind nur als Wert vorgesehen, das Zusammenführen kommt mit EX. Verweise auf Typen
  anderer Pakete sind nur gegen installierte Typen geprüft, nicht gegen `depends`. Die
  Hauptplan-Prüfung mit `go run` setzt voraus, dass `ITSM_DATABASE_URL` und `ITSM_VALKEY_ADDR`
  gesetzt sind; im Werkzeug-Container sind es die Testwerte.

### PL2 — Schema, 2026-10-20

- **Grundlage:** E33, Architektur §3.1 und §3.2; E4 bis E6 dieses Plans.
- **Gebaut:** `internal/schema` mit `Apply`: legt je Typ `rec_<paket>_<typ>` mit den 13
  Systemspalten an (`id` UUIDv7, `org_path` ltree mit GiST-Index, `version`, `state`, `source`,
  `external_key`, `row_hash`, `search`, Zeitstempel, `deleted_at`), ergänzt fehlende Felder als
  Spalten (`ADD COLUMN` ohne flüchtigen Vorgabewert), legt eindeutige Indexe nach E4 an, lehnt
  Typwechsel mit Feldname ab (E6) und protokolliert jeden Schritt in der Kerntabelle
  `schema_change` (Migration `00003`). Alles in einer Transaktion unter
  `pg_advisory_xact_lock`. `itsm package install` ruft `Apply` nach dem Speichern der
  Definitionen auf und meldet die Zahl der Schemaänderungen; Logs gehen auf stderr, die Ausgabe
  bleibt lesbar.
- **Belegt, gemessen am 2026-10-20 im Werkzeug-Container:**
  - `itsm package install packages/probe` auf leerer Datenbank: 11 Schemaänderungen (Tabelle,
    drei Systemindexe, sieben Spalten); ein zweiter Aufruf: 0 Schemaänderungen.
  - Test: Die Tabelle hat genau die 20 erwarteten Spalten mit den erwarteten Typen
    (`information_schema.columns`), die Indexe `_title_key`, `_org_path_idx` und `_source_key`
    existieren, ein doppelter Titel wird abgewiesen, ein neues Feld erzeugt genau eine Änderung
    „add column", ein Typwechsel wird mit dem Feldnamen abgelehnt, das Protokoll hat mindestens
    13 Zeilen.
  - Test: acht parallele `Apply` auf denselben neuen Typ enden alle ohne Fehler, mit genau einer
    Tabelle und genau sechs Protokollzeilen (eine Anlage).
  - Linter ohne Befund, `go test -race ./...` grün.
- **Beleg für den Gate-Umbau nach E69** (am selben Tag, Last 3,6 bis 5,1 auf 16 Kernen):
  - CI-Lauf ohne Basis-Binary: „keine Basislinie gemessen, nur Werte", Gate bestanden.
  - `load baseline --binary=/tmp/itsm`: Binary im Volume, `test/load/baseline.json` mit
    Prüfsumme `df6e5da4fb7f…`, `approved_by` leer.
  - CI-Lauf mit A/B: LP1 0,61 ms gegen 0,56 ms (+9 %), LP2 0,92 gegen 0,93 ms (−1 %), bestanden.
  - CI-Lauf mit 3 ms Bremse vor dem neuen Stand: LP1 5,10 gegen 0,55 ms, LP2 5,43 gegen 0,89
    ms, beide `FAIL`, Rückgabe 1.
  - Unterwegs gefunden und behoben: Der neue Stand wurde immer zuerst gemessen; auf dem
    belasteten Rechner reichte das für 98 % Unterschied bei identischem Code. Jetzt wechselt die
    Reihenfolge je Wiederholung (A B, B A, …), vier Wiederholungen zu 4 s.
- **Nicht geprüft:** Spalten entfernen und Typwechsel (E6) sind nicht gebaut. Der partielle
  eindeutige Index schließt den Papierkorb aus; ob ein wiederhergestellter Datensatz mit einem
  inzwischen angelegten Duplikat kollidiert, behandelt PL3. Die Testdatenbank ist die
  Entwicklungsdatenbank; die Tests räumen nur ihre eigenen Pakete `metatest` und `schematest`
  auf. Das +9 % bei identischem Code zeigt, dass auch die A/B-Messung auf dem Laptop rund 10 %
  Rauschen hat; die Schwelle 10 % plus 0,1 ms hält das knapp. Bei einem echten Zweifelsfall
  entscheidet eine Wiederholung bei freiem Rechner.

### PL3 — Datensätze, 2026-10-20

- **Grundlage:** E29 (Konflikterkennung), E43 (eigener SQL-Builder), E70 (flache JSON-Form),
  Architektur §3.1 und §6.2; E7 bis E9 dieses Plans.
- **Gebaut:** `internal/records` mit `Get`, `List` (Seiten per Schlüssel über die UUIDv7-ID),
  `Create`, `Update`, `Trash`, `Restore`, `Purge`, `History`. Validierung gegen die Definition:
  Pflicht, Vorgabewerte beim Anlegen, Länge, Bereich, Optionen, Datum als `2006-01-02`,
  Zeitpunkt als RFC 3339, Verweis muss auf einen vorhandenen Datensatz zeigen, unbekannte und
  Systemfelder werden mit Code benannt; alle Probleme eines Aufrufs auf einmal. Jede Schreibung
  ist eine Transaktion mit `SELECT … FOR UPDATE`, Versionsprüfung, `UPDATE … WHERE version = $n`
  und den Verlaufszeilen. Eindeutigkeitsverstöße werden zu Konflikten mit Feldname. Migration
  `00004_audit.sql` (E7). SQL entsteht aus den Spalten der Definition mit `pgx.Identifier`; das
  ist der Anfang des Builders aus E43.
- **Belegt, gemessen am 2026-10-20 im Werkzeug-Container** (Tests gegen PostgreSQL, alle grün,
  Linter ohne Befund):
  - Anlegen mit Vorgabewerten: `priority` 3 und `active` true aus der Definition, Datum und
    Zeitpunkt in der API-Form; Verlauf hat eine Zeile „create" und je gesetztem Feld eine Zeile.
  - Validierung nennt sieben Probleme auf einmal mit Code: `required`, `max`, `choice`, `type`,
    `unknown`, `read_only`, `reference`.
  - Zwei Schreiber auf Version 1: der zweite bekommt den Konflikt; ein unveränderter Wert erzeugt
    weder Version noch Verlauf; ein Feld auf `null` setzen leert es; je geändertem Feld genau eine
    Verlaufszeile mit altem und neuem Wert.
  - Papierkorb: Ein Datensatz im Papierkorb fehlt in Liste und Einzelabruf; zweimal in den
    Papierkorb ist ein Konflikt; ein Duplikat des eindeutigen Titels ist erlaubt, solange das
    Original im Papierkorb liegt (E4), und die Wiederherstellung scheitert dann als Konflikt mit
    `title`; nach dem endgültigen Löschen des Duplikats gelingt sie; endgültig löschen außerhalb
    des Papierkorbs ist ein Konflikt. Seiten zu 2 über 3 Datensätze: zwei Seiten, `next` nur auf
    der ersten.
- **Nicht geprüft:** Kein Ereignis in der Outbox (PL24), keine Rechte (PL8), `created_by` und
  `updated_by` bleiben leer bis PL5, `row_hash` und `search` bis PL31 und PL26. Die Typdefinition
  wird bei jedem Aufruf aus der Datenbank gelesen; der Cache kommt mit PL9. Ein Verweis wird nur
  beim Schreiben geprüft; wird das Ziel später in den Papierkorb gelegt, bleibt der Verweis
  stehen. Die Hauptplan-Messung LZ1 und LZ4 läuft erst mit PL4, wenn es die Endpunkte gibt.

### PL4 — REST-API je Typ, 2026-10-20

- **Grundlage:** E11, E29, E60, E70, E71; Architektur §6.1; E10 bis E12 dieses Plans.
- **Gebaut:** `api/openapi.yaml` als fester Vertrag (System, Typen, Datensätze mit Anlegen,
  Lesen, Ändern, Papierkorb, Wiederherstellen, endgültig Löschen, Verlauf; Problem-Schemas);
  `oapi-codegen` erzeugt `internal/httpapi/gen` (chi-Routen, Parameter, Modelle, eingebettete
  Spec). `internal/httpapi/api.go` setzt die Schnittstelle um: ETag aus der Version, `If-Match`
  Pflicht beim Ändern, Body-Grenze 1 MiB, Problem Details nach E11, `/api/v1/openapi.json` mit
  den Typen zur Laufzeit. `internal/records` bekam `ListOptions` (E12) und `Types`. Das Binary
  verdrahtet den Record-Dienst; die Worker-Rolle hat weiter nur `/healthz` und `/readyz`.
- **Belegt, gemessen am 2026-10-20 im Werkzeug-Container** (Tests gegen PostgreSQL mit dem
  Probe-Paket, alle grün, Linter ohne Befund):
  - Anlegen → 201 mit `ETag: "1"` und `Location`; Lesen → 200 mit ETag; Ändern ohne `If-Match`
    → 428; mit falscher Version → 409; mit richtiger → 200 und `ETag: "2"`; Verlauf zeigt
    „create" und den alten Wert; Papierkorb → 204, danach 404; endgültig löschen → 204;
    Wiederherstellen danach → 404.
  - Validierung → 422 als `application/problem+json` mit drei `errors` und deutschem Titel; mit
    `Accept-Language: en-US` englischer Titel. Falsche UUID → 400, unbekannter Typ → 404,
    Body kein Objekt → 400, unbekannter Endpunkt → 404 als Problem.
  - Liste: `limit=2&sort=-created_at&fields=title` liefert zwei Seiten mit Cursor, Filter auf
    Feld und Zahl trifft einen Datensatz, `sort=title` → 422, `limit=abc` → 400.
  - `/api/v1/types/probe.probe` liefert die Definition; `/api/v1/openapi.json` enthält die
    Pfade `/api/v1/records/probe.probe` und `…/{id}`, die generischen Pfade und ein Schema
    `probe.probe` mit allen Feldern, `kind` als Enum mit drei Werten.
- **Nicht geprüft:** keine Anmeldung und keine Rechte (PL5, PL8): Jeder Aufruf ist erlaubt;
  der Durchstich nimmt das in Kauf, der Service Desk nicht. Keine Validierung der Anfragen
  gegen die Spec zur Laufzeit; die Prüfung macht `records`. Der TypeScript-Client aus der
  OpenAPI entsteht mit UI2. LZ1, LZ2 und LZ4 sind ab jetzt messbar, LZ2 und LZ4 vorläufig ohne
  Rechte und Regeln.
- **Nachgetragen am 2026-10-20, erste Messung von LZ1, LZ2, LZ4** (schnelle Stufe, 8 Arbeiter,
  Last 4,3 auf 16 Kernen, Tabelle mit rund 60 Datensätzen, ohne Rechte und Regeln):
  - LZ1 Lesen p95 1,70 ms bei 8.156 Anfragen/s; LZ2 Liste mit Filter p95 4,25 ms bei 3.094
    Anfragen/s; LZ4 Anlegen p95 6,54 ms bei 1.724 Anfragen/s. Die Ziele (50, 150, 100 ms)
    gelten erst in der vollen Stufe mit Prod-Daten (PL31 bis PL33); die Werte sind nur ein
    erster Anhalt. Das Basis-Binary (vor PL4) kennt die Endpunkte nicht, das Gate meldete „die
    Basislinie beherrscht das Szenario noch nicht, kein Vergleich".
  - Dabei gefunden und behoben: Konnte die Basislinie ein Szenario nicht vorbereiten, lief die
    Messschleife trotzdem und zählte 390 Mio. Fehler in 16 s Leerlauf. Jetzt wird die Seite
    nicht gemessen, das Ergebnis trägt `unavailable` mit dem Grund, ein Test deckt es ab.
  - Das Gate wurde rot bei LP2 (+24 % bei unberührtem `/readyz`); deine Entscheidung E76:
    bester Lauf statt Median. Der CI-Lauf mit beiden Änderungen steht im Umsetzungsstand von
    H11 im Hauptplan.

### PL5 — Anmeldung mit lokalen Konten, 2026-10-20

- **Grundlage:** Hauptplan E10, E72 bis E75, E77, E78; E13 und E14 dieses Plans; Architektur §6.
- **Gebaut:** Tabelle `account` (Migration 00005; Benutzername snake_case per CHECK, Hash,
  Kennwortablauf, Sperre). `internal/auth`: Argon2id im PHC-Format (19 MiB, 2 Durchgänge, 1
  Spur, Parameter aus dem Hash gelesen), Kennwortregeln E73, Sitzungen in Valkey mit TTL
  (12 h gleitend, 24 h hart, Auffrischen höchstens je Minute), Dienst mit Anmelden, Abmelden,
  Konto anlegen, Kennwort setzen und ändern, sperren, auflisten. `internal/httpapi/auth.go`:
  `POST /api/v1/auth/login` setzt das Cookie `itsm_session` (HttpOnly, SameSite=Lax, Secure
  hinter TLS), `logout`, `me`, `password`; Middleware prüft CSRF-Header bei POST/PATCH/DELETE
  unter `/api` (403), dann die Sitzung (401), dann den Kennwortablauf (403 `password_expired`,
  nur `me`, `password`, `logout` bleiben offen). Schreibende Aufrufe tragen das Konto als
  `created_by`/`updated_by` und als `actor` im Audit. `itsm user create|passwd|disable|enable|
  list`. `scripts/ci.sh` legt das Konto `load` an, das Lastwerkzeug meldet sich je Ziel an
  (Cookie und Header in jeder Anfrage); LZ4 bekam dafür einen Vorbereitungsschritt.
  `api/openapi.yaml` beschreibt die vier Endpunkte und das Cookie als `securityScheme`.
- **Belegt, gemessen am 2026-10-20 im Werkzeug-Container** (Tests gegen PostgreSQL und
  Valkey, alle grün, Linter ohne Befund):
  - Kennwortregeln: `Secret-1!`, `Aa1!aaaa` und `Pässwörd-9` gehen durch; leer, 7 Zeichen, ohne
    Groß-, ohne Klein-, ohne Ziffer, ohne Sonderzeichen werden abgelehnt. Zwei Hashes desselben
    Kennworts unterscheiden sich (Salt); ein fremdes Hashformat wird abgewiesen.
  - Dienst: ungültiger Name, schwaches Kennwort, doppelter Name werden abgelehnt; falsches
    Kennwort, unbekanntes Konto und gesperrtes Konto geben denselben Fehler. Nach dem Anmelden
    hat der Schlüssel `session:<id>` in Valkey eine TTL von 12 h (auf 5 s genau); nach dem
    Abmelden ist die Sitzung weg. Abgelaufenes Kennwort: Sitzung mit `password_expired`, Ändern
    mit falschem alten oder schwachem neuen Kennwort abgelehnt, nach dem Ändern ist die Sitzung
    frei. Gleitende Verlängerung: nach 30 s kein Schreiben, nach 2 min aufgefrischt, nach 24 h
    plus 1 s ist die Sitzung ungültig, auch wenn der Schlüssel noch existiert.
  - HTTP (Prüfweg): ohne Cookie 401 für Datensätze, Typen und OpenAPI, 200 für Version und
    `/healthz`; unsinniges Cookie 401; mit Cookie 201 und `created_by` ist das Konto aus
    `auth/me`; POST ohne `X-Requested-With: itsm` 403 (auch beim Anmelden), GET ohne Header
    200; falsches Kennwort 401 `invalid_credentials`, fehlendes Kennwort 422; `Set-Cookie`
    enthält `HttpOnly`, `SameSite=Lax`, `Path=/` und ohne TLS kein `Secure`; Abmelden 204, danach
    401 für diese, 200 für eine andere Sitzung. Abgelaufenes Kennwort: Datensätze 403
    `password_expired`, `me` 200, Ändern 422/422/204, danach Datensätze 200 und
    `password_expired: false`.
  - Alle übrigen Tests weiter grün (`go test -race ./...`); der Router-Test für unbekannte
    Endpunkte erwartet ohne Sitzung jetzt 401 (E78).
  - Prüfweg von Hand im Werkzeug-Container mit dem gebauten Binary: `itsm user create pruefweg`
    mit Kennwort auf stdin → Konto; `curl` ohne Cookie 401; Anmelden liefert das Konto und
    `session_expires_at` 12 h später; mit Cookie 200; Anlegen ohne `X-Requested-With` 403, mit
    Header 201 und `created_by` = Konto; Abmelden 204; danach 401; `itsm user list` zeigt das
    Konto als aktiv.
  - `scripts/ci.sh` komplett grün (Lizenzköpfe bis Web-Build). Lastmessung mit Anmeldung je
    Ziel: LZ1 p95 1,53 ms bei 8.356 Anfragen/s, LZ2 3,36 ms bei 3.914/s, LZ4 5,87 ms bei
    1.717/s; die Sitzung kostet das Lesen also keinen messbaren Aufschlag gegenüber PL4
    (1,70 ms). Das Basis-Binary kennt die Anmeldung nicht: „nicht messbar: login as load: 404",
    Gate berichtet ohne Vergleich. LP1 −4 %, LP2 ±0 % im selben Lauf, Gate bestanden.

### PL6 — Organisationsbaum, 2026-10-22

- **Grundlage:** Hauptplan E16, E34; Architektur §5; E15 bis E17 dieses Plans; PL2 (GiST-Index
  auf `org_path` je Typtabelle).
- **Gebaut:** Migration 00007 (`org_node` mit `root`, `org_setting`), `internal/org`
  (Knoten anlegen, lesen, löschen; Einstellung setzen, entfernen, wirksame Einstellungen mit
  Quelle), API `GET|POST /api/v1/org/nodes`, `GET|DELETE …/{path}`, `GET …/{path}/settings`,
  `PUT|DELETE …/{path}/settings/{key}`; `records` nimmt `org_path` beim Anlegen und Ändern an,
  prüft den Knoten, schreibt den Wechsel ins Audit und filtert Listen per `filter[org_path]`
  nach Unterbaum. Der Werkzeug-Container kennt jetzt `ITSM_DATABASE_URL` und
  `ITSM_VALKEY_ADDR`, so dass `go run ./cmd/itsm package install …` dort ohne Zusatz läuft (dein
  Befund vom 2026-10-22).
- **Belegt, 2026-10-22 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Prüfweg: Einstellung `apitest.priority` an der Wurzel 3, am Kind `root.apitest` 1 → der
    Enkel `root.apitest.team` erbt 1 mit Quelle `root.apitest`; nach dem Entfernen am Kind erbt
    er 3 von `root`. Ungültiger Name, fehlender Elternknoten, doppelter Name, ungültiger
    Schlüssel und kaputtes JSON werden abgewiesen; Löschen mit Kindern → 409, Wurzel → 404,
    Blatt → 204.
  - `EXPLAIN SELECT … WHERE org_path <@ 'root.orgtest'` nutzt `rec_probe_probe_org_path_idx`
    (GiST); der Test setzt `enable_seqscan = off`, weil der Planer auf der kleinen Tabelle
    sonst sequenziell liest.
  - Datensätze: Anlegen mit unbekanntem Knoten → 422, mit `root.apitest.team` → 201 und
    `org_path` im Datensatz, ohne Angabe → `root`; `filter[org_path]=root.apitest` liefert
    genau den Datensatz im Unterbaum, ein ungültiger Pfad → 422; Verschieben per PATCH → Version
    2 und Audit-Zeile am Feld `org_path`.
  - `go test -race ./...`, Linter und Web-Tests grün; die Lastmessung der kompletten CI kam am 2026-10-22 nicht zum Zug, weil der Rechner fremd ausgelastet war (Last 12 bis 24 auf 16 Kernen durch eine Supabase-Umgebung und Chrome-Testläufe; Lastwächter E69, Rückgabe 3). Der Selbsttest „Basislinie ohne Endpunkt" des Lastwerkzeugs lief davor mit der echten Rauschgrenze (0,1 ms) und wurde unter `-race` rot (+22 % bei identischem Fake-Server); er nutzt jetzt wie der A/B-Selbsttest 5 ms Boden. **Offen: ein kompletter `scripts/ci.sh`-Lauf bei freiem Rechner** (deine Prüfung).
- **Nicht geprüft:** Rechte je Knoten (PL8) und Sichtbarkeit nach Unterbaum in Listen kommen mit
  PL8; Knoten umbenennen oder verschieben (mit allen Datensätzen und Kindern) ist nicht gebaut;
  Einstellungen haben noch keine Schlüsselliste mit Typen (PL28 und die Pakete definieren sie);
  keine Oberfläche für den Baum (UI); der GiST-Nachweis gilt für die Testtabelle, die Messung
  mit Prod-Daten kommt mit PL31 bis PL33.
- **Nachgetragen am 2026-10-22:** `scripts/ci.sh` komplett grün bei freiem Rechner (Gate
  bestanden); der erste Lauf scheiterte nur daran, dass der Vergleich der erzeugten Web-Typen
  mit meiner parallelen Änderung an `api/openapi.yaml` für PL7 kollidierte.
- **Befund vom 2026-10-29, behoben auf deine Entscheidung (E59):** Die Rechte am Baum hatte PL8
  nicht nachgezogen. Jedes angemeldete Konto konnte Knoten anlegen und löschen und
  Einstellungen setzen und entfernen, auch die Kennwortregeln aus PL28; belegt mit einem
  Konto, das nur Probe-Datensätze lesen darf (204 und 201), die Werte danach zurückgesetzt.
  Gefunden beim Vorbereiten von UI9. Jetzt prüfen die Handler die Sonderrechte `manage_org`
  (Knoten) und `manage_settings` (Einstellungen) am Pseudotyp `core.org` mit der Reichweite
  der Zuweisung; Lesen bleibt offen. Belegt in `TestOrgRightsOverHTTP`: ohne Recht 403 bei
  allen sechs Schreibwegen; eine Einheitsverwaltung (beide Rechte, Unterbaum, zugewiesen an
  `root.orgrights`) legt dort einen Knoten an, setzt und entfernt dort eine Einstellung, nicht
  aber an der Wurzel (Kennwortregel 403, Knoten unter der Wurzel 403, Wurzel löschen 403);
  Lesen von Knoten und Einstellungen bleibt 200. Die übrigen Handler ohne eigene Prüfung
  laufen über den Datensatzdienst und tragen dessen Rechte oder sind Anmeldewege. Ganze CI grün am 2026-10-29 im ersten Lauf (40 Vitest-, 5 Playwright-Fälle, Leistungsgate bestanden; LZ1 p95 3,39 ms, LZ4 8,97 ms).

### PL7 — Personen und Gruppen, 2026-10-22

- **Grundlage:** Hauptplan E79, E80 (deine Antworten vom 2026-10-22); Architektur §3.1; E18 und
  E19 dieses Plans; PL1 bis PL4, UI2.
- **Gebaut:** `packages/core` mit `person` (Nachname, Vorname, Titel, Mailadresse, Telefon,
  Mobil, vorgesetzte Person, Sprache, aktiv), `person_email` (Person, Adresse, Art),
  `group` (Name, Zweck, übergeordnete Gruppe, verantwortliche Person, aktiv) und
  `group_member` (Gruppe, Person); `internal/query/groups.go` mit den Mitgliedern einer Gruppe,
  flach oder rekursiv; `GET /api/v1/groups/{id}/members` in `api/openapi.yaml` und
  `internal/httpapi/groups.go`; die CI installiert `core` neben `probe`.
- **Belegt, 2026-10-22 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Prüfweg per HTTP: drei Personen über `/api/v1/records/core.person` (eine mit Titel, eine mit
    Vorgesetzter, Vorgaben `language=de`, `active=true`), eine zweite Adresse über
    `core.person_email` mit `filter[person]`, Person ohne Nachname → 422. Gruppenbaum IT >
    Support > Frontline mit je einem Mitglied: `/members` ohne `recursive` liefert nur das
    Mitglied von IT, mit `recursive=true` alle drei in der Reihenfolge der Ebenen samt
    Gruppenname, Person und Mailadresse; Mitgliedschaft ohne Person → 422; eine Mitgliedschaft im
    Papierkorb und eine Person im Papierkorb fallen heraus; unbekannte Gruppe → 404.
  - Die Liste, das Formular und die Statusknöpfe der Oberfläche funktionieren für die neuen
    Typen ohne Änderung, weil sie aus der Definition entstehen (UI2).
  - `scripts/ci.sh` komplett grün (Go, Last mit Gate, Web mit Browser-Tests).
- **Nicht geprüft:** keine Eindeutigkeit von Mailadressen (eine Person kann dieselbe Adresse
  zweimal eintragen); das Paar Gruppe/Person in `group_member` ist nicht eindeutig (zweifache
  Mitgliedschaft möglich, Eindeutigkeit über mehrere Felder kommt mit dem Meta-Modell später);
  Verweisfelder prüfen noch nicht, ob das Ziel existiert (PL3 lässt jede UUID zu); die
  Verknüpfung Konto ↔ Person kommt mit PL8; keine Oberfläche für Mitglieder (UI3).
- **Nachtrag SD14 (2026-11-10, Hauptplan E174, SD-Plan E13):** Das Paket `core` bringt zwei
  weitere Typen für die Startseite des Portals: `core.portal_tile` (Kacheln) und
  `core.announcement` (Hinweise von Hand mit Gültigkeit), beide mit Leser-Regeln wie der
  Artikel, dazu als erste Startdaten des Kerns eine Regel, die vorbei gegangene Hinweise auf
  „Abgelaufen“ setzt (`core` 0.2.0).

### PL8 — Rollen und Rechte, Teil 1, 2026-10-22

- **Grundlage:** Hauptplan E34, E81, E82; Architektur §5; E20 bis E22 dieses Plans; PL5 bis
  PL7.
- **Gebaut:** Migration 00008 (`role`, `permission`, `role_assignment`, `account.person_id`,
  Rolle `admin`), `internal/authz` (Zugriff laden, `Filter`, `Permits`, Rollen, Berechtigungen,
  Zuweisungen, Konto ↔ Person), Rechte im Record-Dienst (`internal/records/access.go` und die
  Aufrufe in Liste, Lesen, Anlegen, Ändern, Papierkorb, Löschen, Übergang, Verlauf), Zählung
  `count=true` mit `total`, Middleware lädt den Zugriff nach der Sitzung, API `/api/v1/roles`,
  `/api/v1/roles/{name}/permissions`, `/api/v1/assignments`; CLI `itsm user link` und
  `itsm user grant`; `scripts/check-type-tables.sh` als CI-Schritt; die CI gibt dem Konto
  `load` die Rolle `admin`, die API-Tests ihrem Konto `apitest` ebenso.
- **Belegt, 2026-10-22 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Prüfweg: vier Personen mit Konten und je einer Rolle am Knoten `root.rights` (`own`,
    `unit`, `subtree`, `group`), Datensätze an `root`, `root.rights`, `root.rights.sub` und
    einer von der Person mit `own`: jede sieht genau ihre Datensätze in Liste, Einzelabruf und
    Zählung (`own` 1, `unit` 2, `subtree` 3, `group` über eine gemeinsame Gruppe 1, admin
    alle). Anlegen außerhalb des Knotens → 403, `unit` unterhalb des Knotens → 403; Ändern ohne
    Recht → 403, Ändern eines unsichtbaren → 404, Papierkorb ohne Recht → 403, Verlauf eines
    unsichtbaren → 404, Übergang ohne `update` → 403; Rollen lesen darf jede Person, anlegen nur
    mit `manage_rights` (403); `admin` löschen → 422; ungültige Reichweite → 422; nach dem
    Entzug einer Zuweisung ist der Datensatz mit der nächsten Anfrage unsichtbar und die
    Zählung 0.
  - `scripts/check-type-tables.sh`: auf dem sauberen Baum grün; eine eingeschleuste Zeile
    `"SELECT id FROM rec_probe_probe"` in `internal/platform` bricht ihn mit Dateiname und
    Zeile.
  - Alle anderen Tests weiter grün: die Oberfläche und das Lastwerkzeug arbeiten mit dem
    Konto `load`, das `admin` hat.
  - `scripts/ci.sh` komplett grün: Lizenzköpfe, Typtabellen-Prüfung, Go-Tests, Last mit Gate, Web mit 20 Vitest- und 2 Browser-Tests.
- **Nicht geprüft:** Bedingungen in CEL, Rechte je Feld und der Cache kommen mit PL9 (der
  Zugriff wird je Anfrage mit zwei bis drei Abfragen geladen); Rechte je Übergang (WF) und
  Sonderrechte der Pakete sind nur als Aktionen möglich, noch nicht im Typ definiert; LDAP- und
  OIDC-Gruppen (G121) kommen mit PL9/PL10 der Anmeldung; keine Oberfläche für Rollen; die
  Reichweite `group` meint „angelegt von jemandem aus meiner Gruppe", die Zuständigkeit am
  Datensatz (Bearbeitergruppe) kommt mit WF3 und erweitert die Reichweite dann.

### PL9 — Rollen und Rechte, Teil 2, 2026-10-22

- **Grundlage:** Hauptplan E16 (CEL), E34, E83; Architektur §5; E23 und E24 dieses Plans; PL8.
- **Gebaut:** `internal/authz/celsql.go` (CEL → SQL über den CEL-Syntaxbaum: Vergleiche eines
  Feldes mit einem Literal, `in` mit Liste, `&&`, `||`, `!`, `null`; alles andere → 422 beim
  Speichern), Bedingungen in `Filter` (SQL) und `Permits` (CEL in Go), Feldrechte
  (`ReadableFields`, `WritableFields`), Cache im Prozess und in Valkey mit Generation
  `authz:gen` und Kanal `authz.gen` (`Service.Run` abonniert, `Invalidate` erhöht und meldet),
  `internal/cache/pubsub.go` (Incr, Publish, Subscribe), `records.AfterWrite` für
  Mitgliedschaften; Typen und OpenAPI je Sitzung gefiltert.
- **Belegt, 2026-10-22 im Werkzeug-Container** (Tests gegen PostgreSQL und Valkey, Linter ohne
  Befund):
  - Übersetzer: acht Ausdrücke ergeben das erwartete SQL mit Platzhaltern und Argumenten
    (auch gedrehte Vergleiche, `in`, `IS NULL`, Fortzählung der Platzhalter); neun
    unübersetzbare (Funktionen, Feld gegen Feld, bloßes Feld, Literal gegen Literal, `null` in
    Liste, `has`, Syntaxfehler) werden mit `ErrInvalid` abgelehnt.
  - Prüfweg per HTTP: Rolle mit `read` und `update` auf `probe.probe`, Bedingung
    `record.kind == "fault"`, Felder lesen `title`, `kind`, schreiben `title`: der Einzelabruf
    zeigt `title` und `kind`, nicht `priority`, Systemfelder bleiben; die Frage (kein Fehler) ist
    404 und fehlt in der Liste (`total` 0); `/api/v1/types/probe.probe` nennt zwei Felder, die
    OpenAPI enthält `priority` nicht; `title` ändern → 200, `priority` → 403; eine unübersetzbare
    Bedingung → 422. **Zwei Instanzen:** zweite Instanz mit eigenem Rechte-Dienst auf derselben
    Datenbank und demselben Valkey; Entzug der Zuweisung auf der zweiten → die erste liefert
    binnen einer Sekunde 404 (im Test nach wenigen Millisekunden).
  - Unterwegs gefunden: Die Anmeldung lädt den Zugriff jetzt mit, deshalb war eine danach
    angelegte Gruppenmitgliedschaft 5 s lang unsichtbar; seither verwirft `AfterWrite` bei
    `core.group_member` den Cache (E24).
  - `scripts/ci.sh` komplett grün bei Last 3,6 (Go, Typtabellen-Prüfung, Last mit Gate, Web mit 20 Vitest- und 2 Browser-Tests); ein erster Lauf bei Last 8 wurde am Gate rot (LP1 `/healthz` +31 % bei unverändertem Pfad, Einzelläufe der Basislinie 0,70 bis 1,27 ms), reines Rauschen durch fremde Last.
- **Nicht geprüft:** Bedingungen sehen nur den Datensatz, nicht die Person (`person.…` in CEL
  kommt mit WF3/Rollen am Datensatz); Feldrechte gelten nicht für Filter und Sortierung
  (ein Filter auf ein unsichtbares Feld ist möglich); keine Oberfläche für Rollen; Valkey-
  Ausfall: dann zählt jede Instanz für sich und lädt je Anfrage neu (Hinweis im Log).

### PL10 — Aktive Rolle und Organisationseinheit, 2026-10-22

- **Grundlage:** Hauptplan E84; Inventar G240; PL5, PL8.
- **Gebaut:** `Session.ActiveRole`/`ActiveOrg` in Valkey, `POST /api/v1/auth/context`
  (`internal/httpapi/auth.go`), Prüfung gegen die eigenen Rollen und Knoten, die Middleware
  engt den geladenen Zugriff je Anfrage ein (Kopie, der Cache bleibt ganz), `auth/me` nennt
  `active_role`, `active_org`, `roles`, `nodes`.
- **Belegt, 2026-10-22 im Werkzeug-Container:** Person mit Leserolle an `root.ctx.east` und
  `root.ctx.west`, Schreibrolle nur an `east`: ohne Wahl zwei Datensätze, `me` nennt zwei Rollen
  und zwei Knoten; Wahl `west` → nur der West-Datensatz, der Ost-Datensatz 404, Anlegen in
  `east` 403, `me` zeigt die Wahl; Wahl der Leserolle allein → Ändern in `east` 403; Aufheben →
  Ändern 200; fremde Rolle und Knoten oberhalb der eigenen → 422.
  `scripts/ci.sh` komplett grün bei Last 3,6 (Go, Typtabellen-Prüfung, Last mit Gate, Web mit 20 Vitest- und 2 Browser-Tests); ein erster Lauf bei Last 8 wurde am Gate rot (LP1 `/healthz` +31 % bei unverändertem Pfad, Einzelläufe der Basislinie 0,70 bis 1,27 ms), reines Rauschen durch fremde Last.
- **Nicht geprüft:** keine Oberfläche für den Wechsel (UI3 oder UI10); die Wahl gilt je
  Sitzung, nicht je Gerät; Regeln, die bei der Anmeldung eine Rolle vorwählen, kommen mit PL9
  der Anmeldung (LDAP/OIDC-Gruppen, G121).

### PL23 — Jobs mit River, 2026-10-26

- **Grundlage:** Hauptplan E19, E46, E53 (River 0.49.0), E85, E86; Architektur §1, §2
  (`internal/jobs`), §11; Inventar G126, G166, G183; E25 bis E27 dieses Plans; deine
  Entscheidungen vom 2026-10-26 (Cron-Ausdruck, Vorgabe im Code mit Tabelle, drei Warteschlangen).
- **Gebaut:** `internal/jobs` (Dienst um River: `AddWorker`, `AddPeriodic`, `Start`/`Stop`,
  `Enqueue`/`EnqueueTx`, `Schedules`, `SetSchedule`, `RunNow`, `List`, `Get`, `Cancel`, `Retry`;
  `cron.go` mit robfig/cron v3; `retention.go` mit dem ersten Zeitplan-Job), Migration
  `00009_jobs.sql` (`job_schedule`) und Rivers Migrationen in `db.Migrate`, Konfiguration
  `ITSM_WORKERS_INTERACTIVE|DEFAULT|BULK` und `ITSM_EVENT_RETENTION`, Endpunkte
  `GET /api/v1/jobs` (Filter `queue`, `kind`, `state`, `limit`), `GET /api/v1/jobs/{id}`,
  `POST …/{id}:cancel`, `POST …/{id}:retry`, `GET /api/v1/jobs/schedules`,
  `GET|PATCH /api/v1/jobs/schedules/{name}`, `POST …/{name}:run` (`internal/httpapi/jobs.go`);
  `cmd/itsm serve` startet den Job-Dienst nach Rolle und beendet ihn geordnet, das Platzhalter-
  paket `internal/worker` ist weg. Unterwegs behoben: die Typliste lieferte Typen ohne lesbares
  Feld als `"fields": null` und auch Typen ohne Leserecht (Fehler `t.fields is not iterable` in
  deiner Oberfläche); jetzt nur lesbare Typen, jeder mit Feld-Array (Test in `rights2_test.go`).
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium „bulk verzögert interactive nicht": sechs Jobs zu je 1 s in `bulk` mit zwei
    Workern, danach ein Job in `interactive`: der ist nach 0,28 bis 0,30 s fertig (Grenze im Test
    1,5 s), die sechs Bulk-Jobs erst nach 3,28 bis 3,30 s (mindestens 2,9 s, sonst wäre die
    Begrenzung auf zwei Worker nicht wirksam); das Protokoll zeigt die sechs als `completed` in
    `bulk` mit ihren Argumenten.
  - Zeitpläne: Vorgabe `*/5 * * * *` mit nächstem Lauf binnen 5 min; Ausschalten → kein nächster
    Lauf, `updated_by` gesetzt; Überschreiben auf `0 3 * * *` behält „aus", Einschalten → nächster
    Lauf um 03:00; `61 * * * *` und `every day` → `ErrInvalid`, unbekannter Name → `ErrNotFound`;
    die Tabelle hält den Stand; „sofort ausführen" legt den Job in seine Warteschlange, nach dem
    Lauf nennt der Zeitplan ihn als letzten Lauf (`completed`); Abbrechen eines für später
    geplanten Jobs → `cancelled`, Wiederholen → `available`; zweimal registrieren und ein
    Zeitplan-Job mit falschem Cron werden beim Registrieren abgelehnt.
  - Aufräumen der Ereignisse: ein 8 Tage altes Ereignis verschwindet, ein frisches bleibt;
    `event.retention` steht mit `30 2 * * *` in `default`.
  - API: Liste der Zeitpläne, PATCH (aus, `0 4 * * *`) → 200 mit Vorgabe und wirksamem Wert,
    `nope` → 422, unbekannter Name → 404, `:run` → 202 mit dem Job (`available`, weil im API-Test
    kein Worker läuft), Protokoll mit Filtern, unbekannter Zustand → 422, Einzelabruf, `:cancel`
    → `cancelled`, `:retry` → `available`, unbekannte ID → 404, keine Zahl → 400; ein Konto ohne
    Recht an `core.job` bekommt auf allen fünf Endpunkten 403.
  - Konfiguration: Vorgaben 8/8/2 und 168 h; `ITSM_WORKERS_BULK=0`, `=16`, `48h` werden gelesen;
    `-1` und `soon` werden gemeldet. `itsm migrate` ist weiterhin wiederholbar (db-Test).
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün bei Last 3,8: alle Go-Pakete, Lastgate bestanden (LP1 0,41 ms, LP2 0,56 ms, LZ4 4,88 ms p95 im Schnellstadium), Web-Lint, -Format, -Tests, -Build und Browsertests.
- **Nicht geprüft:** keine Oberfläche für Zeitpläne und Protokoll (UI-Plan, G183 „Verwaltung in
  der Oberfläche"); Leader-Wahl und Pub/Sub-Nachladen nur mit einer Instanz getestet; Cron läuft
  in der lokalen Zeit des Workers (`TZ` des Containers), nicht je Organisationseinheit;
  Wiederholung nach Rivers Vorgabe (bis 25 Versuche, exponentiell), kein Zeitlimit je Job-Art;
  eine Überschreibung in `job_schedule` für einen nicht mehr registrierten Job bleibt
  wirkungslos liegen; keine Metriken zu Rückstand und Dauer (BT-Plan); Benachrichtigungen und
  Webhooks als Job-Arten kommen mit WF15 und PL25.

### PL24 — Ereignisse, 2026-10-26

- **Grundlage:** Hauptplan E39, E46, E87; Architektur §7; KF13; WF1 (E5 des Workflow-Plans);
  E28 bis E30 dieses Plans; deine Entscheidungen vom 2026-10-26 (Zähler mit Zeilensperre, nur
  lesbare Datensätze im Strom).
- **Gebaut:** Migration `00010_event_stream.sql` (`event_counter`, `event_consumer`, Position
  ohne Sequenz), `records.event` schreibt das Ereignis in jedem Schreibpfad samt `NOTIFY`,
  `records.VisibleEvent` wendet die Rechte je Ereignis an, Paket `internal/events` (`Stream`,
  `Latest`, `Subscribe`, `RunDispatcher`/`Dispatch`, `Handle`), Endpunkte
  `GET /api/v1/events` (410, Kopf `ITSM-Last-Position`), `GET /api/v1/events/position`,
  `GET /api/v1/events/stream` (SSE) in `internal/httpapi/events.go`; `cmd/itsm serve` startet den
  Verteiler in der Rolle `worker`.
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL und Valkey, Linter ohne
  Befund):
  - Zwanzig gleichzeitige Anlagen → zwanzig Ereignisse mit lückenlosen Positionen in
    Reihenfolge (Zähler mit Sperre).
  - Anlegen, Ändern, Übergang, Papierkorb, Wiederherstellen, Papierkorb, endgültig löschen →
    sieben Ereignisse in dieser Reihenfolge mit Version (2 beim Ändern, 3 beim Übergang, 6 beim
    Löschen), geänderten Feldern (zwei beim Ändern, `state` beim Übergang), Einheit `root`.
  - Verteiler: ab Position gesetzt, `Dispatch` veröffentlicht die sieben auf Valkey in
    Reihenfolge, ein Abonnent empfängt sie, die Position steht danach auf der letzten.
  - Abnahmekriterium: ein Abonnent bei Position 5 holt 6 und 7 nach; nach dem Löschen der
    ersten fünf ist Position 3 „gone" (`ErrGone`, 410 über HTTP), Position 5 liefert weiter.
  - HTTP: zehn Anlagen → zehn Ereignisse in Reihenfolge mit passenden Datensatz-IDs,
    `ITSM-Last-Position` = Start + 10; ab Start + 5 kommen genau die Ereignisse 6 bis 10. Eine
    Person mit Reichweite `own` sieht von elf Ereignissen nur ihr eigenes, der Kopf nennt
    trotzdem die neueste Position; nach endgültigem Löschen (eigenes und fremdes) sieht sie ihre
    drei (Anlegen, Papierkorb, Löschen), nicht die fremden. SSE mit `after` = Start + 8 beginnt
    mit `id: Start + 9`, meldet `: connected`, und eine danach angelegte Datensatz kommt live
    über den Verteiler als `data:` mit Art `create`. Der Statusmodell-Test (WF1) sieht die fünf
    Übergänge weiter in Reihenfolge, jetzt zwischen Anlegen und Ändern.
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün bei Last 3 bis 4: alle Go-Pakete (die Ereignis-Tests filtern nach eigenen Datensätzen, weil die Pakete parallel gegen dieselbe Datenbank testen), Lastgate bestanden (LP1 0,43 ms, LP2 0,67 ms, LZ1 1,79 ms, LZ2 3,63 ms p95), Web-Lint, -Format, -Tests, -Build und Browsertests. **Befund LZ4 (Schreiben):** p95 31 ms, p50 12 ms, 550/s (vor PL24: p95 4,9 ms, p50 4,1 ms, 1963/s); Ziel ≤ 100 ms gehalten, das Gate hat keine Basislinie für LZ4. Ursache ist die gewählte Zählersperre (E87): jede schreibende Transaktion hält die Zählerzeile bis zum Commit samt fsync, bei parallelen Schreibern warten sie darauf hintereinander; das deckelt den Schreibdurchsatz je Datenbank auf etwa 1 / Commit-Dauer. Alternative bleibt die Sequenz mit Sichtbarkeitshorizont (keine Sperre, keine Serialisierung). Entscheidung dazu offen, siehe Hauptplan.
- **Umbau am selben Tag (E87 revidiert):** Nach deinem „ja" zur Empfehlung ersetzt Migration
  `00011_event_sequence.sql` die Zählerzeile durch die Sequenz; `ListEvents` und
  `LatestEventPosition` filtern auf den Sichtbarkeitshorizont. Belegt: zwanzig gleichzeitige
  Anlagen erscheinen alle mit steigender Position; ein Ereignis in einer offenen Transaktion
  hält jedes spätere zurück (ein danach angelegter und bestätigter Datensatz bleibt unsichtbar,
  auch `Latest` bleibt davor), nach dem Commit erscheinen beide in Reihenfolge. Die Messung
  danach zeigte LZ4 immer noch bei p95 19,6 ms, 477/s: zweite Ursache war `pg_notify` in der
  Schreibtransaktion (PostgreSQL serialisiert Commits mit Benachrichtigung). Seit der Verteiler
  nach dem Commit über Valkey geweckt wird, liegt LZ4 wieder bei p95 5,2 ms, p50 4,1 ms,
  1908/s (Schnellstadium, Werkzeug-Container, Last 3 bis 4). Die vier Pakete, die nebeneinander
  gegen dieselbe Datenbank testen, stören sich nicht mehr: der Ereignis-Test hat ein eigenes
  Paket `eventstest`, die Job-Tests eigene Warteschlangen, River holt nur bekannte Job-Arten
  (`FetchOnlyKnownKinds`, so dass auch dein laufender Entwicklungsserver keine Test-Jobs
  ausführt). Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 nach dem Horizont-Umbau grün (alle Go-Pakete, Lastgate, Web, Browsertests); der Lauf nach dem Valkey-Wecken folgt mit PL11.
- **Nicht geprüft:** keine Partitionierung nach Tag (Architektur §7 nennt sie; bei 7 Tagen
  Aufbewahrung reicht das Löschen nach Alter, Messung mit H11); Webhooks und interne Abnehmer
  (Regeln, Fristen) hängen noch nicht am Verteiler (PL25, WF); die Rechte einer SSE-Verbindung
  gelten vom Verbindungsaufbau an, ein Entzug wirkt erst beim Neuverbinden; je SSE-Verbindung
  eine Valkey-Verbindung (bei vielen Browsern ein gemeinsamer Abonnent je Instanz, UI8); kein
  Filter nach Typ im Strom; die Oberfläche nutzt SSE erst mit UI8.

### PL11 — Anmeldung über OIDC, 2026-10-26

- **Grundlage:** Hauptplan E10, E61, E75, E78 (ergänzt), E88; Architektur §2 (`internal/authn`),
  §6.1 Anmeldung; Inventar G121; E31 bis E33 dieses Plans; deine Entscheidungen vom 2026-10-26
  (ein Anbieter per Umgebung, CEL-Regel → Rolle am Knoten, Konto und Person anlegen).
- **Gebaut:** `internal/authn` (`authn.go`: Begin/Complete mit go-oidc 3.21 und oauth2 0.37,
  Konto/Person/Identität; `rules.go`: Regeln und `ApplyRules`), `internal/authn/authntest`
  (ein kleiner OIDC-Anbieter für Tests: Discovery, Autorisierung, Token mit RS256 und PKCE,
  JWKS), Migration `00012_identity.sql` (`identity`, `login_rule`, `role_assignment.rule_id`),
  `rules.CompileClaims`, Konfiguration `ITSM_PUBLIC_URL` und `ITSM_OIDC_*`, Endpunkte
  `GET /api/v1/auth/providers`, `GET …/oidc/start`, `GET …/oidc/callback`,
  `GET|POST /api/v1/auth/rules`, `PUT|DELETE …/rules/{id}` (`internal/httpapi/oidc.go`),
  Anmeldeseite mit Knopf je Anbieter und Fehlertext aus `?error=`, Compose-Profil `auth` mit
  Keycloak 26.8.1 und Realm-Import `compose/keycloak/itsm-realm.json` (Realm `itsm`, Client
  `itsm`, Gruppen-Mapper, Nutzer `test` in `itsm-admins`, `user` in `itsm-users`).
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL, Valkey und den
  Test-Anbieter, Linter ohne Befund):
  - Prüfweg per HTTP: `providers` nennt den Anbieter ohne Anmeldung; Regel „Mitglieder von
    `itsm-readers` lesen Probe" angelegt (201, Vorgaben `root`, aktiv, Position 100), falsche
    Bedingung und unbekannte Rolle → 422, Regeln ohne Sitzung → 401. Start → 302 zum Anbieter
    mit `state`, `nonce`, `code_challenge_method=S256`, Scopes `openid profile email`; Anbieter
    → 302 zum Rückweg; Rückweg → 302 auf den gewünschten Pfad mit Sitzungscookie. `me` zeigt
    `max_muster` (aus `Max.Muster`), Anzeigename, Rolle `sso_reader`; die Liste Probe ist
    lesbar; die Person trägt Vor- und Nachname und Mailadresse; lokale Anmeldung des Kontos →
    401; die Zuweisung erscheint unter `/api/v1/assignments`.
  - Zweite Anmeldung ohne die Gruppe: dasselbe Konto, keine Rolle mehr, keine zweite Person.
    Ein zweites Subjekt mit gleichem Namen wird `max_muster_2`.
  - Abgelehnt: gesperrtes Konto → `/login?error=disabled`, unbekannter `state` →
    `…error=state`, Fehler vom Anbieter → `…error=refused`.
  - Regeln ändern (aus, Position 5) → 200; löschen → 204, zweites Löschen → 404, die
    regelverwaltete Zuweisung ist mit der Regel verschwunden.
  - Konfiguration: Vorgaben (kein Anbieter, `http://localhost:8080`), Issuer ohne Client-ID
    wird gemeldet, Scopes und öffentliche Adresse werden gelesen.
  - Keycloak: Realm-Datei in einem Wegwerf-Container importiert (26.8.1): Aussteller
    `http://localhost:18091/realms/itsm`, Client `itsm` mit Rückweg `http://localhost:8092/*`
    und Mapper `groups`, Nutzer `test` und `user` mit Mailadressen, `test` in `/itsm-admins`.
  - Web: Lint, Format, 20 Tests grün (Anmeldeseite mit Anbieterliste).
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün: alle Go-Pakete parallel gegen eine Datenbank, Lastgate bestanden (LP1 0,37 ms, LP2 0,61 ms, LZ1 1,49 ms, LZ2 3,15 ms, LZ4 5,61 ms p95, 1828/s), Web-Lint, -Format, -Tests, -Build und Browsertests. Der Lauf deckt auch den Umbau von PL24 ab (Horizont, Wecken über Valkey).
- **Nicht geprüft:** der Klick durch Keycloak selbst (dein Prüfweg: Profil `auth`,
  Anmeldung als `test`, Regel auf `itsm-admins`); Abmeldung beendet nur die Sitzung hier, nicht
  die bei Keycloak (RP-initiated logout, später mit UI); keine Aktualisierung von Name und
  Mailadresse der Person bei späteren Anmeldungen (nur die Identität wird nachgeführt); nur ein
  Anbieter; ADFS und Entra nur per Konfiguration, nicht getestet; kein Verknüpfen eines
  vorhandenen lokalen Kontos mit einer Identität (kommt mit dem LDAP-Sync PL12, der dieselbe
  Person treffen muss); SAML erst Meilenstein 3 (E61). *Das Verknüpfen über die Mailadresse
  kam mit PL12 (E89).*

### PL12 — LDAP-Sync, 2026-10-26

- **Grundlage:** Hauptplan E10, E61, E89; Architektur §2 (`internal/authn`); Inventar G184;
  E34 bis E36 dieses Plans; deine Entscheidungen vom 2026-10-26 (Tabelle mit API, Kennwort aus
  der Umgebung; Person per Verzeichnis-ID, Konto per Mail; Zeitplan-Job plus Befehl).
- **Gebaut:** Migration `00013_directory.sql` (`ldap_directory`, `directory_entry`),
  `internal/authn/ldap.go` (Verzeichnisse, `Sync`, `SyncAll`, Abgleich von Personen, Gruppen,
  Mitgliedschaften, Deaktivieren, Verknüpfen per Mail) mit go-ldap 3.4.15, `ldapjob.go` (Job
  `ldap.sync`, stündlich in `bulk`, Ergebnis im Jobprotokoll), `cmd/itsm ldap list|sync`,
  Endpunkte `GET|POST /api/v1/directories`, `GET|PUT|DELETE …/{name}`, `POST …/{name}:sync`
  (`internal/httpapi/directories.go`), `internal/authn/authntest/ldap.go` (Test-LDAP mit gldap
  0.1.14), Compose-Profil `auth` um `lldap` 0.6.3 (LDAP 3890, Oberfläche 17170) und den
  einmaligen `lldap-bootstrap` mit `compose/lldap` (Gruppen `itsm-admins`, `itsm-users`,
  Personen `ltest`, `luser`), `ITSM_LDAP_PASSWORD_LLDAP` im Compose, README-Zeile.
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL, Valkey, den Test-LDAP
  und den Test-OIDC-Anbieter, Linter ohne Befund):
  - Prüfweg: Verzeichnis per API angelegt (Vorgaben Filter, Knoten `root`, aktiv); `http://`,
    kaputter Filter und unbekannter Knoten → 422, unbekannter Name → 404; `:sync` → 202 mit
    Job `ldap.sync` in `bulk`.
  - Erster Abgleich mit zwei Personen und zwei Gruppen: 2 Personen und 2 Gruppen angelegt, 3
    Mitgliedschaften; Lena mit Vor- und Nachname, Mailadresse, aktiv; `apitest-users` hat zwei
    Mitglieder, `apitest-admins` Lena. Zweiter Lauf ändert nichts (0 angelegt, 0 geändert).
    Umbenennen (`sn`) → 1 geändert, dieselbe Datensatz-ID.
  - Lena verschwindet aus dem Verzeichnis: 1 deaktiviert, 2 Mitgliedschaften entfernt, der
    Datensatz bleibt mit `active=false`, ihr Konto ist gesperrt (Anmeldung 401), die Gruppe hat
    nur noch Lutz. Lena kehrt zurück: 1 reaktiviert, dieselbe ID, aktiv. Das Verzeichnis nennt
    `last_sync_at` und `last_result`. Falsches Bind-Kennwort → Fehler, nichts verändert.
  - OIDC-Anmeldung mit Lutz' Mailadresse: das neue Konto hängt an der synchronisierten
    Person, keine zweite Person (E89).
  - PUT (aus) → 200; DELETE → 204, die Personen bleiben.
  - lldap: Bild 0.6.3 in einem Wegwerf-Container mit den Bootstrap-Daten gestartet;
    `ldapsearch` zeigt `uid=ltest` mit `entryUUID`, `givenName`, `sn`, `mail` und
    `cn=itsm-admins` als `groupOfUniqueNames` mit `member`/`uniqueMember`, genau die Vorgaben.
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün: alle Go-Pakete parallel gegen eine Datenbank (der Verteiler-Test verwirft seither doppelt zugestellte Ereignisse, weil die API-Tests einen zweiten Verteiler laufen lassen), Lastgate bestanden (LP1 0,54 ms, LP2 0,84 ms, LZ1 1,77 ms, LZ2 3,83 ms, LZ4 6,34 ms p95 bei Last 4), Web-Lint, -Format, -Tests, -Build und Browsertests.
- **Nicht geprüft:** der Lauf gegen lldap aus dem Compose-Profil (dein Prüfweg); Active
  Directory nur über die Attributzuordnung, nicht getestet (StartTLS/ldaps nur gegen Code);
  Gruppen in Gruppen (geschachtelte `member`-Einträge auf Gruppen) werden nicht als
  `parent` abgebildet; große Verzeichnisse: der Abgleich arbeitet Datensatz für Datensatz über
  den Record-Dienst (70.000 Personen etwa 2 bis 3 min), die Bulk-API (PL31) kann das später
  beschleunigen; keine Oberfläche für Verzeichnisse; ein gesperrtes Konto wird bei Rückkehr
  der Person nicht automatisch entsperrt.

### PL13 — API-Zugänge, 2026-10-26

- **Grundlage:** Hauptplan E11, E20, E90; Architektur §6.1 (Maschinen mit Client Credentials,
  Ratenbegrenzung in Valkey); Inventar G179, G187, G226; E37, E38 dieses Plans; deine
  Entscheidungen vom 2026-10-26 (undurchsichtiges Token in Valkey, Zugang = Konto ohne
  Kennwort, Einstellungen am Wurzelknoten).
- **Gebaut:** Migration `00014_api_client.sql`, `internal/authn/client.go` (Zugänge, Token
  ausstellen und prüfen, IP-Bereiche, Rate, Widerruf), `internal/cache.IncrExpire`, Token-
  Endpunkt und Bearer-Prüfung in der Middleware (`internal/httpapi/clients.go`, `auth.go`),
  Endpunkte `GET|POST /api/v1/clients`, `GET|PUT|DELETE …/{id}`, `POST …/{id}:rotate`,
  `itsm client create|list|rotate`, Konfiguration `ITSM_TRUST_PROXY`, README-Zeilen.
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL und Valkey, Linter
  ohne Befund):
  - Prüfweg: Zugang angelegt (201 mit Geheimnis und Person), doppelt → 422, unsinniger
    IP-Bereich → 422, Einzelabruf ohne Geheimnis. Rolle an die Person des Zugangs. Token per
    Formular wie mit `curl`: falsches Geheimnis → 401 `invalid_client`, richtiges → Bearer mit
    3600 s. Mit dem Token: `auth/me` nennt das Konto und die Rolle, die Liste Probe kommt, auf
    Personen ohne Rolle kommt nichts; der vierte Aufruf in der Minute (Rate 3) → 429 mit
    `Retry-After` und Problemtyp `rate_limited`; unbekanntes Token → 401.
  - Adressen: Zugang auf `10.0.0.0/8` begrenzt → Token 403 `access_denied` und Aufruf 403;
    `127.0.0.1` und `::1` erlaubt → wieder 200.
  - Einstellung `api.token_ttl` = 120 am Wurzelknoten → ein Zugang ohne eigene Dauer bekommt
    120 s.
  - Sperren → laufendes Token 401, neues Token 401; Drehen → altes Geheimnis 401, neues 200;
    Löschen → Token 401, Zugang 404; ohne `manage_rights` → 403 auf `/api/v1/clients`.
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün (gemeinsamer Lauf für PL13 und PL14): alle Go-Pakete, Lastgate bestanden, Web-Lint, -Format, -Tests, -Build und Browsertests. Ein Vorlauf scheiterte nur daran, dass ich die OpenAPI während des Laufs für PL14 änderte.
- **Nicht geprüft:** kein Token-Introspection- oder Revocation-Endpunkt nach RFC 7662/7009
  (Widerruf nur über Sperren und Löschen); die Rate gilt je Zugang, nicht je Adresse oder für
  Browser-Sitzungen (Anmeldeversuche begrenzt PL28); kein Ablaufdatum des Geheimnisses; keine
  Oberfläche für Zugänge; `X-Forwarded-For` nur erster Eintrag, keine Liste vertrauter Proxys.

### PL14 — JSON-Abfrage, 2026-10-26

- **Grundlage:** Hauptplan E40, E91; Architektur §6.1, §6.3; Inventar G228; E39 dieses Plans.
- **Gebaut:** `internal/records/query.go` (`Query`, `Aggregate`, `Rows`, Abfragebau mit
  Rechten und Papierkorb, Pfade, Cursor, Aggregate), Endpunkt `POST /api/v1/records/{typ}:query`
  (`QueryRecords` in `internal/httpapi/api.go`, auch je Typ in der wirksamen OpenAPI),
  Schemas `Query`, `Rows`.
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Gegen den Testtyp mit sechs Datensätzen (einer im Papierkorb): leere Abfrage → fünf in
    id-Reihenfolge; `and` aus `eq` und `gte` → zwei; `or` aus `contains` (ohne Groß/Klein) und
    `lt` → zwei; `not` mit `in` → zwei; `null: false` → einer; Referenz per `eq` → zwei;
    Pfad `parent.title` mit `starts` in der Bedingung und `parent.title`, `parent.priority` in
    `fields` → die beiden Kinder mit den Werten des Elternteils, andere Felder fehlen.
  - Sortieren `-priority` mit Seiten zu zwei: drei Seiten, Cursor setzt genau fort, letzte
    ohne `next`; Sortieren nach `due_on` stellt den einzigen gesetzten Wert nach vorn
    (Nullwerte zuletzt).
  - Aggregate: je `kind` Anzahl, Summe und Maximum der Priorität (a: 3, 9, 5; b: 2, 6);
    Anzahl mit Pfadbedingung → 2.
  - Abgelehnt mit Problemliste: unbekannter Operator, `rel` (PL15), Pfad über ein
    Textfeld, Text für ein Zahlenfeld, unbekanntes Feld, Sortieren nach Pfad, Summe über Text.
  - HTTP (Abnahmekriterium): die Abfrage aus Architektur §6.3 ohne den `rel`-Teil (Zustände
    `new`/`in_progress`, Priorität ≥ 3, Art Störung, Titel-Präfix; Felder, Sortierung,
    Zählung) liefert genau die zwei erwarteten Datensätze in Reihenfolge mit `total` 2 und
    ohne die nicht gewählte Spalte; dieselbe Abfrage als Person mit Reichweite `own` liefert
    nur ihren eigenen, das Aggregat je Art zählt für sie 1; falscher Operator und `rel` → 422.
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün (gemeinsamer Lauf für PL13 und PL14): alle Go-Pakete, Lastgate bestanden, Web-Lint, -Format, -Tests, -Build und Browsertests. Ein Vorlauf scheiterte nur daran, dass ich die OpenAPI während des Laufs für PL14 änderte.
- **Nicht geprüft:** `rel` über Beziehungen (PL15); kein Volltext (`contains` ist `ILIKE`,
  Volltext kommt mit PL26); LZ2 misst weiter die Liste, eine Messung der JSON-Abfrage auf
  100.000 Zeilen steht aus (H11, Lastszenario anpassen); gespeicherte Suchen und Dashboards
  (UI) nutzen die Sprache noch nicht; Aggregate ohne `having` und ohne Zeitbuckets.

### PL15 — Beziehungen, 2026-10-26

- **Grundlage:** Hauptplan KF2, E92; Architektur §2 (`internal/relations`), §3.1, §6.3;
  Inventar G48; E40 dieses Plans; deine Entscheidungen vom 2026-10-26 (relations/*.yaml,
  Auswirkung über `impact`-Arten, Audit und Ereignis an beiden Enden).
- **Gebaut:** `meta.RelationKind` (Laden aus `relations/`, Prüfung, Speicherung als
  Definition), Migration `00015_relation.sql`, `internal/relations` (Arten, Anlegen, Löschen,
  Liste je Datensatz, Graph, Auswirkung), `records.Check` und `records.Trace`, der
  `rel`-Operator der JSON-Abfrage, Endpunkte `GET /api/v1/relations/kinds`,
  `POST /api/v1/relations`, `GET|DELETE /api/v1/relations/{id}`,
  `GET /api/v1/records/{typ}/{id}/relations|graph|impact` (auch je Typ in der OpenAPI),
  Kernarten `core.depends_on`, `core.related`, `core.duplicate_of`.
- **Belegt, 2026-10-26 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: Kette A → B → C → D (`depends_on`), E `related` an B. Graph von A
    ausgehend mit Tiefe 2 → genau B (1) und C (2), zwei Kanten; von B in beide Richtungen →
    A, C, E auf 1, D auf 2; nur `depends_on` → ohne E. Kreis D → A: Tiefe 10 endet mit vier
    Knoten und vier Kanten (die schließende Kante dabei). Tiefe 11 → abgelehnt.
  - Auswirkung von D: C (1), B (2), A (3), E nicht (nur `related`).
  - Liste je Datensatz mit Art und Richtung; doppeltes Anlegen gibt dieselbe Beziehung;
    unbekannte Art und Selbstbezug → 422.
  - JSON-Abfrage: `rel` mit Bedingung am Ziel (`title = "rel C"`) → B; `direction: in` mit
    `starts` → B.
  - Verlauf von A und B nennt `relate` mit der Art, die Ereignisse `relate`; nach dem
    Löschen `unrelate`, die Beziehung ist 404, B von A aus nicht mehr erreichbar.
  - Rechte: eine Person mit Reichweite `own` kann ihren Datensatz nicht an einen fremden
    hängen (404), ihr Graph zeigt den fremden Nachbarn nicht, ein fremder Startpunkt ist 404.
  - Komplette CI (`docker compose run --rm tools scripts/ci.sh`) am 2026-10-26 grün: alle Go-Pakete parallel, Lastgate bestanden (LP1 0,47 ms, LP2 0,78 ms, LZ1 1,83 ms, LZ2 4,38 ms, LZ4 7,57 ms p95 bei Last 4 bis 5), Web-Lint, -Format, -Tests, -Build und Browsertests.
- **Nicht geprüft:** keine Oberfläche für Beziehungen und Graph (UI-Plan); Löschen eines
  Datensatzes räumt seine Beziehungen nicht auf (endgültiges Löschen lässt Beziehungen auf
  eine fehlende ID zurück, die der Graph als unlesbar ausblendet; Aufräumen beim Purge
  folgt); keine Kardinalitäten je Art; Sichtbarkeit je Knoten per Einzelabruf (E40); keine
  Messung mit großen Graphen (H11).
- **Nachtrag SD18 (2026-11-10, Hauptplan E173, SD-Plan N16):** Eine Beziehungsart kann mit
  `update: from` sagen, dass Anlegen und Lösen das Recht zum Ändern nur am Ausgang brauchen; am
  Ziel genügt Lesen (ein Bearbeiter verknüpft einen Artikel mit seiner Störung, ohne den
  Artikel ändern zu dürfen). Ohne Angabe bleibt es bei beiden Enden.

### PL16 — Auswahllisten, 2026-10-27

- **Grundlage:** Hauptplan E93; Architektur B3 (Typ mit Merkmal, keine Tabelle je Liste),
  §3.1; Inventar G19, G138 bis G158; E41 dieses Plans; deine Entscheidungen vom 2026-10-27
  (eine Kerntabelle `list_entry`, Referenz auf den Listentyp, `parent` plus ltree-Pfad,
  Lesen implizit und Pflege per Rolle).
- **Gebaut:** `meta.TraitList`, `ListSpec` und `ListFields` (Felder vom Merkmal, Prüfung der
  Einträge), Migration `00016_list_entry.sql`, `schema.ListTable`, `internal/records/lists.go`
  (Listenbedingung, Baumpflege `path`/`full_name` per rekursivem UPDATE, Kreisprüfung,
  `under` in Liste und JSON-Abfrage, `SeedLists`), `authz.Access.ReadAll`, Ausgabe von
  `itsm package install` mit Listen und angelegten Einträgen, Probe-Paket mit `probe.color`
  (flach, drei Einträge) und `probe.place` (Baum mit Koordinaten, drei Einträge) und zwei
  Referenzfeldern `color` und `place` an `probe.probe`.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: ein Testpaket mit der flachen Liste `tag`, dem Baum `region` mit
    Koordinaten und einem Typ mit zwei Referenzen legt keine Tabelle `rec_liststest_tag`
    oder `rec_liststest_region` an; das Referenzfeld `tag` lehnt die ID eines
    `region`-Eintrags mit `reference` ab und nimmt einen `tag`-Eintrag; über HTTP lehnt
    `probe.probe.color` die ID eines Orts mit 422 ab.
  - Startwerte: sechs Einträge beim ersten Aufruf, null beim zweiten; Schlüssel eindeutig je
    Liste (`a` zweimal in `tag` → 409, `a` in `region` erlaubt); ein Eintrag ist über die
    andere Liste nicht erreichbar (404).
  - Baum: `full_name` „Europa > Deutschland > NRW"; `filter[under]=eu` gibt zwei Einträge,
    die JSON-Abfrage `{"field": "parent", "under": eu}` dieselben mit `full_name`; `eu` unter
    `nrw` hängen → 422 `cycle`; `de` unter `us` verschieben und `us` umbenennen gibt
    `Amerika > Deutschland > NRW` für den Enkel; ein Pfad `region.name` durch die Referenz
    trifft den Eintrag. Über HTTP: Hauptgebäude unter Deutschland hängen kürzt den Namen,
    Deutschland unter das Hauptgebäude → 422.
  - Rechte: eine Person ohne jede Rolle liest `probe.color` (drei Einträge) und einen Ort
    mit `full_name`, sieht in `/api/v1/types` die Listen, aber nicht `probe.probe`, bekommt
    für `probe.probe` eine leere Liste und beim Anlegen eines Eintrags 403; die
    Administration legt an.
  - Meta: Felder und `title_field` einer Liste kommen vom Merkmal (`name`, `key`, …, `parent`,
    `lat`, `lon`, `alt`), eigene Felder, unbekannte Merkmale, `list` ohne Merkmal, Eltern
    ohne Baum oder mit unbekanntem Schlüssel und Koordinaten ohne Kennzeichen werden mit
    Ort und Grund abgelehnt.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27: Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,39 ms, LP2 0,60 ms, LZ1 1,82 ms, LZ2 3,58 ms, LZ4 6,29 ms p95; LP1/LP2 gegen die Basislinie -9 %/-7 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Oberfläche für Listen (Auswahlfeld statt UUID-Eingabe,
  Baumansicht; UI-Plan); keine Zusatzfelder je Liste (jsonb, wenn ein Paket sie braucht);
  Übersetzung der Einträge (KF19); Startwerte aktualisieren vorhandene Einträge nicht;
  `search` wird für Listen noch nicht gefüllt (wie bei allen Typen); keine Messung mit
  großen Listen.

### PL17 — Merkmale, 2026-10-27

- **Grundlage:** Hauptplan E94; Architektur B2, §4 (`traits: [...]`); Inventar G18; E42
  dieses Plans; deine Entscheidungen vom 2026-10-27 (ohne `history` kein Verlauf;
  Mechanismus plus `history`, der Rest deklarierbar; Kern-Merkmale in Go, Paket-Merkmale als
  Feldbündel).
- **Gebaut:** `internal/meta/traits.go` (Register, `Package.Traits`, `expandTraits`,
  `validateTraits`, Definition `kind=trait`, `meta.Traits`), `Field.Trait`, `audit` und
  `History` prüfen das Merkmal, `GET /api/v1/traits`, Unterressourcen `/documents`,
  `/notes`, `/reservations` (501 mit Hinweis auf die Aufgabe, 404 ohne Merkmal), OpenAPI je
  Typ nur mit den Unterressourcen seiner Merkmale, `itsm package install` nennt Merkmale und
  Herkunft der Felder; Probe-Paket mit `traits/tagged.yaml` (`tag_no`, `tagged_on`) und
  `probe.probe` mit `traits: [history, documents, tagged]`; `core.person`, `core.group`,
  `core.group_member` mit `history`.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: ein Testpaket mit `kept` (`history`, `tagged`) und `plain` (ohne
    Merkmale): nach Anlegen, Ändern und Papierkorb hat `kept` vier Verlaufszeilen über die
    API und fünf in `audit`, `plain` null Zeilen und `History` → nicht gefunden; beide
    haben drei Ereignisse. Über HTTP: `probe.probe` mit `tag_no` angelegt, `/history` nennt
    das Feld; `/documents` → 501 mit „PL18", `/notes` → 404 „no trait notes"; die OpenAPI
    von `probe.probe` hat `/documents` und `/history`, nicht `/notes`; ein Typ `apitest.plain`
    ohne Merkmale hat weder `/history` (404) noch den Pfad in der OpenAPI.
  - Meta: Felder eines Paketmerkmals hängen am Typ mit `trait: tagged`, eigene Felder ohne;
    `SubResources` von `[history, documents]` sind `history, documents`; eine Liste hat
    `history` immer. Abgelehnt: Feldkollision mit einem Merkmal, Paketmerkmal an einer Liste,
    Paketmerkmal mit Kernnamen, Systemspalte und unbekannter Feldtyp in einem Merkmal, leeres
    Merkmal, unbekanntes und doppeltes Merkmal am Typ.
  - `GET /api/v1/traits` nennt `history` und `tagged` (Paket `probe`).
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27: Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,52 ms, LP2 0,86 ms, LZ1 2,26 ms, LZ2 4,87 ms, LZ4 7,47 ms p95; LP1/LP2 gegen die Basislinie -12 %/-14 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** `documents`, `notes`, `reservable` haben noch keinen Inhalt (PL18,
  PL35, Reservierungspaket); keine Merkmale über Paketgrenzen; keine Oberfläche, die
  Merkmale zeigt (das Formular zeigt die Felder wie alle anderen); Merkmale mit Verhalten
  aus Paketen (Go-Erweiterungspunkt) sind nicht vorgesehen.
- **Nachtrag SD18 (2026-11-10, Hauptplan E173, SD-Plan N14):** Neues Kern-Merkmal `rating`:
  Ja oder Nein je Person und Datensatz in der Tabelle `rating` (Migration 43), änderbar,
  `GET` und `PUT /api/v1/records/{typ}/{id}/rating`; der Datensatz behält seine Version.

### PL18 — Dateien, 2026-10-27

- **Grundlage:** Hauptplan E22, E95; Architektur §2 (`internal/files`), §5 (`/api/v1/files`);
  Inventar G90, G245; E43 dieses Plans; deine Entscheidungen vom 2026-10-27 (erst hochladen,
  dann verknüpfen; je Datei ein Objekt mit SHA-256; Größe und Endungen, Virenscan als
  Schnittstelle; Lesen über die verknüpften Datensätze).
- **Gebaut:** Migration `00017_file.sql` (`file`, `attachment`, Verlaufsaktionen `attach`,
  `detach`), Abfragen `db/queries/file.sql`, `internal/files` (`Store` mit `FS`, `Scanner`,
  Dienst mit Hochladen, Lesen, Anhängen, Lösen, Liste, Aufräumen; Job `files.cleanup`
  stündlich in `bulk`), Endpunkte `POST /api/v1/files`, `GET /api/v1/files/{id}` (mit Range,
  `ITSM-SHA256`, `Content-Disposition` nach RFC 6266/5987), `GET|POST
  /api/v1/records/{typ}/{id}/documents`, `DELETE …/documents/{fileId}`; OpenAPI je Typ mit
  dem Merkmal; Dockerfile und Compose mit Volume `files` unter `/data/files`.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: 540 KB Logdatei per HTTP hochgeladen, an einen Probe-Datensatz
    gehängt, heruntergeladen: SHA-256 der Antwort gleich dem des Originals und gleich
    `ITSM-SHA256`; `Range: bytes=0-3` → 206 mit den ersten vier Bytes. Im Dienst dasselbe mit
    einer PDF (Inhaltstyp `application/pdf` erkannt).
  - Abgebrochene Uploads: ein Strom, der nach 50 KB abbricht, hinterlässt weder Zeile noch
    Datei; ein nachgestellter Absturz (Zeile `uploading` und `.part`-Datei, zwei Stunden alt)
    ist nach `Cleanup` weg; eine vorläufige Datei von vor 25 Stunden ist weg, eine frische
    und eine angehängte (auch 25 Stunden alt) bleiben.
  - Grenzen: `.EXE` → 422 `blocked`, ohne Namen → 422, 1 MiB + 1 Byte bei 1 MiB Grenze →
    `ErrTooLarge`; keiner davon hinterlässt Zeile oder Datei. Der Name kommt auch aus
    `Content-Disposition`.
  - Rechte: eine Person ohne Rechte am Probe-Typ bekommt für die angehängte Datei 404, darf
    die eigene vorläufige lesen, aber nicht an einen fremden Datensatz hängen (404); die
    vorläufige Datei einer anderen Person ist auch für die Administration 404.
  - Verlauf: `attach` und `detach` am Datensatz; zweimal anhängen ändert nichts, zweimal
    lösen → 404; ein Typ ohne `documents` → 404.
  - Speicher: halbe Ströme bleiben unsichtbar und ohne `.part`-Rest, Schlüssel mit `..` oder
    absolutem Pfad werden abgelehnt.
  - Image: mit `docker build` gebaut, ein frisches benanntes Volume unter `/data/files` gehört
    65532:65532 und ist für den Nutzer beschreibbar.
  - Nebenbei: Der Jobs-Test `TestBulkDoesNotDelayInteractive` fiel im ersten CI-Lauf unter
    Last (paralleler Image-Bau), weil sein Helfer Abschlussereignisse anderer Jobs verwarf;
    der Helfer merkt sie sich jetzt (`watch`), dreimal hintereinander grün.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 im zweiten Lauf: Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,42 ms, LP2 0,60 ms, LZ1 1,83 ms, LZ2 3,60 ms, LZ4 5,52 ms p95; LP1/LP2 gegen die Basislinie -8 %/-2 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** S3-Speicher (V23 offen); kein Virenscanner angeschlossen; verwaiste
  Dateien nach dem Lösen oder nach dem endgültigen Löschen eines Datensatzes räumt erst PL36
  auf; keine Oberfläche zum Hochladen (UI3, UI7); keine Messung mit großen Dateien oder vielen
  parallelen Uploads; Anhängen und Verlaufszeile laufen in zwei Transaktionen.

### PL19 — Versionen, 2026-10-27

- **Grundlage:** Hauptplan KF17, E96; Architektur §2 (`internal/records`: Versionen);
  Inventar G107; E44 dieses Plans; deine Entscheidungen vom 2026-10-27 (aus dem Verlauf
  rekonstruieren; Zurücksetzen als normale Änderung; Teil von `history`).
- **Gebaut:** `internal/records/versions.go` (`Versions`, `Version`, `Compare`,
  `RestoreVersion`), Endpunkte `GET …/{id}/versions`, `GET …/{id}/versions/{n}`,
  `POST …/{id}/versions/{n}:restore`, `GET …/{id}/compare?from=&to=`; in der OpenAPI je Typ
  nur bei Typen mit `history`; Schemas `VersionInfo`, `Comparison`.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: drei Änderungen (Titel und Priorität, Art geleert, Titel); der
    Vergleich von Version 1 und 3 zeigt genau `title` (v1 → v2), `priority` (3 → 5) und
    `kind` (a → null), nicht `due_on` und nicht die vierte Änderung; Vergleich 3 mit 3 ist
    leer.
  - Liste: vier Versionen, die erste mit `create`, die zweite mit `title, priority`, die
    dritte mit `kind`. Stand 1 hat Titel, Art, Vorgabe-Priorität und Datum von damals;
    Stand 0 und 5 → 404.
  - Zurücksetzen: mit veralteter Version → Konflikt; mit der aktuellen schreibt es Version 5
    mit den Werten von Stand 1, der Vergleich 1 mit 5 ist danach leer. Über HTTP: ohne
    `If-Match` → 428, mit → 200, `ETag: "5"`, Status bleibt `in_progress`.
  - Status: ein Übergang erscheint als Version mit `transition`, der Vergleich nennt
    `state` (new → in_progress).
  - Abgeschnittener Verlauf: nach dem Löschen der Zeilen vor Version 3 ist Stand 1 „no
    longer in the history" (404), Stand 3 stimmt weiter.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27: Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,53 ms, LP2 0,71 ms, LZ1 1,98 ms, LZ2 4,02 ms, LZ4 5,98 ms p95; LP1/LP2 gegen die Basislinie -3 %/+4 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Oberfläche (Vergleichsansicht kommt mit UI3); Freigabe von
  Wissensartikeln (Entwurf und veröffentlichter Stand) und CMDB-Baselines über mehrere
  Datensätze sind Sache der Pakete (SD, CM); keine Messung bei Datensätzen mit tausenden
  Verlaufszeilen; Zurücksetzen von Referenzen auf inzwischen gelöschte Datensätze scheitert
  mit 422 statt sie auszulassen.

### PL20 — Kosten und Zeiten, 2026-10-27

- **Grundlage:** Hauptplan KF18, E97; Architektur B1 und §2 (`internal/records`: Kosten und
  Zeiten als Untersätze); Inventar G46 (Kosten am Vorgang: Zeit, fest, Material), G80
  (Bearbeitungszeit am Ticket); E45 dieses Plans; deine Entscheidungen vom 2026-10-27
  (Kerntabelle `cost`; Art, Dauer, Satz, Betrag mit einer Währung je Installation; Summen
  gespeichert ohne Versionssprung; Rechte wie am Datensatz).
- **Gebaut:** Migration `00018_cost.sql` (Tabelle `cost`, Verlaufsaktionen `cost_add`,
  `cost_change`, `cost_remove`), Abfragen `db/queries/cost.sql`, Kern-Merkmal `costs` mit den
  Feldern `time_spent` und `cost_total` (`Field.Computed`), `internal/records/costs.go`
  (`Costs`, `AddCost`, `UpdateCost`, `RemoveCost`), Endpunkte `GET`/`POST …/{id}/costs` und
  `PATCH`/`DELETE …/{id}/costs/{costId}`, Einstellungen `costs.currency` und
  `costs.hourly_rate`; Purge löscht die Einträge mit; der Probe-Typ hat jetzt `costs`. Im
  Formular sind berechnete Felder schreibgeschützt und werden nicht gesendet
  (`web/src/render/fields.ts`, `Form.tsx`).
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: drei Einträge (90 Minuten zu 60 €, 30 Minuten zum Satz der Einstellung
    80 €, Material 25,50 €) → am Datensatz 120 Minuten und 155,50 €, Version bleibt 1; nach
    Entfernen des ersten Eintrags 30 Minuten und 65,50 €; nach Ändern von Material und Minuten
    45 Minuten und 90 €.
  - Bearbeiten des Datensatzes mit `If-Match: "1"` nach allen Buchungen geht durch.
  - 20 parallele Buchungen an einem Datensatz → Summen genau 60 Minuten und 20 € (mit `-race`).
  - Prüfungen: Art fehlt oder unbekannt, Zeit ohne Minuten, halbe Minuten, Betrag an Zeit,
    Minuten oder Satz an fest/Material, negativer Betrag, falsches Datum, unbekannte Person,
    unbekanntes Feld, Art ändern, Summe schreiben (anlegen und ändern) → je 422 mit Feld und
    Code.
  - Verlauf: Version 1 zeigt `create, update, cost_add, cost_remove, cost_change`, geändert nur
    `title`; ein Stand hat keine Summen.
  - Papierkorb: Buchen → 404; Purge → keine Einträge mehr in `cost`.
  - HTTP: Buchen 201 mit `ETag`, Liste mit Währung EUR und Summen, Ändern ohne `If-Match` 428,
    mit alter Version 409, mit aktueller 200 und `ETag: "2"`, Entfernen 204, Filter
    `filter[cost_total]=62.5`, `core.person` hat keine Kosten (404), OpenAPI mit den Pfaden und
    `cost_total` als `readOnly`.
  - Oberfläche: Vitest prüft, dass berechnete Felder nie im gesendeten Datensatz stehen; der
    Playwright-Test „anlegen und bearbeiten“ lief erst rot (das Formular schickte die Summen,
    422) und nach der Korrektur grün.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (dritter Lauf; der erste scheiterte an gosec G115, der zweite am Playwright-Test des Formulars): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,35 ms, LP2 0,54 ms, LZ1 1,88 ms, LZ2 3,80 ms, LZ4 5,21 ms p95; LP1/LP2 gegen die Basislinie -8 %/-8 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Zeiterfassung in der Oberfläche, das Formular zeigt nur die Summen
  (Buchen kommt mit UI3); keine
  Budgets, Kostenstellen und Verrechnung (IP11, Paket); keine Zeiterfassung aus Aufgaben (G41
  kommt mit den Untervorgängen in WF); Gutschriften mit negativem Betrag sind nicht möglich;
  wer die Einstellung `costs.hourly_rate` ändert, ändert keine alten Einträge (der Satz wird
  beim Buchen festgehalten); keine Messung bei Datensätzen mit tausenden Einträgen.

### PL21 — Mehrsprachige Inhalte, 2026-10-27

- **Grundlage:** Hauptplan KF19, E42, E98; Architektur §2 (`internal/meta`: Übersetzungen);
  Inventar G237 (Übersetzung von Inhalten); E46 dieses Plans; deine Entscheidungen vom
  2026-10-27 (Grundspalte Deutsch plus Tabelle; Textfelder mit `translatable`, Listen immer;
  `Accept-Language` und `Content-Language`; Deutsch und Englisch fest).
- **Gebaut:** Migration `00019_translation.sql`, Abfragen `db/queries/translation.sql`,
  `Field.Translatable` mit Prüfung (nur Text), Name und Beschreibung der Auswahllisten
  übersetzbar, `internal/records/translations.go` (`splitTranslations`, `Localize`,
  `WithLang`), Middleware `withLanguage` in `internal/httpapi/language.go` (gewichtete Wahl
  aus `Accept-Language`, ersetzt die alte Wahl der Fehlertitel), Parameter `translations` an
  Liste, JSON-Abfrage und Einzelabruf, übersetzbare Felder in der OpenAPI als `oneOf` aus Text
  und Objekt je Sprache; der Probe-Titel ist übersetzbar; Purge löscht die Übersetzungen mit.
  Der lokale Port der Anwendung ist seit heute 8092 statt 8090 (Compose, Keycloak-Realm,
  README, Vite, Pläne).
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: ein Datensatz mit `{"de": "Drucker", "en": "Printer"}` liefert mit
    `Accept-Language: en-GB,en;q=0.9` „Printer“ und `Content-Language: en`, ohne Header
    „Drucker“; ein Datensatz ohne Übersetzung liefert auf Englisch den deutschen Wert, `fr`
    liest Deutsch.
  - `?translations=all` → `{"de": "Drucker", "en": "Printer"}`; Liste und JSON-Abfrage
    übersetzen ebenso.
  - Schreiben mit `Content-Language: en` ändert nur Englisch (Version 2, Deutsch bleibt);
    derselbe Wert noch einmal ist keine Änderung; `{"en": null}` entfernt die Übersetzung.
  - Verlauf: Version 1 zeigt `title, code, title.en`, Version 2 `title.en`.
  - Prüfungen: Sprache `fr` im Objekt und als `Content-Language` → `language`; zu lange oder
    nicht textuelle Übersetzung → `title.en` mit `max_length` bzw. `type`; nur Englisch bei
    einem Pflichtfeld → `title` `required`; Objekt an einem nicht übersetzbaren Feld → `type`;
    `translatable` an einer Zahl → Definitionsfehler.
  - Auswahlliste: der Eintrag „red“ von `probe.color` bekommt `{"en": "Red"}` und liefert es
    auf Englisch, auf Deutsch nicht.
  - Purge → keine Zeilen mehr in `translation`; `pickLanguage` mit Gewichten, `*`, `q=0`,
    Großschreibung.
  - Leistung: der erste Messlauf zeigte LZ1 bei 2,60 ms p95 statt 1,88 ms, weil `Localize`
    auch bei Deutsch die Typdefinition lud; jetzt endet es bei Deutsch sofort.
  - Zwei Tests anderer Pakete waren unter der höheren Last paralleler Tests rot und sind jetzt
    robust: `TestConcurrentWritersAndHorizon` liest den Strom, bis die 20 Ereignisse sichtbar
    sind (höchstens 10 s, Horizont E87), `TestEventRetention` liest 1000 statt 10 Zeilen.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (fünfter Lauf; davor ein ungenutzter Helfer, die zwei Lasttests und die LZ1-Messung): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,37 ms, LP2 0,57 ms, LZ1 1,89 ms, LZ2 3,99 ms, LZ4 5,29 ms p95; LP1/LP2 gegen die Basislinie -8 %/-8 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Bearbeitung je Sprache in der Oberfläche (kommt mit UI3, bis dahin
  arbeitet sie auf Deutsch); `full_name` der Baumlisten bleibt deutsch; Versionen und
  Vergleich zeigen den deutschen Wert, Zurücksetzen stellt keine Übersetzungen wieder her;
  Suche und Filter finden nur den deutschen Wert; die Typen in `GET /api/v1/types` liefern
  weiter alle Beschriftungen, die Oberfläche wählt selbst.

### PL22 — Berechnete Felder, 2026-10-27

- **Grundlage:** Hauptplan KF6, E45, E99; Architektur §2 (`internal/records`: berechnete Felder,
  `internal/rules`: CEL); E47 dieses Plans; deine Entscheidungen vom 2026-10-27 (Feldnamen
  direkt und voll typgeprüft; leeres Ergebnis bei leerem Feld; Job rechnet den Bestand neu;
  Formeln über Beziehungen als eigene Aufgabe PL40).
- **Gebaut:** `Field.Formula`, `internal/rules/formula.go` (`CompileFormula`, `Eval`),
  `internal/meta/formula.go` (`CompileFormulas`, Prüfung beim Installieren),
  `internal/records/formulas.go` (Rechnen beim Anlegen und Ändern, Job `records.recompute`,
  `Recompute`), Worker im Server, Einreihen in `itsm package install` mit Ausgabe, berechnete
  Felder in der OpenAPI als `readOnly` mit der Formel, im Formular schreibgeschützt. Der
  Probe-Typ hat jetzt `impact`, `urgency` und `itil_priority` nach der ITIL-Matrix.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: `priority = impact * urgency` ist beim Anlegen 6 (2 × 3) und nach Ändern
    der Dringlichkeit 2; die daraus berechnete Auswahl `level` folgt (high, low), das Datum
    `due_on = opened_on + 72h` ist der 30.10.; leeres `impact` macht alle drei leer.
  - Typfehler beim Installieren: Tippfehler im Feldnamen, Text als Ergebnis eines Zahlenfelds,
    Zyklus `a → b → a`, Selbstbezug, Pflichtfeld mit Formel, berechnete Referenz und Formel
    über `cost_total` → je abgelehnt mit Ort und Grund.
  - Verlauf: die Version mit neuer Dringlichkeit nennt `urgency, priority, level`.
  - Schreiben eines berechneten Felds → 422 `read_only`; Ergebnis „none" außerhalb der
    Optionen → 422 `formula`.
  - Neuberechnung nach geänderter Formel (`impact + urgency`): zwei von drei Datensätzen
    geändert, Werte 3, 4, 5, Version bleibt 1, keine neuen Verlaufszeilen, der Datensatz ohne
    Eingaben bleibt leer.
  - HTTP mit dem Probe-Typ: hoch/gering → 3, hoch/hoch → 1, Dringlichkeit leer → leer,
    Schreiben → 422, Typ mit `formula`, OpenAPI `readOnly`; Vitest: das Formular sendet keine
    berechneten Felder. Der Vitest fand dabei einen echten Fehler: in Zod 4 ist `z.unknown()`
    ein Pflichtschlüssel, ein leeres berechnetes Feld hätte das Formular blockiert; jetzt
    `z.unknown().optional()`.
  - Von Hand gegen die Compose-Instanz: `itsm package install` mit geänderter Matrix (gering
    → 9) meldet „werden neu berechnet (Job 545)", der Datensatz hoch/gering steht danach auf 9
    bei Version 1; das Original zurück (Job 546) setzt ihn auf 3; beide Jobs `completed`.
  - Playwright: der Selektor `getByLabel("Priorität")` traf auch „Priorität nach Matrix“ und
    ist jetzt exakt.
  - Nebenbei: `TestEventRetention` (PL24) legt seine Ereignisse jetzt in einer nie bestätigten
    Transaktion an, weil zwei andere Tests zum Simulieren der Aufbewahrung alle Ereignisse bis
    zu einer Position löschen und dabei seine trafen; der Worker hat dafür eine austauschbare
    Verbindung. `TestEventsAPI` (PL24) liest den Strom, bis seine zehn Ereignisse sichtbar
    sind (höchstens 10 s, mit `limit=1000`), denn jede offene Schreibtransaktion eines anderen
    Testpakets hält den Strom kurz zurück (Horizont E87). Die Lizenzprüfung nimmt `compose.override.yaml` aus (lokale Einstellungen eines
    Rechners, per `.git/info/exclude` nie in Git).
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (siebter Lauf; davor Lizenzkopf der lokalen Override-Datei, ein Linter-Hinweis, die Ereignis-Tests unter Last, der Zod-Fehler und der Playwright-Selektor): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,35 ms, LP2 0,54 ms, LZ1 2,02 ms, LZ2 3,81 ms, LZ4 5,31 ms p95; LP1/LP2 gegen die Basislinie -12 %/-8 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** Formeln über Beziehungen (PL40); Systemfelder wie `state` oder
  `created_at` sind in Formeln nicht verfügbar; der Job ist nicht unter Last mit großen Tabellen
  gemessen;
  eine Formel, die eine Übersetzung brauchte, sieht nur den deutschen Wert.

### PL25 — Webhooks, 2026-10-27

- **Grundlage:** Hauptplan KF13, E39, E100; Architektur §2 und §7; Inventar G165; E48 dieses
  Plans; deine Entscheidungen vom 2026-10-27 (River-Job je Zustellung; Standard Webhooks;
  Ereignis wie im Strom mit den Rechten des Besitzers; 8 Versuche bis etwa 8 h, nur https).
- **Gebaut:** Migration `00020_webhook.sql`, Abfragen `db/queries/webhook.sql`,
  `internal/webhooks` (Abonnements, `Handle` am Verteiler, Job `webhooks.deliver` mit eigener
  Rückstellung, `Sign` und `Verify` nach Standard Webhooks), Endpunkte `/api/v1/webhooks`
  (Liste, Anlegen, Lesen, Ändern, Löschen, `:rotate`, `/deliveries`), Verdrahtung im Server.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium mit echtem River: ein Testempfänger antwortet zweimal 500, die Zustellung
    ist beim dritten Versuch `completed` (Rückstellung im Test 10 ms); die Signatur der
    letzten Anfrage prüft `Verify` mit dem Geheimnis, ein anderes Geheimnis nicht;
    `webhook-id` bleibt über die Versuche gleich (`evt_<Position>`); die Nutzlast ist das
    Ereignis.
  - Doppelt verteiltes Ereignis → eine Zustellung.
  - Filter und Rechte: ein Abonnement nur für `update` bekommt `create` nicht; ein Besitzer
    ohne Rechte bekommt nichts, seine Position rückt trotzdem vor; ein gelöschtes Abonnement
    stellt nicht zu.
  - Ziele: https ja, http nur für erlaubte Hosts, ftp, Benutzer in der URL und relative URLs
    nein; unbekannter Typ und fehlender Name → 422. Rückstellung 1 min bis 4 h, Summe 7 h 51
    min.
  - HTTP: Anlegen 201 mit `whsec_…`, Lesen und Liste ohne Geheimnis, http-Ziel 422, Ändern,
    Rotieren mit neuem Geheimnis, ein Probe-Datensatz ergibt eine wartende Zustellung mit 8
    Versuchen, ohne Recht 403, Löschen 204, danach 404.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (zweiter Lauf; der erste scheiterte an gofmt in einer Testdatei): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,36 ms, LP2 0,58 ms, LZ1 1,96 ms, LZ2 3,68 ms, LZ4 5,59 ms p95; LP1/LP2 gegen die Basislinie -8 %/-3 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Oberfläche für Abonnements; kein Test gegen einen echten
  https-Empfänger mit Zertifikat; keine Last mit vielen Abonnements (je Ereignis eine Abfrage
  und je passendem Abonnement eine Transaktion); das Geheimnis ist nicht verschlüsselt
  gespeichert; ein dauerhaft fehlschlagender Empfänger wird nicht automatisch abgeschaltet.

### PL26 — Volltextsuche, 2026-10-27

- **Grundlage:** Hauptplan E21, E101, LZ3; Architektur §11; E49 dieses Plans; deine
  Entscheidungen vom 2026-10-27 (`records` schreibt die Spalte; alle lesbaren Typen,
  eingrenzbar; Trigram auf dem Titelfeld zusätzlich zum Volltext; Websyntax plus Präfix am
  letzten Wort).
- **Gebaut:** `meta.Type.SearchFields`, Indizes in `schema.Apply` und Migration
  `00021_search.sql`, `internal/records/search.go` (Ausdruck, Pflege beim Schreiben,
  `RecomputeSearch`, `SearchUnfilled`, `Search`), Job `records.recompute` mit `search`,
  Einreihen in `itsm package install`, `internal/search` (Schnittstelle `Engine`, PostgreSQL),
  Endpunkt `GET /api/v1/search`, Lastszenario LZ3.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: „Drucker" findet „Störung an Druckern im 3. OG", den Titel „Drucker" und
    „Monitor flackert" (Treffer im Text, als letzter, weil der Titel mehr wiegt), nicht
    „Tastatur"; „Drukcer" findet „Drucker" per Trigram; der Ausschnitt markiert mit `<mark>`.
  - Präfix: „Druck" findet „Drucker" und „Druckern"; „printer" findet den Datensatz mit
    englischem Titel; „Drucker -Toner" schließt aus; „"Toner leer"" findet die Phrase; ein
    nicht durchsuchbares Feld wird nicht gefunden; über 200 Zeichen → 422.
  - Bestand: nach `search = NULL` meldet `SearchUnfilled` ja, der Titel bleibt per Trigram
    findbar, `RecomputeSearch` füllt vier Datensätze, danach trifft der Text wieder.
  - HTTP: Treffer mit Typ, ID und markiertem Ausschnitt; eingegrenzt auf `core.person` leer;
    unbekannter Typ 404; ohne `q` 400; eine Person ohne Rechte findet den Datensatz nicht.
  - Nebenbei: `TestDeliveryRetriesUntilAccepted` (PL25) lief in der CI einmal rot, weil der
    Worker der laufenden Compose-Instanz auf derselben Datenbank die Test-Zustellung abholte
    und mangels Rechten des Testkontos ohne Versand beendete. Der Webhook-Dienst hat jetzt eine
    einstellbare Warteschlange (`Queue`, Vorgabe bulk), der Test nutzt eine eigene.
  - Leistung: ein Lauf mit zweitem `UPDATE` für die Suchspalte beim Anlegen zeigte LZ4 bei 8,25
    ms p95 (vorher 5,59 ms); jetzt rechnet das `INSERT` die Spalte aus den Werten selbst. Im
    letzten Lauf war der Rechner insgesamt langsamer (Basislinie LP1 0,49 statt 0,39 ms, LZ1
    ohne Bezug zu PL26 +30 %); LZ4 liegt mit 7,27 ms im selben Verhältnis, gegen die
    Basislinie ist der neue Stand schneller.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (dritter Lauf; davor der Webhook-Test gegen die laufende Instanz und die Messung mit zweitem `UPDATE`): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,43 ms, LP2 0,70 ms, LZ1 2,55 ms, LZ2 4,68 ms, LZ3 7,73 ms, LZ4 7,27 ms p95; LP1/LP2 gegen die Basislinie -12 %/-9 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Oberfläche (Suchfeld kommt mit UI); keine Messung mit dem
  Datenbestand in Prod-Größe (kommt mit PL31 bis PL33); der Titel im Treffer ist deutsch, auch
  bei englischem `Accept-Language`; Gewichtung und Ähnlichkeitsschwelle sind nicht
  einstellbar.
- **Nachtrag SD18 (2026-11-10, Hauptplan E173, SD-Plan N13):** Die Suche kennt „jedes Wort
  zählt“ (`SearchQuery.Any`): die Wörter eines langen Texts mit ODER statt UND, bis 2.000
  Zeichen, das letzte Wort auch als Wortanfang. Sie dient den Vorschlägen
  (`POST /api/v1/records/{typ}:suggest`), die ein Typ mit `suggest` für andere Typen anbietet;
  `/api/v1/search` bleibt bei allen Wörtern.

### PL27 — Ereignisprotokoll, 2026-10-27

- **Grundlage:** Hauptplan E102; Inventar G128; E50 dieses Plans; deine Entscheidungen vom
  2026-10-27 (Anmeldung, Konten, Rechte, Einstellungen; Zeit, Konto, Quelle, Ergebnis,
  Details; 1 Jahr als Einstellung; zwei Endpunkte auf einer Tabelle).
- **Gebaut:** Migration `00022_security_event.sql`, Abfragen `db/queries/security_event.sql`,
  `internal/seclog` (Aufzeichnen, Abfragen, Aufbewahrung, Job `seclog.retention` um 2:45),
  `auth.LoginError`, Aufzeichnung in den Handlern für Anmeldung, OIDC, Token, Abmeldung,
  Kontext, Kennwort, Rollen, Berechtigungen, Zuweisungen, Login-Regeln, API-Zugänge,
  Verzeichnisse, Knoten und Einstellungen sowie in `itsm user` und `itsm client`, Endpunkte
  `/api/v1/audit/auth` und `/api/v1/audit/changes`.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL, Linter ohne Befund):
  - Abnahmekriterium: drei fehlgeschlagene Anmeldungen (zweimal falsches Kennwort, einmal
    unbekanntes Konto) stehen in `/api/v1/audit/auth` mit Zeit, Quelle `127.0.0.1`,
    User-Agent und Grund `wrong_password` bzw. `unknown_account`, das bekannte Konto mit ID.
  - Blättern mit `limit=1` und `before`; erfolgreiche Anmeldung und Abmeldung erscheinen;
    eine Person ohne Recht liest das Protokoll nicht (403); `ip=nonsense` → 400.
  - Änderungen: zweimal dieselbe Berechtigung gesetzt → der zweite Eintrag hat vorher `own`,
    nachher `all`; Rolle gelöscht mit altem Stand und ausführendem Konto; Einstellung gesetzt
    und entfernt, der Eintrag zum Entfernen hat vorher 400.
  - Aufbewahrung: Einstellung 30 Tage gilt, ein ungültiger Wert fällt auf 365 zurück; ein 31
    Tage alter Eintrag wird gelöscht, ein frischer bleibt.
  - Komplette CI (`scripts/ci.sh`) grün am 2026-10-27 (erster Lauf): Lizenzköpfe, gofmt, sqlc, oapi-codegen, vet, golangci-lint, E34-Prüfung, alle Go-Tests, Build, Lastprüfung (LP1 0,38 ms, LP2 0,60 ms, LZ1 2,22 ms, LZ2 4,25 ms, LZ3 7,46 ms, LZ4 6,34 ms p95; LP1/LP2 gegen die Basislinie -12 %/-6 %), Web-Lint, Format, Tests, Build, Playwright mit axe.
- **Nicht geprüft:** keine Oberfläche; OIDC-Anmeldung und Token-Ausgabe nur über bestehende
  Pfade ohne eigenen Test des Protokolleintrags; Anfragen mit ungültigem Bearer-Token und
  abgelehnte Änderungsversuche werden nicht protokolliert; keine Begrenzung von
  Anmeldeversuchen (kommt mit PL28).

### PL28 — Sicherheitseinstellungen, 2026-10-27

- **Grundlage:** Hauptplan E72, E73, E103; Inventar G177; E51 dieses Plans; deine
  Entscheidungen vom 2026-10-27 (Kennwortregeln einstellbar, Vorgabe bleibt E73; Sitzung
  einstellbar plus Sperre nach Fehlversuchen; Zwei-Faktor über Keycloak; Realm-Datei verlangt
  OTP, Prüfung von Hand; Einstellung für lokale Anmeldung).
- **Gebaut:** `internal/auth/policy.go`, `internal/auth/lockout.go`, Sperrliste
  `common_passwords.txt`, Lebensdauer in der Sitzung, `Login` mit Adresse und den Gründen
  `locked` und `local_login_disabled`, `ForgetPolicy` nach Einstellungsänderungen,
  Verdrahtung in `main.go`, OTP-Richtlinie und Pflichtaktion „OTP einrichten“ in
  `compose/keycloak/itsm-realm.json`, neues Kennwort des Lastkontos in `ci.sh`, im Lastwerkzeug
  und in den Playwright-Tests.
- **Belegt, 2026-10-27 im Werkzeug-Container** (Tests gegen PostgreSQL und Valkey, Linter ohne
  Befund):
  - Abnahmekriterium Kennwort: mit `security.password.min_length` 12 lehnt die eigene
    Kennwortänderung „Kurz-Pw1!“ mit 422 und „at least 12 characters“ ab, ein langes
    Kennwort geht durch.
  - Regeln: zu kurz, ohne Klassen, „Passwort123!“, „!Kennwort2026“ und ein Kennwort mit dem
    Benutzernamen werden abgelehnt, jeweils mit der Regel; mit Mindestlänge 12 ohne Klassen
    geht „wolkentischlampe“; eine eigene Sperrliste greift; ungültige Einstellungen fallen auf
    die Vorgaben zurück.
  - Sperre: drei Fehlversuche (Einstellung 3) sperren auch das richtige Kennwort mit Grund
    `locked`, `Unlock` hebt es auf; fünf Fehlversuche verschiedener Namen von einer Adresse
    sperren die Adresse, eine andere Adresse kommt durch; über HTTP steht `locked` im
    Ereignisprotokoll.
  - Lokale Anmeldung: `none` sperrt alle, `admins` lässt nur Konten mit `manage_rights` durch
    (über HTTP: Testkonto 401 mit `local_login_disabled` im Protokoll, Administrator 200).
  - Sitzung und Kennwortalter: mit `idle_hours` 2 hat die Sitzung in Valkey eine Lebensdauer
    von 2 Stunden; mit `max_age_days` 30 läuft ein neues Kennwort nach 30 Tagen ab.
  - Gefunden in der CI: die Tests melden sich absichtlich falsch an, alle von `127.0.0.1`; nach
    20 Fehlversuchen war die Adresse gesperrt und alle weiteren HTTP-Tests kamen nicht mehr
    hinein. Der Testserver schaltet die Sperre jetzt ab, solange ein Test sie nicht selbst
    setzt.
  - CI `scripts/ci.sh` am 2026-10-27 grün, in zwei Teilen, weil der erste Lauf mit dem Sitzungsende nach den Web-Unittests abbrach: Lizenzköpfe, Format, Generate, Vet, Linter, E34, alle Go-Tests mit `-race`, Lastgate (LP1 p95 0,48 ms gegen 0,53 ms der Basislinie, LP2 0,74 gegen 0,78 ms, LZ1 2,64 ms, LZ2 4,92 ms, LZ3 8,84 ms, LZ4 8,51 ms), Web-Lint, -Format und -Tests (21); danach Web-Build und Playwright (2 bestanden, der Bildlauf über 100.000 Datensätze wie immer übersprungen).
- **Nicht geprüft:** Zwei-Faktor über Keycloak prüfst du von Hand: die erste Anmeldung von
  `test` verlangt die Einrichtung einer Authenticator-App, jede weitere den Code; ohne
  gültigen Code kommt niemand zu itsm durch. Lokal gilt seit 2026-10-28 für alles das
  gemeinsame Keycloak aus `Projects/keycloak-group24/local` (Port 8081); dort ist der Realm am
  2026-10-28 mit keycloak-config-cli nachgespielt (`test` und `user` haben jetzt die
  Pflichtaktion OTP, der Befehl steht in deiner `compose.override.yaml`). Das eigene Keycloak
  von itsm (Profil `own-keycloak`) ziehen wir erst nach, wenn itsm unabhängig getestet
  werden soll.
  Die Sperre ist je Instanz-Valkey, nicht über mehrere Valkey-Knoten; die Sperrliste ist
  klein; keine Oberfläche für die Einstellungen. Achtung beim Betrieb: ohne
  `ITSM_TRUST_PROXY` hinter einem Proxy oder Docker-Netz kommen alle Anfragen von einer
  Adresse, dann sperren 20 Fehlversuche beliebiger Personen alle; dort die Proxy-Regel setzen
  oder `security.lockout.ip_failures` auf 0.

### PL29 — Lizenzprüfung der Abhängigkeiten, 2026-10-28

- **Grundlage:** Hauptplan E15, E104; E52 dieses Plans; deine Entscheidungen vom 2026-10-28
  (permissiv plus MPL-2.0; nur was ausgeliefert wird; eigenes kleines Go-Programm; Ausnahmeliste
  mit Begründung, ohne Version).
- **Gebaut:** `test/licenses/main.go` mit Test, `test/licenses/licenses.txt` (erlaubte Lizenzen,
  noch keine Ausnahme), Schritt „licenses“ in `scripts/ci.sh` nach `web: install`, neue
  Abhängigkeit `github.com/google/licensecheck` v0.3.1 (nur für das Prüfprogramm), Absatz in
  der README.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Heutiger Stand: 48 Go-Module und 37 Web-Pakete, alle erkannt und erlaubt: MIT 53,
    Apache-2.0 12, BSD-3-Clause 9, MPL-2.0 5 (River), ISC 1 (lucide-react), Unlicense 1
    (isbot), dazu Module mit zwei Lizenzen (Apache-2.0 AND MIT 2, Apache-2.0 AND
    BSD-3-Clause 1, BSD-3-Clause AND MIT 1). Die ganze Liste gibt
    `go run ./test/licenses --web=… --report` aus.
  - Abnahmekriterium: Mit einem Web-Paket unter GPL-3.0 und einem ohne erkennbare Lizenz
    scheitert der Lauf mit Rückgabe 1 und nennt beide mit Version und Lizenz; mit den heutigen
    Abhängigkeiten ist er grün (Test `TestRun`). Für Go erkennt der Prüfer die AGPL aus der
    eigenen `LICENSE` als nicht erlaubt und eine Datei in eigenen Worten als unbekannt
    (`TestGoLicense`).
  - SPDX-Ausdrücke: `(MIT OR CC0-1.0)` und `GPL-3.0 OR MIT` gehen durch, `MIT AND GPL-3.0` und
    Unlesbares nicht. Ausnahmen: eine passende gilt, eine mit anderer Lizenz und eine für ein
    Paket, das nicht mehr ausgeliefert wird, scheitern.
  - CI `scripts/ci.sh` am 2026-10-28 grün mit dem neuen Schritt „licenses“ (48 Go-Module, 37 Web-Pakete, „licenses ok“), dazu alle Go-Tests mit `-race`, Linter, Lastgate (LP1 p95 0,36 ms gegen 0,39 ms der Basislinie, LP2 0,57 gegen 0,63 ms; LZ1 2,04 ms, LZ2 5,25 ms, LZ3 16,34 ms, LZ4 9,22 ms, LZ3 und LZ4 höher als zuletzt, weil ich während der Messung im Werkzeug-Container nachgelesen habe), Web-Tests (21) und Playwright (2 bestanden, Bildlauf wie immer übersprungen). Der erste Lauf brach ab, weil Docker während der Lastmessung neu startete und den itsm-Stack nicht wieder hochfuhr; nicht von PL29.
- **Nicht geprüft:** Einen echten Probe-Zweig mit einer GPL-Abhängigkeit im Lockfile habe ich
  nicht angelegt; der Test speist die Ausgabe von `pnpm licenses` mit einem GPL-Paket ein. Die
  Entwicklungswerkzeuge prüft der Schritt bewusst nicht (heute auch Python-2.0 und CC-BY-4.0
  darunter). Lizenzhinweise, die nur in einzelnen Quelldateien und nicht in der Lizenzdatei
  des Moduls stehen, sieht der Prüfer nicht.

### PL30 — API-Abdeckung der Pakete von Meilenstein 1, 2026-10-28

- **Grundlage:** Hauptplan E105; Inventar G208, G209, G213, G214, G216, G219, G223; E53
  dieses Plans; deine Entscheidungen vom 2026-10-28 (allgemeiner Test jetzt, Antworten gegen
  die Beschreibung prüfen, der Test legt je Typ einen Datensatz an).
- **Gebaut:** `internal/httpapi/coverage_test.go` (`TestAPICoverage`), Hilfsfunktion
  `installPackage` in `api_test.go` (auch von `apiServer` genutzt). Zwei Fehler, die der Test
  gefunden hat, sind behoben:
  - `api/openapi.yaml` war kein gültiges OpenAPI: In der Kurzschreibweise `{ … }` trennte das
    Komma in zwölf unquotierten Beschreibungen einen zusätzlichen Schlüssel ab (zum Beispiel
    `rate` in `CostCreate` mit dem Schlüssel „time entries only“). Die Beschreibungen stehen
    jetzt in Anführungszeichen; Server-Code und Web-Typen sind neu erzeugt.
  - Das Schema einer Baumliste nannte `full_name` nicht, das die API neben den Feldern
    ausliefert (PL16). `typeSchema` nimmt es jetzt als nur lesbares Feld auf.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Abnahmekriterium: Für alle 7 Typen der Pakete (`core.group`, `core.group_member`,
    `core.person`, `core.person_email`, `probe.color`, `probe.place`, `probe.probe`) gehen
    Typdefinition, Liste, Einzelabruf und Einzelabruf mit `translations=all` mit Status 200
    durch und passen zur Beschreibung; ebenso `/api/v1/org/nodes`, der Wurzelknoten und seine
    Einstellungen. Vor den beiden Korrekturen ist der Test an genau diesen Stellen gescheitert
    (ungültige Beschreibung; `property "full_name" is unsupported` bei `probe.place`).
  - CI `scripts/ci.sh` am 2026-10-28 grün: Lizenzköpfe, Generate (Server-Code und Web-Typen aus der korrigierten Beschreibung), Linter, alle Go-Tests mit `-race` samt `TestAPICoverage`, Lastgate (LP1 p95 0,37 ms gegen 0,42 ms der Basislinie, LP2 0,62 gegen 0,72 ms; LZ1 2,36 ms, LZ2 4,43 ms, LZ3 8,43 ms, LZ4 5,90 ms), Lizenzen, Web-Tests (21), Playwright (2 bestanden, Bildlauf wie immer übersprungen).
- **Nicht geprüft:** Vorgänge, Wissen und Benachrichtigungen, weil es die Pakete noch nicht
  gibt; der Test nimmt sie mit, sobald sie in `packages/` liegen (Hauptplan E105). Geprüft
  werden nur die GET-Antworten, nicht die Anfragen und nicht Anlegen, Ändern oder Löschen
  gegen die Beschreibung; verschachtelte Objekte innerhalb eines Schemas (zum Beispiel die
  Sprachen eines übersetzbaren Felds) bleiben offen für weitere Felder.

### PL31 — Bulk synchron bis 1.000, 2026-10-28

- **Grundlage:** Hauptplan E24, E38, E106; architektur.md §6.4; Inventar G35; E54 dieses Plans;
  deine Entscheidungen vom 2026-10-28 (Quelle je Aufruf und Schlüssel je Zeile; jede Zeile für
  sich; Prüfsumme über die gelieferten Felder, nur diese ändern; Idempotenz in Valkey,
  24 Stunden).
- **Gebaut:** `internal/bulk` mit Test, `internal/records/bulk.go` (`Sync`, `SyncKey`),
  `create` und `update` mit Abgleichschlüssel, `internal/httpapi/bulk.go` mit Test, Pfad
  `/api/v1/records/{typeName}:bulk` und die Schemas `BulkRequest`, `BulkOutcome` und
  `BulkResult` in `api/openapi.yaml`, Pfad je Typ in der wirksamen Beschreibung, Valkey für die
  Idempotenz in `httpapi.Options` und `main.go`, Problemtitel `too_large` und `unavailable`.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Abnahmekriterium: 1.000 Zeilen `probe.probe` einmal → 1.000 angelegt (1.000 Ereignisse);
    dieselben noch einmal → 1.000 unverändert, Verlauf und Ereignisse dieser Datensätze bleiben
    gleich, Version 1; mit demselben `Idempotency-Key` byte-gleiche Antwort mit
    `Idempotent-Replayed: true`, mit anderem Inhalt `422` „reused“ (`TestBulkOverHTTP`).
  - Ein geändertes Feld gibt `updated` mit Version 2; eine Zeile mit weniger Feldern lässt die
    übrigen stehen; Quelle und Schlüssel stehen am Datensatz.
  - Zeilen scheitern allein: ohne `external_key` und mit `priority` 9 je ein Fehler mit dem Feld,
    die gute Zeile daneben wird angelegt; ein Datensatz im Papierkorb gibt `conflict`;
    1.001 Zeilen und eine leere Quelle geben `422`, ein unbekannter Typ `404`; ein Fehler der
    Datenbank bricht den Aufruf ab (`internal/bulk`).
  - Die wirksame Beschreibung hat `/api/v1/records/probe.probe:bulk` mit `BulkRequest`, und
    `:query` behält sein eigenes Schema.
  - Dauer ohne Race-Detektor: 1.000 neue Zeilen in 3,5 s, 1.000 unveränderte in 0,56 s
    (mit Race-Detektor 6,9 s und 3,6 s).
  - CI `scripts/ci.sh` am 2026-10-28 grün: Lizenzköpfe, Generate, Linter, E34, alle Go-Tests mit `-race` (neu `internal/bulk` und `TestBulkOverHTTP`), Lastgate (LP1 p95 0,34 ms gegen 0,39 ms der Basislinie, LP2 0,54 gegen 0,60 ms; LZ1 2,01 ms, LZ2 3,78 ms, LZ3 7,33 ms, LZ4 5,98 ms), Lizenzen, Web-Tests (21), Playwright (2 bestanden, Bildlauf wie immer übersprungen).
- **Nicht geprüft:** Eine Zeile, die an Rechten scheitert (`forbidden`), nur mit einem
  nachgebauten Schreiber, nicht über HTTP mit einem eingeschränkten Konto. Zwei gleichzeitige
  Aufrufe mit demselben `Idempotency-Key` laufen beide (der zweite findet die Zeilen dann
  unverändert). Jede Zeile ist eine eigene Transaktion, rund 3,5 ms je neuer Zeile; schneller
  wird es mit `COPY` im asynchronen Bulk (PL32). Unterobjekte, Vollstand und Feldhoheit folgen
  mit PL33 und PL34.

### PL32 — Bulk asynchron, 2026-10-28

- **Grundlage:** Hauptplan E24, E38, E106, E107; architektur.md §6.4; E55 dieses Plans; deine
  Entscheidungen vom 2026-10-28 (mengenbasiert in `records`; Verlauf wie bisher; derselbe Pfad
  mit NDJSON; Summen plus die ersten 1.000 Fehler; den eigenen Bulk-Job immer lesen; LZ5 als
  neue Messart auch in der CI).
- **Gebaut:** `internal/records/bulkset.go` mit Test, Nachschlagen je Stapel in `validate`,
  `searchParts` und `searchOf` in `search.go`, `internal/bulk/async.go`, NDJSON-Weg und
  gemeinsame Idempotenz in `internal/httpapi/bulk.go`, Leserecht auf den eigenen Bulk-Job in
  `jobs.go`, `output` in `jobs.Job` und im Schema `Job`, Parameter `source`, Inhalt
  `application/x-ndjson` und Antwort `202` am Bulk-Pfad, Verdrahtung in `main.go` (nur mit
  `ITSM_FILES_DIR`), ein Bulk-Worker auf eigener Queue im Testserver, LZ5 im Lastwerkzeug
  (`scenario/bulk.go`, Messart und Gate), `ITSM_FILES_DIR` in der Laststufe von `ci.sh`,
  `events.Service.Consumer`.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Abnahmekriterium: 100.000 Zeilen `probe.probe` als NDJSON → `202` mit dem Job; der
    Jobstand zeigt den Fortschritt bis „fertig“; 100.000 angelegt, 100.000 Ereignisse, 500.000
    Verlaufszeilen (anlegen plus Titel, Priorität und die Vorgaben `active` und `kind`) in rund
    5 s einschließlich Upload, also gut 20.000 Zeilen/s; derselbe Strom noch einmal: 100.000
    unverändert in rund 1 s, kein neues Ereignis (`TestBulkStreamOverHTTP`).
  - Ein Datensatz aus dem Strom ist wie einer über die API: Werte, Status `new`, Vorgabewerte,
    Version 1, die Volltextsuche findet ihn, der Verlauf zeigt das Anlegen.
  - Derselbe `Idempotency-Key` mit demselben Strom gibt dieselbe Antwort mit derselben
    `Location` und `Idempotent-Replayed: true`, ohne zweiten Job.
  - Fehler je Zeile mit Zeilennummer: kein JSON, ohne Schlüssel, ein Schlüssel zum zweiten Mal
    (`conflict`), `priority` 9; eine Leerzeile zählt nicht; die gute Zeile daneben wird
    angelegt, eine bekannte geändert. Ohne Quelle und mit leerem Strom `422`, über
    `bulk.max_size` `413`.
  - Ein Konto ohne Rechte auf den Typ: seine Zeilen scheitern mit `forbidden`; seinen eigenen
    Job darf es lesen, den eines anderen und die Jobliste nicht (`403`)
    (`TestBulkStreamOwnJob`).
  - Mengenweg direkt (`TestSyncBatch`): alle Feldarten, Verlauf und Ereignisse wie bei Create
    und Update, ein geändertes Feld ändert nur dieses, eine neue Prüfsumme bei gleichen Werten
    ändert nichts; ein eindeutiger Wert zweimal im Stapel geht für genau diese eine Zeile über
    den Zeilenweg (gezählt), Papierkorb gibt `conflict`.
  - Gate: ein Durchsatz unter dem Ziel und 17 % unter der Basislinie scheitern, 4 % darunter
    nicht; die volle Stufe misst in Zeilen/s.
  - Gefunden in der CI: `TestEventsAPI` wartete vergeblich auf ein Live-Ereignis, weil sein
    Verteiler die gemeinsame Position mit dem laufenden Container teilt und hinter den
    100.000 Ereignissen des Bulk-Tests anfing. `events.Service` hat jetzt einen einstellbaren
    `Consumer` (Vorgabe `dispatcher`); der Test nimmt einen eigenen ab dem Ende des Protokolls.
    Im Betrieb heißt das: 100.000 Datensätze aus einem Bulk-Lauf sind 100.000 Ereignisse, die
    der Verteiler (Webhooks, SSE) danach abarbeitet.
  - Ebenfalls gefunden: `TestStreamDispatchAndRetention` las den Strom, während ein Stapel des
    Bulk-Tests im anderen Testpaket seine Transaktion offen hatte; der Sichtbarkeitshorizont
    (E87) hält dann alles Spätere zurück, und der Test fand nichts. Das ist gewollt; der Test
    wartet jetzt wie die anderen Ereignistests, bis die Ereignisse sichtbar sind. Im Betrieb
    sehen Leser des Stroms während eines großen Bulk-Laufs neue Ereignisse um bis zu einen
    Stapel verzögert (rund eine Viertelsekunde).
  - CI `scripts/ci.sh` am 2026-10-28 grün (dritter Lauf, nach den beiden Funden unten): Lizenzköpfe, Generate, Linter, E34, alle Go-Tests mit `-race`, Lastgate mit LZ5 erstmals gemessen: 20.000 Zeilen (15.000 `probe.probe`, 5.000 `core.person`, alle geändert) mit 11.039 Zeilen/s, Ziel ≥ 5.000, die Basislinie kann es noch nicht; LP1 p95 0,34 ms gegen 0,37 ms der Basislinie, LP2 0,54 gegen 0,59 ms; LZ1 1,91 ms, LZ2 5,09 ms, LZ3 3,56 ms, LZ4 6,68 ms; Lizenzen, Web-Tests (21), Playwright (2 bestanden, Bildlauf wie immer übersprungen).
- **Nicht geprüft:** Ein Lauf, der an einem Fehler der Datenbank mitten im Strom scheitert und
  von River wiederholt wird (die schon geschriebenen Stapel sind dann unverändert, die Zahlen
  im Ergebnis zählen nur den letzten Versuch). Zwei Läufe derselben Quelle gleichzeitig. Die
  Fortschrittsanzeige ist nur über die Ausgabe des Jobs belegt, nicht während eines langen
  Laufs beobachtet. Ein Strom über 1 GiB.

### PL49 — Laufende Nummer je Typ und Mandant, 2026-11-02

- **Grundlage:** E63 dieses Plans, Hauptplan E157; SD-Plan §2.3 (N1); deine Entscheidungen vom
  2026-11-02 (Sequenz je Typ und Mandant, Zahl speichern, Bestand nachnummerieren, Import mit
  Quelle).
- **Gebaut:**
  - Feldtyp `sequence` mit `prefix` (höchstens 20 Zeichen ohne Leerzeichen; kein `required`,
    `default`, keine Formel). Er zählt zu den berechneten Feldern: Anlegen und PATCH mit einer
    Nummer sind 422 `read_only`. Spalte `bigint`, eindeutiger Index `(tenant, feld)` ohne Ausnahme
    für den Papierkorb, damit keine Nummer zweimal vorkommt.
  - `internal/records/sequence.go`: Sequenz je Typ, Feld und Mandant (`seq_` plus Hash, passt in
    63 Zeichen), beim ersten Gebrauch unter `pg_advisory_xact_lock` angelegt und danach im
    Prozess gemerkt; `nextval` in der Transaktion des Anlegens. Der Mandant kommt aus
    `tenant_of(knoten)`, in beiden Betriebsarten (eigene Datenbank: die Sequenz liegt dort). Eine
    neue Sequenz beginnt über der höchsten Nummer, die der Mandant schon hat: Datensätze, die ein
    Mandant beim Anlegen mit `--adopt` von der Wurzel übernimmt, behalten ihre Nummern (Fund aus
    `TestTenantBoundaryOverHTTP`, der sonst an der Eindeutigkeit scheiterte).
  - Import: `:bulk` und der Datenstrom mit Quelle dürfen die Nummer mitgeben (Zahl oder Text mit
    Präfix, etwa `PRB-5000`); die Sequenz springt per `setval` darüber, im selben Stapel auch für
    die Zeilen ohne Nummer. Dieselbe Zeile wieder mit derselben Nummer ist erlaubt, mit einer
    anderen ein Fehler der Zeile. Stapel ziehen ihre Nummern in einem Zug in der Reihenfolge der
    Zeilen.
  - Suche: die Suchspalte bekommt „PRB-24817 24817“; Filter nehmen die Nummer mit oder ohne
    Präfix. Export, Mails (`{{feld}}`) und Oberfläche zeigen sie mit Präfix, das Formular nur
    lesend.
  - Bestand: bekommt ein Typ das Feld beim Paket-Update, nummeriert der Job `records.recompute`
    (neu mit `number`) die Datensätze ohne Nummer nach Anlagezeit und füllt die Suchspalte neu;
    über die Kundenschicht geschieht das gleich in der Anfrage.
  - Paket `probe`: Feld `number` mit Präfix `PRB-`.
- **Belegt:**
  - Go `TestSequenceOverHTTP` (geteilt und eigene Datenbank je Mandant): A bekommt 1 und 2, B
    beginnt wieder bei 1; 100 Anlagen zugleich ergeben 100 verschiedene Nummern; PATCH und Anlegen
    mit Nummer 422; die Suche findet `PRB-2` und `2`, der Filter `PRB-2`; ein Import mit `PRB-5000`
    behält sie, die Zeile ohne Nummer im selben Stapel bekommt 5001, das nächste Anlegen 5002;
    dieselbe Zeile wieder gilt als geändert, eine andere Nummer dafür als Fehler.
  - Go `TestSequenceForExistingRecords`: drei Datensätze, dann das Feld über die Kundenschicht
    (`D-`): sie bekommen 1, 2, 3 in der Reihenfolge des Anlegens, der nächste 4, die Suche findet
    `D-2`.
  - `TestBulkStreamOverHTTP` zählt jetzt sechs Verlaufszeilen je Datensatz statt fünf: die
    Nummer steht im Verlauf des Anlegens (angepasst, weil das so gewollt ist).
  - Vitest (`render/Sequence.test.tsx`, 3 Fälle): Liste zeigt `K-2`, axe ohne Befund, das
    Formular zeigt `K-2` nur lesend; Filter schickt `K-3`; das Formular sendet die Nummer nie.
  - Playwright (`e2e/sequence.spec.ts`, Konto `load`): neuer Datensatz bekommt eine Nummer; Strg K
    und `PRB-<Nummer>` finden ihn, die Detailseite zeigt die Nummer nur lesend (der Schritt, den die
    Befehlszeile laut UI-Plan E36 an PL49 abgegeben hat).
  - Ganze CI am 2026-11-03 (`scripts/ci-run.sh`, 9:08 min, zweiter Lauf): alle 19 Schritte grün,
    90 Vitest-Fälle, 17 Browser-Tests, LZ1 p95 2,88 ms, LZ5 8.840 Zeilen/s, LZ13 7.881 Zeilen/s.
    Vorher fand die ganze Go-Suite zwei Fehler: übernommene Datensätze (`--adopt`) stießen mit der
    neuen Sequenz des Mandanten zusammen (behoben, die Sequenz beginnt über der höchsten Nummer)
    und ein Wettlauf beim Spiegelkonto (PL47b, eigener Nachtrag). Im ersten CI-Lauf war nur der
    Test der Zeitleiste (UI3, UI7) rot, zum zweiten Mal nach UI30: ein neuer Eintrag stand unter
    dem Statuswechsel „Version 2“, dessen Zeit in der Zeitleiste später lag als die des Eintrags,
    obwohl der Wechsel im Test zwei Sekunden früher geschah; beide Zeiten kommen aus `now()` der
    Datenbank. Ursache offen, die Spur liegt in WSL unter `~/ci-befunde/zeitleiste-2026-11-02`.
- **Nicht geprüft:** Der Typ-Editor (UI25) bietet `sequence` noch nicht als Art an; über die API
  der Kundenschicht geht es. Das Nummerieren des Bestands über die Kundenschicht läuft in der
  Anfrage, bei sehr vielen Datensätzen dauert sie entsprechend. Übernimmt ein Mandant per `--adopt`
  Datensätze, nachdem seine Sequenz schon besteht, kann sie unter deren Nummern liegen; der
  eindeutige Index verhindert doppelte Nummern, das Anlegen scheitert dann, bis jemand die Sequenz
  per `setval` hochsetzt.

### PL51 — CI mit eigenem Stand je Lauf, 2026-11-02

- **Grundlage:** Hauptplan E150; deine Entscheidung vom 2026-11-02 (Punkte 1 bis 4 aus der
  Durchsicht der Testsuite: eigene Server, zeitkritische Pakete nacheinander, kein Abbruch,
  Browser-Tests einzeln; immer derselbe Stand am Anfang, alles aufgeräumt am Ende, alle Fehler
  gesammelt und bewertet).
- **Gebaut:**
  - `compose.yaml`: Dienste `postgres-ci` und `valkey-ci` im Profil `ci`, Daten im
    Arbeitsspeicher (`tmpfs`, Valkey ohne Persistenz), ohne Ports; der Werkzeug-Container zeigt
    mit `ITSM_TEST_DATABASE_URL` und `ITSM_TEST_VALKEY_ADDR` auf sie.
  - `scripts/ci-run.sh` (neu, auf dem Rechner): startet beide mit `--force-recreate` und wartet
    auf gesund, ruft `scripts/ci.sh` im Werkzeug-Container, entfernt beide danach (auch bei
    Abbruch, `trap`).
  - `scripts/ci.sh`: jeder Schritt ist eine Funktion und läuft über `run` in einer Unterschale
    mit `set -e`; Ausgabe auf den Bildschirm und in ein Protokoll je Schritt; ein Schritt, der
    einen roten Schritt braucht (`go: build` für die Laststufe, `web: install` für die
    Web-Schritte, Laststufe und Web-Build für die Browser-Tests), wird übersprungen. Am Ende die
    Zusammenfassung aller Schritte und je rotem Schritt die Zeilen mit `--- FAIL`, `FAIL`,
    Testdatei und Zeile, `✘`, `×` oder `Error`; Rückgabe 1 bei einem roten Schritt, sonst
    `ci: all steps passed`. Der Schritt „Datenbank der CI“ wartet auf den Server und zeigt die
    Datenbanken, die er vorfindet.
  - Go-Tests in zwei Phasen: alle Pakete außer `internal/events`, `internal/webhooks`,
    `internal/jobs`, `internal/httpapi` und `test/load` nebeneinander wie bisher, dann diese
    fünf mit `-p 1` nacheinander.
  - `web/playwright.config.ts`: `workers: 1`.
- **Belegt, 2026-11-02:**
  - Erster Lauf der neuen Suite grün in einem Durchgang (8 min 48 s, vorher etwa 7 min): der
    Schritt „Datenbank der CI“ fand auf dem frischen Server nur `itsm_ci` und `postgres`; Go in
    zwei Phasen (38 Pakete nebeneinander, dann events, httpapi 183 s, jobs, webhooks und das
    Lastwerkzeug nacheinander); Leistungsgate bestanden (LZ1 p95 3,19 ms, LZ2 5,26 ms, LZ3
    6,99 ms, LZ4 10,34 ms, LZ5 11.120 Zeilen/s, LZ13 10.100 Zeilen/s); 62 Vitest-Fälle; 10
    Playwright-Fälle mit axe einzeln in 51 s (vorher 24 bis 31 s nebeneinander). Die
    Zusammenfassung führt alle 19 Schritte mit `ok`.
  - Nach dem Lauf: keine Container `postgres-ci` und `valkey-ci` mehr (`docker ps -a`); der
    Server der Instanz hat unverändert `itsm`, `itsm_ci` (alt, seit E150 ungenutzt),
    `itsm_mandanten`.
  - Der Fall „ein roter Schritt hält die übrigen nicht auf“ ist im Lauf nicht vorgekommen; die
    Mechanik (Unterschale mit `set -e`, Status je Schritt, Überspringen) ist mit `dash -n`
    geprüft und greift beim nächsten roten Lauf.
- **Nachtrag 2026-11-02 (Befund aus der CI von UI31, deine Entscheidung):** Im ersten Lauf nach
  einem Neustart legten zwei Testpakete gleichzeitig die Tabellen von River auf der frischen
  Datenbank an; einer scheiterte an `pg_type_typname_nsp_index`. Die eigenen Migrationen sperrt
  goose schon, die von River nicht. Im Betrieb träfe es zwei Server, die gleichzeitig auf einer
  neuen Datenbank starten (etwa ein neuer Mandant mit eigener Datenbank).
  - Gebaut: `internal/db/db.go` hält um die River-Migration eine Postgres-Sperre
    (`pg_advisory_lock`, Schlüssel `riverLock`) auf einer eigenen Verbindung.
  - Belegt: `TestMigrateRiverAtOnceOnAnEmptyDatabase` startet acht River-Migrationen zugleich auf
    einer leeren Datenbank (`dbtest.Isolated`): alle gelingen, jede Version steht einmal in
    `river_migration`. Gegenprobe ohne Sperre: der Test scheitert in beiden Läufen.
  - Ganze CI am 2026-11-02 grün im ersten Lauf (9:21 min): alle 19 Schritte, 82 Vitest-Fälle,
    15 Browser-Tests, LZ1 p95 3,38 ms, LZ13 10.015 Zeilen/s.
- **Nicht geprüft:** Der Lauf dauert durch die zweite Phase und die einzelnen Browser-Tests
  knapp 2 min länger (8 min 48 s statt etwa 7 min). Die Caches von Go und pnpm und das Volume der Basislinie bleiben zwischen den
  Läufen, das ist gewollt. Der Werkzeug-Container selbst wird nicht neu gebaut.

### PL52 — Schnellere CI in zwei Stufen, 2026-11-03

- **Grundlage:** Hauptplan E162; deine Entscheidungen vom 2026-11-03 (Teil-CI während der
  Arbeit, ganze CI vor jedem Commit, Lastmessung nur bei Änderungen am Server; Basislinie ist
  der grüne Stand am Ende von PL52 mit deiner Freigabe; LZ9 einmal ungemessen aufwärmen; das
  Zeitziel gilt für die ganze CI ohne Lastmessung).
- **Gebaut:**
  - `scripts/ci-run.sh`: `--part` für die Teil-CI, `--all` für die ganze CI wie bei einer
    Änderung überall; ohne Angabe die ganze CI. Was sich seit dem letzten Commit geändert hat,
    schreibt es nach `.ci-changed` (in `.gitignore`); die Kopie in WSL bringt die Liste mit.
  - `scripts/ci.sh`, Umfang über `ITSM_CI_SCOPE`:
    - Teil-CI: Go-Vet, Lint und Tests nur für die geänderten Pakete und alle, die sie nutzen
      (`go list` über Abhängigkeiten und Test-Importe); Vitest mit `vitest related` für die
      geänderten Quellen; die Browser-Tests des Bereichs (Zuordnung in `e2e_specs`, alle bei
      Änderungen an API, Übersetzungen, Test-Helfern oder Konfiguration); keine Lastmessung.
    - Ganze CI: Go-Schritte nur bei Änderungen an Go-Code, Migrationen, API oder Paketen; die
      Laststufe und die Tests des Lastwerkzeugs nur bei Änderungen an `cmd/`, `internal/`,
      `db/`, `go.mod` oder `test/load/`; die Lizenzprüfung bei geänderten Modulen; die
      Web-Schritte bei Änderungen unter `web/`. Jeder übersprungene Schritt nennt den Grund.
    - `internal/httpapi` läuft in drei Teilen nebeneinander, je mit eigener Datenbank
      (`itsm_ci_h1` bis `_h3`) und eigener Valkey-Datenbank; die Tests aus Dateien, die
      `/api/v1/events` lesen, laufen danach allein, weil der Horizont des Ereignisstroms (E87)
      der älteste offene Vorgang des ganzen Servers ist.
    - Die Zusammenfassung nennt die Dauer jedes Schritts.
  - `internal/cache`: eine Adresse `host:port/n` wählt die Valkey-Datenbank `n`; die Kanäle von
    Pub/Sub bekommen dann den Vorsatz `db<n>:`, weil sie anders als die Schlüssel für den ganzen
    Server gelten (`TestOpenSelectsADatabase`).
  - Wackelnde Tests: `records.spec` besucht die Liste einmal ungemessen vor LZ9; `forms.spec`
    wartet zwei Bildaufbauten nach der Ansage, bevor die Pfeiltaste kommt; Formular-Baukasten
    und Workflow-Editor haben in Vitest 15 s statt 5 s je Test.
  - Basislinie der schnellen Stufe neu: Binary des Stands nach SD4 und PL52 in
    `/var/lib/itsm-load/itsm-baseline` (sha256 `b0ac8f7cab39…`), Messwerte aller Szenarien in
    `test/load/baseline.json`; `approved_by` ist leer und wartet auf deine Freigabe.
- **Belegt, 2026-11-03:**
  - Teil-CI mit allen Änderungen grün (3:55 min, `internal/httpapi` 130 s statt 210 s); Teil-CI
    für eine Änderung nur in `web/src` grün in 1:08 min: Go-Schritte und Lastmessung
    übersprungen mit Grund, gewählt die Browser-Tests `live`, `records` und `relations`.
  - Ganze CI mit Lastmessung grün in 11:08 min: die schnelle Stufe vergleicht LP1, LP2, LZ1 bis
    LZ5 und LZ13 wieder gegen die Basislinie im selben Lauf (LZ1 p95 3,02 ms gegen 3,38 ms, LZ4
    10,71 ms gegen 10,68 ms, LZ13 8.709 gegen 9.101 Zeilen/s). Dauer je Schritt: Go-Tests
    255 s, Lastmessung 295 s, Browser-Tests 84 s, alles andere zusammen unter 40 s.
  - Ganze CI ohne Lastmessung (Änderung an Paket-YAML, Go-Tests ohne das Lastwerkzeug) grün in
    5:18 min, also 5 min schneller als 10:27 min.
  - Vier ganze Läufe nacheinander grün ohne wackelnden Test: 9:37 min (alte Basislinie),
    11:08 min, 5:18 min und vor dem Commit 10:43 min (LZ1 p95 2,97 ms gegen 3,23 ms der
    Basislinie, LZ4 9,16 ms gegen 8,99 ms). LZ9 maß 80 ms, 110 ms und 96 ms.
- **Nicht geprüft:**
  - Mit Lastmessung ist die ganze CI länger als vorher (11:08 statt 10:27 min), weil die alte
    Basislinie die Szenarien LZ1 bis LZ13 gar nicht mehr messen konnte; die Messung dauert
    jetzt gut 2 min länger, ist dafür wieder ein echter Vergleich.
  - Die API-Tests dauerten in drei Läufen 126 s, 139 s und 183 s; die Streuung kommt aus den
    Tests des Ereignisstroms, die allein laufen. Nicht weiter untersucht.
  - Der Selbsttest des Lastwerkzeugs braucht allein 72 s und läuft nicht neben anderen Tests.
    Nebenher zu laufen würde etwa 70 s sparen; das bleibt bei deiner Entscheidung offen.
  - Die Auswahl der Browser-Tests nach Dateien ist eine Liste von Hand; ein neuer Bereich ohne
    Eintrag wählt keinen Browser-Test, bis die ganze CI vor dem Commit läuft.
- **Nachtrag 2026-11-04 (Befund aus der CI von SD16b, deine Entscheidung):** Die CI verschluckte
  rote Go-Pakete. `go_test` merkte sich einen Fehler der Pakete nebeneinander in `rc`, dann setzte
  `httpapi_test` dieselbe Variable auf 0, weil keine der beiden Funktionen sie als `local`
  führte; waren die API-Tests danach grün, meldete der Schritt „ok“. So blieb der veraltete Test
  `TestProblems` in `internal/forms` seit SD9 unbemerkt rot (in den Läufen von SD9, SD16a und
  SD16b); in den aufgehobenen Protokollen seit PL52 ist er das einzige verdeckte Paket.
  - Gebaut: `local rc=0` in `go_test` und `httpapi_test` (`scripts/ci.sh`).
  - Belegt: ganze CI mit dem noch roten Test am 2026-11-04: „go: test“ steht auf failed, die
    Zusammenfassung „1 of 19 steps failed“. Der Test selbst ist mit SD16b nachgezogen.

### PL56 — Zeitgrenze je API-Anfrage, 2026-11-15

- **Grundlage:** E69; gefunden mit H14 auf `ki-lab`: Suchen, die länger als 10 s liefen, hielten
  alle Verbindungen; auch das Anlegen und die Wahl des Leiters von River liefen in die Zeit.
  Deine Entscheidung vom 2026-11-14: 10 s, einstellbar.
- **Gebaut:** `internal/httpapi/timelimit.go`: jede Anfrage unter `/api/` bekommt eine Frist,
  außer Export, Bulk-Strom, Ereignisstrom und Hochladen; eine abgebrochene Abfrage antwortet
  503 `timeout` und steht als Warnung im Protokoll. `ITSM_REQUEST_TIMEOUT` (Vorgabe `10s`, `0`
  für keine) in `internal/platform` und im README.
- **Belegt:** Go `TestRequestTimeLimit` (welche Pfade eine Frist bekommen, die Antwort einer
  abgebrochenen Abfrage), `TestLoadConfig…` (Vorgabe, `0`, falscher Wert).
  `scripts/ci.sh` am 2026-11-15 (PL55 und PL56 zusammen): alle 19 Schritte grün; die Schnellstufe
  gegen die Basislinie überall schneller (LZ3 p95 6,5 statt 11,0 ms).
- **Nicht geprüft:** Eine Einstellung am Wurzelknoten statt der Umgebungsvariablen.

### PL55 — Suche neueste zuerst, 2026-11-15

- **Grundlage:** E68; gefunden mit H14 auf `ki-lab` (LZ3: jede Suche über 10 s). Deine
  Entscheidung vom 2026-11-14: neueste Treffer zuerst bewerten. Auf `ki-lab` vor dem Bau gemessen:
  alles 13 s, Zeitfenster über die Indizes 2,6 s, rückwärts am Anlagedatum mit Abbruch nach
  200 Treffern 3 ms, ein Wort ohne Treffer im Fenster von 30 Tagen 335 ms (darum sieben Tage),
  ein seltenes Wort über alle Jahre 6 ms.
- **Gebaut:** `internal/records/search.go`: erst je Typ die neuesten bis zu 1.000 Treffer der
  letzten sieben Tage, bewertet; sind das weniger als verlangt, die Suche über alles wie bisher.
- **Belegt:** Go `TestSearch` erweitert: ein altes Wort wird über alles gefunden, zwei neue
  Treffer genügen für zwei verlangte und der alte bleibt draußen, für zwanzig verlangte zählt
  wieder alles. Die übrigen Suchtests unverändert grün.
  `scripts/ci.sh` am 2026-11-15 (PL55 und PL56 zusammen): alle 19 Schritte grün; die Schnellstufe
  gegen die Basislinie überall schneller (LZ3 p95 6,5 statt 11,0 ms).
- **Nicht geprüft:** Wörter, die nur vor langer Zeit häufig waren (teuer, die Zeitgrenze
  schützt den Server).

### PL57 — Rechtefilter zusammenfassen, 2026-11-15

- **Grundlage:** E67; gefunden mit H14 auf `ki-lab`: die echte Abfrage von LZ2 (aus dem Protokoll
  der Datenbank) hatte vier gleiche ODER-Bedingungen und Team 07 viermal in der Liste der
  Gruppen; je Abfrage 3 s, mit einer Bedingung 0,2 s. Deine Entscheidung vom 2026-11-15:
  Filter zusammenfassen und Doppelungen verhindern.
- **Gebaut:**
  - `internal/authz`: gleiche Rechte zählen beim Laden einmal; die Inhaber für „beteiligt“
    (Person und Gruppen) je einmal; alle Rechte „beteiligt“ ohne Bedingung werden eine Bedingung
    `((org_path <@ … OR …) AND id IN (…))`; dieselbe Zuweisung ein zweites Mal ist ein Konflikt
    (409). `internal/query`: die Gruppen einer Person ohne Doppelung.
  - Migration `00048_assignment_unique.sql`: Doppelungen weg, eindeutiger Index über Rolle,
    Person, Gruppe und Knoten.
  - `meta.Index.Unique` und `internal/schema`: eindeutiger erklärter Index `<tabelle>_ux_…`
    über Mandant und Spalten außerhalb des Papierkorbs; ein Konflikt nennt die Spalten
    („group, person must be unique together“); Doppelungen in der Tabelle halten die
    Installation mit klarer Meldung an. Kernpaket 0.9.0: `core.group_member` erklärt Gruppe
    und Person eindeutig.
  - Nachtrag nach der Messung auf `ki-lab` (deine Entscheidung 2026-11-15: nur für Listen; die
    Suche behält den Plan-Speicher, weil das Neuplanen ihrer Abfrage über alle Typen rund 8 ms
    je Suche kostete und sie auf `ki-lab` keine Ausreißer hatte): pgx bereitet Abfragen vor, und PostgreSQL wechselte nach fünf Ausführungen auf einen
    allgemeinen Plan, der für die Warteschlange des Teams alle 365.000 Rollen-Einträge hashte
    (16,4 s statt 0,85 s, nachgewiesen mit `PREPARE`/`EXECUTE`). Liste, Abfrage und Aggregat
    laufen jetzt als unbenannte Anweisung (`pgx.QueryExecModeCacheDescribe`), die
    PostgreSQL jedes Mal mit ihren Werten plant (`planned` in `internal/records/query.go`);
    einzelne Datensätze behalten den Plan-Speicher.
- **Belegt:**
  - Go `TestInvolvedGrantsBecomeOneCondition` (vier gleiche und ein weiteres Recht werden eine
    Bedingung über zwei Knoten, jeder Inhaber einmal, gelesen wird dasselbe; ein Recht mit
    Bedingung behält seine eigene), `TestDeclaredIndexesFollowTheDefinition` erweitert
    (eindeutiger Index, Papierkorb zählt nicht, Doppelungen halten die Installation an),
    `TestRolesAtTheRecordOverHTTP` erweitert (Zuweisung und Mitgliedschaft doppelt: 409).
  - `scripts/ci.sh` am 2026-11-15 (PL57 und PL58 zusammen geprüft): alle 19 Schritte grün; die
    Schnellstufe gegen die Basislinie überall schneller (LZ4 p95 7,1 statt 13,2 ms, LZ2 4,4 statt 6,6 ms).
  - Nachtrag (Listen jedes Mal geplant): `scripts/ci.sh` am 2026-11-15 alle 19 Schritte grün
    (LZ2 p95 5,0 statt 7,1 ms, LZ3 7,0 statt 11,8 ms gegen die Basislinie); ein Lauf mit auch
    neu geplanter Suche war rot (LZ3 14,1 statt 11,2 ms), darum behält die Suche den Plan-Speicher.
- **Nicht geprüft:** Rechte verschiedener Art zugleich (etwa „beteiligt“ ODER „eigene
  Einheit“) bleiben eine ODER-Bedingung; bei Teams mit Hunderttausenden Vorgängen kostet das
  weiter rund eine halbe Sekunde (gemessen auf `ki-lab`).

### PL58 — Zwischenspeicher laden im Hintergrund, 2026-11-15

- **Grundlage:** E66; gefunden mit H14 auf `ki-lab`: LZ4 (Störung anlegen) scheiterte bei 32
  gleichzeitigen Anfragen ohne jede langsame Abfrage, jede Anfrage nach genau 10 s mit
  `sla: load: … context canceled`. Deine Entscheidung vom 2026-11-15: Nachladen im Hintergrund.
- **Gebaut:**
  - `internal/memo`: `Value` gibt den letzten Stand sofort; zu alt oder veraltet gemeldet, liest
    genau ein Ablauf im Hintergrund neu (mit dem Kontext der Anfrage, ohne deren Ende, höchstens
    30 s); ein fehlgeschlagenes Nachladen behält den letzten Stand; `Refresh` liest sofort im
    Aufrufer, ein älteres Nachladen überschreibt es nicht; nur das allererste Laden läuft im
    Aufrufer.
  - `internal/sla`: SLAs über `memo`; Kalender der Uhren behalten ihren Stand und werden im
    Hintergrund neu gelesen; eine eigene Änderung an SLA oder Kalender liest nach dem Commit
    sofort, die Meldung über Valkey markiert nur.
  - `internal/rules/engine`: die Regeln ebenso; `internal/settings`: die Erklärungen und die
    Werte an den Knoten (die Priorität jeder Störung liest einen) ebenso, ein unbekannter
    Schlüssel liest die Erklärungen sofort.
  - `internal/records/typecache.go`: der Ablauf der Minute und die Meldung über
    `itsm_definitions` lesen im Hintergrund; ein Schreiben des eigenen Prozesses liest wie bisher
    sofort.
  - `cmd/itsm`: Typen, SLAs, Regeln und Einstellungen werden vor der ersten Anfrage geladen.
  - `internal/customer`: nach dem Speichern der Kundenschicht gilt der neue Typ sofort für das
    Nummerieren und die Suche danach (gefunden mit der CI: bisher kam die Nachricht von
    PostgreSQL zufällig rechtzeitig, `TestSequenceForExistingRecords`).
- **Belegt:**
  - Go `TestStaleServesWhileOneLoadRuns` (50 gleichzeitige Aufrufer bekommen den alten Stand
    sofort, genau ein Nachladen), `TestRefreshForgetAndFailedLoads` (sofortiges Neuladen gewinnt
    gegen ein älteres im Hintergrund, ein fehlgeschlagenes behält den Stand), beide auch mit dem
    Race-Detector. Die Tests zu SLA, Kalendern, Regeln, Einstellungen, Typen und Kundenschicht
    über HTTP unverändert grün.
  - `scripts/ci.sh` am 2026-11-15 (PL57 und PL58 zusammen geprüft): alle 19 Schritte grün; die
    Schnellstufe gegen die Basislinie überall schneller (LZ4 p95 7,1 statt 13,2 ms, LZ2 4,4 statt 6,6 ms).
- **Nicht geprüft:** Das allererste Laden eines Speichers läuft weiter in der Anfrage; der
  Start lädt darum vor. Andere Stellen, die in einer Transaktion eine zweite Verbindung holen,
  bleiben möglich; der Pool hat weiter so viele Verbindungen wie der Rechner Kerne.

### PL54 — Indizes je Typ, 2026-11-14

- **Grundlage:** E65; gefunden mit H14 (Hauptplan E128) auf dem Server `ki-lab` (E184): LZ2
  (Warteschlange eines Teams, 50 Zeilen, Rolle nur für beteiligte Gruppen) brauchte auf
  18,25 Mio. Störungen 6,2 s in der Datenbank, weil PostgreSQL ohne Index auf Status oder
  Anlagedatum die ganze Tabelle las. Deine Entscheidung vom 2026-11-14: Typen erklären Indizes,
  der Kern indiziert das Anlagedatum.
- **Gebaut:**
  - `internal/meta/index.go`: `Type.Indexes` (YAML `indexes:`, je Eintrag `fields`), geprüft
    beim Laden: ein bis vier Spalten, Felder des Typs oder erlaubte Systemspalten, keine doppelt,
    Textfelder nur mit `max_length` bis 500.
  - `internal/schema`: Index `<tabelle>_created_at_idx` für jeden Typ, auch für bestehende
    Tabellen; erklärte Indizes als `<tabelle>_ix_<spalten>` (zu lange Namen enden in einer
    Prüfsumme), markiert mit einem Kommentar, und erklärte, die das Paket nicht mehr nennt,
    werden entfernt. Jede Änderung steht wie bisher im Schema-Protokoll.
  - Pakete `incident` 0.12.0 und `request` 0.6.0: `indexes: [state, created_at]`.
  - Nachtrag nach der ersten Messung auf `ki-lab`: die Liste sortierte jede Spalte mit
    `NULLS LAST`; damit kann PostgreSQL den Index auf `created_at` nicht rückwärts lesen und
    sortierte die 88.000 offenen Störungen über den Status-Index (LZ2 weiter 8,5 s). Spalten
    ohne leere Werte (`version`, `created_at`, `updated_at`) sortieren jetzt ohne `NULLS LAST`
    und blättern über `(spalte, id)`; das Ergebnis ist dasselbe (`internal/records/query.go`).
- **Belegt:**
  - Go `TestQuery` blättert neueste zuerst und älteste zuerst nach `created_at`.
  - Go `TestIndexesAreChecked` (Prüfung beim Laden), `TestDeclaredIndexesFollowTheDefinition`
    (anlegen, nichts bei der zweiten Installation, Wechsel legt den neuen an und entfernt den
    alten, die Indizes des Kerns bleiben, lange Namen bleiben unter 63 Zeichen und verschieden).
    `TestCreateTableAddColumnUniqueIndexAndLog` und `TestParallelApplyEndsWithOneTable` zählen
    je eine Änderung mehr (Grund: der neue Index auf dem Anlagedatum).
  - `scripts/ci.sh` am 2026-11-14: alle 19 Schritte grün, die Schnellstufe gegen die Basislinie
    überall gleich oder schneller (LZ2 p95 5,0 statt 7,5 ms, LZ5 10.321 statt 7.934 Zeilen/s).
  - Nachtrag: `scripts/ci.sh` am 2026-11-15 mit der Sortierung ohne `NULLS LAST` alle 19 Schritte
    grün (LZ2 p95 4,3 statt 6,5 ms gegen die Basislinie).
- **Nicht geprüft:** Bau eines Index ohne Sperre (`CREATE INDEX CONCURRENTLY`); auf einer großen
  Tabelle hält die Installation das Schreiben für die Dauer des Baus an. Die Kundenschicht und
  der Typ-Editor kennen `indexes` noch nicht.

### PL53 — Typen und Kalender im Speicher, 2026-11-13

- **Grundlage:** E64; gefunden mit H14 (Hauptplan E128): der Massenimport von 18,25 Mio.
  Störungen lief mit 55 Zeilen/s; das Profil (neu: `ITSM_PPROF_ADDR`, nur auf der Loopback)
  zeigte mehr als die Hälfte der Zeit in `meta.Types` (alle Definitionen lesen und dekodieren)
  und 40 % im Laden des Kalenders je SLA-Uhr. Deine Entscheidung vom 2026-11-13: eigene
  Aufgabe jetzt, dann H14 weiter.
- **Gebaut:**
  - `internal/records/typecache.go`: die Typen im Speicher, neu geladen, wenn
    `meta.DefinitionsGeneration` sich bewegt hat oder nach einer Minute; `WatchTypes` hört
    auf `itsm_definitions` (eigene Verbindung, bei Verlust neu und mit einem Neuladen).
  - Migration `00047_definition_notify.sql`: Trigger auf `definition` mit `pg_notify`.
  - `meta.Store` und die Kundenschicht (`internal/customer` Add und Remove) sagen nach dem
    Schreiben `meta.DefinitionsChanged`.
  - `internal/sla`: Kalender je Datenbank und Id zehn Sekunden im Speicher; eine Änderung an
    `core.calendar*` oder einem SLA wirft sie weg (auch auf den anderen Instanzen über den
    vorhandenen Kanal). `internal/calendar`: die Zeiten werden einmal geordnet (`sync.Once`),
    vorher bei jeder Rechnung, was bei einem geteilten Kalender ein gleichzeitiges Schreiben in
    eine Map war (gefunden mit acht parallelen Strömen).
  - `cmd/itsm`: `go rec.WatchTypes`; Profile über `ITSM_PPROF_ADDR` nur auf der Loopback.
- **Belegt:**
  - Massenimport von Störungen (Pakete sd und incident, mit Regeln und SLA): ein Strom 55 auf
    165 Zeilen/s, acht Ströme mit acht Bulk-Arbeitern etwa 800 Zeilen/s, sechzehn nicht mehr
    (620 Zeilen/s, die Grenze ist jetzt die Datenbank: je Zeile eine eigene Transaktion mit
    Verlauf, Ereignis, Suche, Rollen, Nummer und zwei Uhren). Das Ziel LZ5 (5.000 Zeilen/s)
    verfehlen Typen mit Uhren damit weiter; das ist ein Befund von H14.
  - Go `TestTypesFollowDefinitionsOverHTTP`: ein Feld aus dem Typ-Editor ist sofort da, das
    Löschen der Kundenschicht an der API vorbei kommt über die Nachricht binnen 5 s an.
    `TestCalendarSharedBetweenGoroutines`: sechzehn Abläufe rechnen mit demselben neuen
    Kalender ohne Wettlauf. `TestCustomerLayerInsideItsPackage` löscht die Kundenschicht per
    SQL und sagt es jetzt dem Speicher (Grund: die Typen kommen nicht mehr bei jedem Aufruf frisch).
  - `scripts/ci.sh` am 2026-11-13: 18 von 19 Schritten grün. Die Schnellstufe der Lastmessung
    verfehlte nur das feste Ziel LZ5 (3.828 statt 5.000 Zeilen/s), während andere Container den
    Rechner auslasteten; die Basislinie kam im selben Lauf auf 3.358, am Vormittag auf 9.170.
    Im Vergleich mit der Basislinie ist der neue Stand überall gleich oder schneller (p95 LZ2
    6,1 statt 9,9 ms, LZ4 10,9 statt 22,1 ms, LZ16 2,8 statt 7,3 ms, LZ17 11,0 statt 19,1 ms).
- **Nicht geprüft:** Mehrere Instanzen gleichzeitig (die Nachricht ist dieselbe wie für eine);
  ein Kalender, der auf einer anderen Instanz geändert wird, gilt dort sofort, hier über Valkey.

### PL48 — Auswertungen über Mandanten, 2026-10-31

- **Grundlage:** Hauptplan E137, E146; architektur.md §5.1; deine Entscheidungen vom 2026-10-31
  (Recht nur ausdrücklich und nur für die Installation; eigener Endpunkt; Mandant ohne Recht
  überspringen und melden; nur API).
- **Gebaut:**
  - `authz`: `TenantType` (`core.tenant`), `ActionReportTenants` (`report_tenants`) und
    `Access.ReportsAcrossTenants`: wahr nur für ein Konto der Installation (`Home` leer) mit
    einer Berechtigung genau dieser Art und Aktion an der Wurzel (Reichweite all oder subtree)
    und, wenn die Sitzung auf eine Rolle eingeengt ist, in dieser Rolle. Platzhalter zählen nicht.
  - `POST /api/v1/tenants:query` (`internal/httpapi/tenants.go`, OpenAPI `TenantQuery`,
    `TenantRows`): je Mandant `Access.Enter` und dieselbe JSON-Abfrage wie `:query` in dessen
    Datenbank; Zeilen mit der Spalte `tenant`; `skipped` mit `unknown` oder `no_read`;
    `query.after` ergibt 422; eine Liste ohne Grenze bekommt 500. Je ausgewertetem Mandanten
    ein Eintrag `report.tenants` (Kategorie data) in seinem Sicherheitsprotokoll, mit Abfrage,
    Zeilenzahl und der Liste der Mandanten.
  - Typen der Oberfläche neu erzeugt (`web/src/api/schema.d.ts`).
  - Nachtrag (Befund aus dem Test, deine Entscheidung 2026-10-31): Mit einer Datenbank fand
    `POST /api/v1/assignments` die Person eines Admins der Installation nicht (404), wenn seine
    Sitzung in einem Mandanten arbeitete: die Person liegt an der Wurzel, außerhalb des
    Mandanten; mit eigenen Datenbanken ging es über den Spiegel. Jetzt prüft die API die Person
    eines Kontos der Installation zentral (`authz.Service.InstallationPerson`), wenn der Admin
    das Recht auf Rollen an der Wurzel hat; in beiden Betriebsarten gleich.
- **Belegt:**
  - `TestTenantQueryOverHTTP/shared` und `/database` (der Admin gibt sich die Rolle über die
    API aus einer Sitzung im Mandanten): Die eingebaute Rolle admin bekommt 403;
    mit der Rolle `pl48_report` an der Wurzel kommen die Datensätze je Status als
    `root.ta new 2`, `root.tb new 1`; eine Liste von B trägt den Mandanten, `root.tc` steht als
    `unknown` in `skipped`; `after` gibt 422; ein Admin von A bekommt 403; beide Mandanten haben
    den Eintrag in ihrem Protokoll, in ihrer eigenen Datenbank.
  - Der Grenz-Rundgang ruft den neuen Endpunkt als Konto eines Mandanten auf und bleibt grün.
  - Geändert: `TestConsumersKeepTheirPositions` lässt den freigegebenen Abnehmer laufen, bis er
    weiterkommt (bis zu einer Minute), statt nach einem Durchlauf zu prüfen; in der CI war das
    Ereignis der Änderung unter Last noch nicht sichtbar (E87, wie in PL47d und PL47e).
  - Ganze CI grün am 2026-10-31 im zweiten Lauf: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 4,00 ms, LZ2 7,95 ms, LZ3 8,82 ms, LZ4 14,52 ms, LZ5 9.062 Zeilen/s, LZ13 8.958 Zeilen/s. Davor einmal `TestConsumersKeepTheirPositions` (oben).
  - Ganze CI nach dem Nachtrag grün am 2026-10-31 im ersten Lauf: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,95 ms, LZ2 6,66 ms, LZ3 8,78 ms, LZ4 12,42 ms, LZ5 11.870 Zeilen/s, LZ13 9.888 Zeilen/s.
- **Nicht geprüft und Befund:**
  - Keine Oberfläche (E146); Übersetzungen der Feldwerte kommen roh (ohne `Accept-Language`).
  - Wie lange eine Auswertung über viele Mandanten dauert, ist nicht gemessen; die Mandanten
    werden nacheinander abgefragt.

### PL47e — Datenbank je Mandant, Tests, 2026-10-30

- **Grundlage:** Hauptplan E141, E145; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (Rundgang in jeder CI in beiden Betriebsarten; eigene CI-Datenbank und Testreste aufräumen).
- **Gebaut:**
  - `apiServerWith(t, tenantURLs)`: der Testserver der API mit einem Router wie im Binary; alle
    Dienste, Paketinstallation (`installPackage` legt die Tabellen in jeder Datenbank an),
    Jobs, Dateien und die Verteiler der Tests hängen am Router. `apiServer` ist der Fall mit
    einer Datenbank. Die Abnehmer der Tests springen in jeder Datenbank ans Ende, und
    `waitForMail` lässt den Verteiler jeder Datenbank laufen.
  - `TestTenantBoundaryOverHTTP` läuft als `shared` und `database`. Mit eigenen Datenbanken
    entstehen die Mandanten mit `tenant.CreateIn` (leer, ohne Übernahme), die Konten mit
    `CreateAccountIn`; danach derselbe Rundgang über jede Route in beide Richtungen, die
    Nebendaten aus PL45 (Webhooks, Benachrichtigungen, API-Client, Ereignisprotokoll) und die
    Regeln zum Anlegen aus PL46.
  - Neu nur mit eigenen Datenbanken: jeder Datensatz jedes Typs liegt in der Datenbank seines
    Mandanten, in keiner anderen und nicht in der Steuer-Datenbank.
  - CI: eigene Datenbank `itsm_ci` (`ITSM_TEST_DATABASE_URL` in `compose.yaml`); `scripts/ci.sh`
    legt sie an, wenn sie fehlt; die Tests migrieren sie selbst. Die Entwicklungsinstanz auf
    8092 bleibt auf `itsm`, die Befehle aus dem Werkzeug-Container ebenso.
  - Aufgeräumt in `itsm` (einmalig): 6 Mio. erledigte Jobs, rund 960.000 Regel-Jobs aus der
    Laststufe, die die Instanz auf 8092 ununterbrochen abarbeitete, die Abnehmer-Zeilen der Tests
    und die Ereignisse bis auf die letzten 10.000; Datenbank von 25 auf 15 GB. Datensätze und
    Verlauf (`audit_default`, 15 GB) sind geblieben.
  - Nachtrag (deine Entscheidung 2026-10-30): Migration 40 macht die Nummern des
    Sicherheitsprotokolls je Datenbank monoton. Eine neue Nummer ist mindestens die letzte plus
    eins (Tabelle `security_event_last` mit einer Zeile, Funktion `security_event_next_id()`),
    sonst die Zeitnummer aus PL47b; springt die Uhr zurück, bleibt die Reihenfolge richtig.
- **Belegt:**
  - `TestTenantBoundaryOverHTTP/shared` und `/database` grün (gezielt, 5 s und 7 s).
  - `TestIDsStayInOrderWhenTheClockStepsBack`: Die letzte Nummer steht eine Stunde vor der
    Uhr; die nächsten beiden Einträge bekommen sie plus eins und plus zwei und stehen in der
    Liste oben.
  - Geändert: Mit eigenen Datenbanken antwortet das Anlegen eines Datensatzes von A im Knoten
    von B mit 422 („kein Knoten des Baums“) statt 403, weil die Datenbank von A den Knoten von B
    nicht kennt; abgelehnt wird beides, und 422 verrät nicht einmal, dass es den Knoten gibt.
  - Geändert: `TestConsumersKeepTheirPositions` wartet vor dem ersten Durchlauf, bis die
    Ereignisse seiner beiden Datensätze sichtbar sind (bis zu einer Minute); sonst fand der
    Durchlauf unter Last noch nichts und meldete keinen Fehler (E87, wie die Tests in PL47d).
  - Ganze CI grün am 2026-10-30 im dritten Lauf, zum ersten Mal auf `itsm_ci` (vom ersten Lauf frisch angelegt): Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,73 ms, LZ2 5,89 ms, LZ3 7,67 ms, LZ4 14,12 ms, LZ5 13.616 Zeilen/s, LZ13 8.143 Zeilen/s (LZ2 und LZ3 deutlich schneller als auf der aufgeblähten `itsm`). Davor einmal `TestConsumersKeepTheirPositions` (oben) und einmal `TestSecuritySettingsOverHTTP`: der neueste Eintrag des Sicherheitsprotokolls war nicht die Sperre, weil die Uhr der WSL zurücksprang und die zeitgeordnete Nummer (PL47b) kleiner ausfiel; behoben mit monotonen Nummern je Datenbank (deine Entscheidung 2026-10-30, Migration 40, siehe oben); die CI danach steht im folgenden Eintrag.
  - Ganze CI nach Migration 40 grün am 2026-10-30 im ersten Lauf: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 4,05 ms, LZ2 6,78 ms, LZ3 8,58 ms, LZ4 12,18 ms, LZ5 9.842 Zeilen/s, LZ13 8.720 Zeilen/s.
- **Nicht geprüft:**
  - Die Instanz auf 8092 und die CI teilen sich weiter Valkey (Weckrufe, Sperrzähler,
    Rechte-Cache).
  - Der Verlauf in `itsm` (15 GB) ist nicht aufgeräumt.
  - Die übrigen API-Tests laufen nur mit einer Datenbank; mit eigenen Datenbanken laufen der
    Rundgang, `TestControlDatabaseOverHTTP`, `TestBackgroundPerDatabase` und `TestTenantDatabase`.

### PL47d — Datenbank je Mandant, Betrieb, 2026-10-30

- **Grundlage:** Hauptplan E141, E144; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (der Betrieb legt die Datenbank an; streng; Sicherung mit den Werkzeugen von PostgreSQL;
  Webhook der Installation liest alle Datenbanken); mein Vorschlag ohne Einwand: kein Umzug
  bestehender Mandanten in PL47d.
- **Gebaut:**
  - Konfiguration: `ITSM_TENANT_MODE=shared|database` (Vorgabe `shared`),
    `ITSM_TENANT_<NAME>_DATABASE_URL` je Mandant (`platform.TenantDatabaseVar`). Die URL eines
    Mandanten steht nur in der Umgebung, die Steuer-Datenbank kennt nur den Namen.
  - `db.OpenTenants`: öffnet die Datenbank jedes Mandanten im Verzeichnis. Fehlt eine URL
    (`db.ErrNoDatabase`, mit dem Namen der Variable) oder steht eine Datenbank auf einer anderen
    Migration als die Steuer-Datenbank, startet itsm nicht. `db.MigrateTenants` migriert alle.
  - Befehlszeile: `openDatabases` öffnet die Datenbanken für `serve` und alle Befehle (`user`,
    `client`, `ldap`, `calendar`, `package`). `migrate` migriert auch jede Mandanten-Datenbank.
    `package install` legt die Tabellen in jeder Datenbank an und stellt die Neuberechnung je
    Datenbank ein; die Definitionen bleiben zentral. `user create --tenant` legt das Konto in der
    Datenbank des Mandanten an, die Person mit `user grant` ebenso; `user tenant` lehnt mit
    eigenen Datenbanken ab, weil ein Konto seine Datenbank nicht wechselt.
  - `tenant create` mit eigenen Datenbanken (`tenant.CreateIn`): migriert die leere Datenbank,
    legt die Tabellen aller installierten Typen, den Knoten und die Auswahllisten an und trägt
    den Mandanten zuletzt ins Verzeichnis. Bricht es mittendrin ab, kennt die Installation den
    Mandanten noch nicht, und ein zweiter Aufruf macht weiter. Eine Datenbank mit fremden Knoten
    oder Konten wird abgelehnt, `--adopt` auch.
  - `tenant check <name>` (`tenant.Check`): vergleicht Migration, Knoten in beiden Datenbanken,
    Konten mit dem zentralen Namensverzeichnis und Spiegelkonten mit den Konten der Installation.
    Es ändert nichts und endet mit Fehler, wenn etwas nicht passt.
  - Webhooks der Installation über alle Datenbanken: Migration 39 (`webhook_position`, in jeder
    Mandanten-Datenbank die Position eines zentralen Webhooks); der Verteiler eines Mandanten
    reiht die Zustellungen ein, beginnend mit den Ereignissen nach dem Anlegen des Webhooks; die
    Zustellung liest das Ereignis in der Datenbank ihres Jobs; die Liste der Zustellungen mischt
    alle Datenbanken; Löschen entfernt die Positionen.
  - Behoben (Fehler aus PL47b): `records.SeedListsInto` schrieb die Auswahllisten eines Mandanten
    in die Steuer-Datenbank; jetzt in die Datenbank des Mandanten (`Router.BindFor`).
  - Werkzeug-Container: `postgresql-client-18` aus dem Paketarchiv von PostgreSQL, für den Test
    von Sicherung und Wiederherstellung.
  - README: Abschnitt „Datenbank je Mandant“ und die neuen Variablen.
- **Belegt:**
  - `TestTenantDatabase` (eigene Datenbanken):
    - Ohne URL und mit `--adopt` lehnt `CreateIn` ab, ein zweites Mal mit `ErrExists`.
    - Der neue Mandant hat seine Auswahllisten in seiner Datenbank, keine in der Steuer-Datenbank.
    - Ohne URL stoppt `OpenTenants` mit dem Namen der Variable.
    - Ein danach installiertes Paket (probe) legt seine Tabelle auch in der Mandanten-Datenbank an.
    - `pg_dump` sichert die Mandanten-Datenbank; ein Konto, das danach entsteht, fehlt in der
      mit `pg_restore` in eine neue Datenbank wiederhergestellten Fassung, und `tenant.Check`
      nennt genau dieses Konto; Datensatz und älteres Konto sind wieder da.
  - `TestBackgroundPerDatabase` erweitert: Ein Webhook der Installation liegt zentral, bekommt
    je einen Zustelljob in A und B, hat dort je eine Position, zeigt beide Zustellungen in seiner
    Liste und nimmt beim Löschen die Positionen mit.
  - `TestLoadConfigTenantMode`: Vorgabe, Variable je Mandant, unbekannte Betriebsart.
  - Geändert: `TestStreamDispatchAndRetention`, die Ereignis-Hilfe der Webhook-Tests und
    `waitUntil` warten bis zu einer Minute statt 10 bzw. 20 Sekunden, bis ihr Ereignis sichtbar
    ist. Der Horizont gilt für den ganzen Datenbankserver; eine Bulk-Transaktion anderer Tests
    hielt ihn in der CI mit `-race` auf einem ausgelasteten Rechner länger als 10 Sekunden auf
    (in vier Läufen seit PL47c rot). Ein Lauf mit sekündlichem Blick auf `pg_stat_activity`
    fand keine Transaktion aus dem Code über 4 Sekunden; ein grüner Lauf wird nicht langsamer.
  - Durchgang von Hand mit Wegwerf-Datenbanken (`migrate`, `package install`, `tenant create`
    mit und ohne URL, `user create --tenant`, `user tenant`, `tenant check`, `pg_dump` und
    `pg_restore`, `serve` ohne und mit URL): alles wie erwartet; `serve` meldet zwei Datenbanken
    für Jobs und Verteiler.
  - Ganze CI grün am 2026-10-30 im dritten Lauf, in der WSL-Kopie: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 4,92 ms, LZ2 10,79 ms, LZ3 18,22 ms, LZ4 15,39 ms, LZ5 7.795 Zeilen/s, LZ13 5.695 Zeilen/s (der Rechner war durch glpi-perf ausgelastet). Davor einmal Lint (gosec G602, Fehlalarm an `args[1]`), einmal die drei Ereignis-Tests oben.
- **Nicht geprüft:**
  - `serve` mit `ITSM_TENANT_MODE=database` im Container (der Start wird über `OpenTenants`
    getestet, nicht über den ganzen Prozess); folgt mit PL47e.
  - Umzug eines bestehenden Mandanten in eine eigene Datenbank: nicht gebaut (spätere Aufgabe).
  - Ein Mandant, der nach dem Start angelegt wird, braucht einen Neustart der Instanzen.
  - Ist `ITSM_TENANT_<NAME>_DATABASE_URL` gesetzt, aber die Betriebsart `shared`, wird die
    Variable still übergangen.

### PL47c — Datenbank je Mandant, Hintergrund, 2026-10-30

- **Grundlage:** Hauptplan E141, E143; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (Jobansicht je Datenbank; Zeitpläne zentral für alle; wiederkehrende Jobs je Datenbank, nur der
  LDAP-Abgleich zentral).
- **Gebaut:**
  - `db.Database` mit `Name` und `Bind`: Eine an eine Datenbank gebundene Arbeit (Job,
    Verteiler) bekommt von `Router.Tenant` diese Datenbank, was die Sitzung auch sagt.
    `Router.Databases()` zählt jede Datenbank einmal mit Namen auf (leer für die
    Steuer-Datenbank), `Router.Current(ctx)` nennt die Datenbank einer Anfrage.
  - `jobs`: ein River-Client je Datenbank. Eine Middleware bindet jeden Job an die Datenbank
    seines Clients. Einreihen, Liste, Abbrechen und Wiederholen gehen an den Client der Datenbank,
    in der die Anfrage arbeitet; `EnqueueTx` schreibt wie bisher in die Transaktion der Änderung.
    Wiederkehrende Jobs stehen in jedem Client, ein Job mit `Periodic.Central` nur in der
    Steuer-Datenbank. Die Überschreibungen der Zeitpläne liest und schreibt der Dienst zentral.
  - `events`: ein Verteiler je Datenbank mit eigenen Positionen, gebunden an seine Datenbank. Das
    Wecksignal nach einem Schreiben weckt nur den Verteiler der Datenbank, in die geschrieben
    wurde (Kanal `events.wake.<Mandant>`, die Steuer-Datenbank behält `events.wake`).
  - Regeln nach Zeitplan: Der Minutenjob einer Datenbank führt die Regeln aus, die in ihr liegen.
    Eine Regel der Installation liegt zentral, sucht ihre Datensätze in jeder Datenbank und gilt
    danach einmal als gelaufen.
  - Aufbewahrung des Ereignisprotokolls: Der nächtliche Job räumt nur die eigene Datenbank auf;
    er läuft ja in jeder.
  - LDAP-Abgleich: `Central`.
  - Webhooks: Die Liste der Zustellungen liest die Jobs der Datenbank des Abonnements.
  - Leistungsgate: `measure.Best` lässt Läufe ohne erfolgreiche Anfrage weg (E76 ergänzt).
- **Belegt:**
  - `TestBackgroundPerDatabase` (drei eigene Datenbanken):
    - Ein Job, der an B gebunden eingereiht wird, liegt und läuft in B; einer aus einer Anfrage
      in A liegt und läuft in A.
    - Die Jobliste von A zeigt ihn, die der Wurzel nicht.
    - Jede Datenbank bekommt ihren Verteiler.
    - Ein neuer Datensatz in A erzeugt den Zustelljob für den Webhook von A in A, keinen in der
      Steuer-Datenbank oder in B.
    - Eine Regel der Installation nach Zeitplan läuft nicht aus den Jobs von A und B, sondern
      aus der Steuer-Datenbank, setzt den Wert in beiden Mandanten und läuft in derselben Minute
      kein zweites Mal.
  - `TestRouter`: `Databases`, `Bind`, `Current`.
  - `TestBest`: Läufe ohne erfolgreiche Anfrage zählen nicht.
  - Geändert: `jobs_test.go` hört die erledigten Jobs am Client der Steuer-Datenbank ab
    (`s.clients[""]` statt `s.client`), weil es jetzt einen Client je Datenbank gibt.
  - Ganze CI grün am 2026-10-30 im vierten Lauf, in der WSL-Kopie: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,44 ms, LZ2 7,82 ms, LZ3 14,64 ms, LZ4 12,08 ms, LZ5 8.260 Zeilen/s, LZ13 7.780 Zeilen/s. Davor dreimal rot in Tests, die PL47c nicht berührt, während fremde Container den Rechner auslasteten (glpi-perf-db bei 78 % CPU, ein Supabase-Stapel, Last um 7): zweimal fanden `TestFanOutFiltersAndRights` und `TestStreamDispatchAndRetention` ihr Ereignis nicht binnen 10 s, einmal brach der Bulk-Job in `TestBulkStreamOverHTTP` nach einer Minute ab. Allein liefen die Tests je fünfmal grün; ein Lauf aller Go-Tests mit sekündlichem Blick auf `pg_stat_activity` fand keine Transaktion aus dem Code über 3 s.
- **Nicht geprüft:**
  - Benachrichtigungen per Mail mit eigenen Datenbanken. Sie laufen über denselben Verteiler wie
    die Webhooks, ein eigener Test fehlt.
  - Ein Webhook der Installation (an der Wurzel angelegt) liegt zentral und sieht deshalb mit
    eigenen Datenbanken nur die Ereignisse der Steuer-Datenbank, nicht die der Mandanten. Mit
    einer Datenbank sieht er alle, die sein Besitzer lesen darf. Entschieden und gebaut mit PL47d
    (E144): er liest alle Datenbanken.
  - Die Rechnung „ein Verteiler je Datenbank“ für viele Mandanten (Verbindungen, Abfragen je
    Weckruf) ist nicht gemessen.
  - Betrieb, Kommandozeile und Anlegen eines Mandanten mit eigener Datenbank kommen mit PL47d,
    die Rundgänge in beiden Betriebsarten mit PL47e.

### PL47b — Datenbank je Mandant, Steuer-Datenbank, 2026-10-30

- **Grundlage:** Hauptplan E141, E142; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (Anmelderegeln und Identitäten zentral; eine Rolle in Gebrauch wird nicht gelöscht; der
  Spiegel gleicht sich bei jedem Betreten an; Namen zentral zuerst reservieren und bei einem
  Fehler zurückgeben; alle Zuweisungen der Installationskonten zentral).
- **Gebaut:**
  - Migration 38: Tabelle `account_home` (Konto → Name → Mandant; nur bei eigenen
    Datenbanken gepflegt), `identity.tenant`. Die Fremdschlüssel `role_assignment.role_id`,
    `role_assignment.rule_id` und `identity.account_id` fallen weg, weil sie über die Grenze
    zeigen; ihre Kaskaden übernimmt der Code. Die Nummern des Ereignisprotokolls sind zeitgeordnet
    (Mikrosekunden × 1024 plus Zufall), damit sich Einträge aus zwei Datenbanken nach Nummer
    mischen und blättern lassen.
  - `db/accounts.go`:
    - `Router.Account(id)` und `Router.User(name)` finden die Heimat-Datenbank eines Kontos.
    - `Reserve` reserviert den Namen zentral, `Release` gibt ihn bei einem Fehler zurück. Eine
      Reservierung ohne Konto, die älter als zehn Minuten ist, darf ein neues Konto übernehmen.
    - `Taken` prüft, ob ein Name irgendwo vergeben ist.
    - Mit einer Datenbank macht keiner dieser Aufrufe eine Abfrage.
  - `auth`: Mit `CreateAccountIn(tenant, …)` entsteht ein Konto gleich in seinem Mandanten.
    Anmeldung, Kennwort und Sperre arbeiten in der Heimat-Datenbank; `ListAccounts` liest alle
    Datenbanken.
  - `authz`:
    - Das Konto, seine Person, Gruppen und Zuweisungen kommen aus der Heimat-Datenbank, die
      Rechte der Rollen aus der Steuer-Datenbank (`AssignmentsOfHolders`, `PermissionsOfRoles`).
    - Eine Zuweisung an die Person eines Installationskontos liegt zentral, alle anderen beim
      Mandanten. `Assignments` ergänzt die Rollennamen aus der Steuer-Datenbank.
    - `DeleteRole` zählt die Zuweisungen in jeder Datenbank und lehnt mit 409 ab
      (`authz.ErrInUse`).
  - `authn`:
    - Anmelderegeln liegen zentral. Löscht man eine Regel, löscht das auch ihre Zuweisungen in
      der Datenbank ihres Knotens.
    - Identitäten liegen zentral und tragen ihren Mandanten.
    - Ein neues OIDC-Konto und das Konto eines API-Clients entstehen in ihrer
      Mandanten-Datenbank, der Name wird vorher reserviert.
    - Clients findet der Code über den Namen ihres Kontos.
  - `authn.Mirror` und `records.Mirror`: Spiegelkonto ohne Kennwort und Spiegelperson unter
    denselben Kennungen. Die Middleware zieht sie nach, wenn die Sitzung einen Mandanten betritt
    (`Session.Entered`): nach der Anmeldung und nach jedem Wechsel.
  - `org`: Ein Knoten lebt in der Datenbank seines Mandanten (`For(path)`). Wurzel und
    Mandantenliste liegen zentral. Einstellungen der Wurzel gelten im Mandanten, wo dieser
    nichts setzt.
  - Regel-Engine: Sie liest alle Datenbanken. Eine Regel wird in der Datenbank ihres Mandanten
    geschrieben, eine Regel der Installation zentral.
  - Ereignisprotokoll: Ein Eintrag geht in die Datenbank seines Mandanten, Einträge der
    Installation gehen zentral. Die Liste mischt beide; der Ablauf der Aufbewahrungsfrist läuft
    über alle Datenbanken.
  - Webhooks: Die Zustellung liest Abo, Ereignis und Fehler in der Datenbank des Mandanten aus
    den Job-Argumenten.
  - Einstellungen eines Kontos liegen in seiner Heimat-Datenbank.
- **Belegt:**
  - `httpapi.TestControlDatabaseOverHTTP` mit drei eigenen Datenbanken (Steuer, A, B), jeweils
    direkt in der Datenbank geprüft:
    - **Konten:** Jedes Konto liegt nur in seiner Datenbank, das Verzeichnis kennt alle.
      `ctl_anna` ein zweites Mal in B wird abgelehnt und nicht geschrieben.
    - **Anmeldung:** Anna, Ben und der Admin melden sich an. Der Admin landet in A und bekommt
      dort Spiegelkonto und Spiegelperson, in B erst nach dem Wechsel.
    - **Regeln:** Eine Regel der Installation liegt zentral und setzt die Lösung beim Anlegen in
      A und in B.
    - **Datensätze:** Sie liegen in ihrer Mandanten-Datenbank, keiner in der Steuer-Datenbank.
    - **Spiegel:** Der Admin trägt sich in A als Bearbeiter ein, 201.
    - **Rollen:** Eine Zuweisung in A liegt in A. Die Rolle zu löschen gibt 409, ohne die
      Zuweisung 204.
    - **Protokoll:** Annas Anmeldung liegt in A, die des Admins zentral. Der Admin sieht in A
      beide, Ben nicht. Die Nummern sind zeitgeordnet.
  - Angepasst: 21 Tests räumten alte Rollen auf und verließen sich darauf, dass die Datenbank
    deren Zuweisungen mitlöscht. Sie rufen jetzt `dropRole` auf (erst die Zuweisungen, dann die
    Rolle), weil eine Rolle in Gebrauch abgelehnt wird. `actors_test` löscht seine Rolle am Ende
    ebenso.
  - Ganze CI grün am 2026-10-30 im dritten Lauf, in der WSL-Kopie: Go und Web (56 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,33 ms, LZ2 7,70 ms, LZ3 13,84 ms, LZ4 10,95 ms, LZ5 9.488 Zeilen/s, LZ13 7.259 Zeilen/s. Davor: einmal `TestRightsScopesOverHTTP`, weil `DeleteRole` die eingebaute Rolle admin erst nach der Zählung der Zuweisungen abwies (409 statt 422; jetzt zuerst die Prüfung auf admin); einmal das Leistungsgate an LP2, weil ein Lauf der Basislinie während eines Uhrensprungs der WSL 0 Anfragen maß und das Gate den besten Lauf (0,00 ms) als Vergleich nahm; seitdem lässt `measure.Best` Läufe ohne erfolgreiche Anfrage weg (E76 ergänzt, Test `TestBest`).
- **Nachtrag 2026-11-03 (Befund aus dem Test von PL49, deine Entscheidung):** Mehrere Anfragen
  eines Installations-Admins gleich nach dem Wechsel in einen Mandanten mit eigener Datenbank
  legten Spiegelperson und Spiegelkonto zugleich an; die zweite scheiterte an der Eindeutigkeit
  (500 beim Konto, 409 bei der Person). Der Browser schickt nach dem Wechsel genau solche
  parallelen Anfragen.
  - Gebaut: `authn.Mirror` versucht es nach so einem Zusammenstoß einmal neu und findet den
    Spiegel dann vor. Eine Postgres-Sperre je Konto war der erste Ansatz; sie hielt eine
    Verbindung des Pools beim Warten und blockierte bei 30 Anfragen den Pool (im Test gesehen,
    verworfen).
  - Belegt: `TestMirrorAtOnceOverHTTP` (eigene Datenbanken): dreimal Wechsel A, B, A mit je 30
    Anfragen zugleich, alle 200, ein Spiegelkonto in A; ohne die Wiederholung scheiterten 22 von
    30. Dreimal hintereinander grün.
  - Ebenfalls am 2026-11-03 (Befund aus UI34): der Feldname `tenant` ist reserviert wie die
    anderen Systemspalten (`meta/validate.go`); `TestValidateNamesFieldAndReason` prüft ihn mit, dazu die Regeln des Feldtyps `sequence`.
  - Ganze CI am 2026-11-03 grün im ersten Lauf (9:33 min): alle 19 Schritte, 90 Vitest-Fälle,
    17 Browser-Tests, LZ1 p95 3,97 ms, LZ13 8.092 Zeilen/s.
- **Nicht geprüft:**
  - Gruppen eines Admins in einem Mandanten bringen ihm keine Rechte. Seine Rechte kommen nur
    aus direkten Zuweisungen; seine Gruppen und Gruppenkollegen liest der Code zentral.
  - Kommandozeile und `tenant.Create`/`tenant.Bind` arbeiten weiter mit einer Datenbank.
    `itsm user create --tenant` und `itsm user tenant` kommen mit PL47d auf mehrere Datenbanken.
    Ein Konto kann seine Datenbank nicht wechseln.
  - Jobs, Verteiler und Zeitpläne laufen noch an der Steuer-Datenbank (erledigt mit PL47c). Damit auch:
    Regeln nach dem Schreiben, Benachrichtigungen, Webhook-Zustellung und LDAP-Abgleich bei
    eigenen Datenbanken.
  - Eine Reservierung bleibt stehen, wenn der Prozess zwischen den beiden Schreibvorgängen
    abbricht. Sie wird erst beim nächsten Anlegen desselben Namens nach zehn Minuten frei. Einen
    eigenen Aufräumbefehl gibt es nicht.
  - Die Rundgänge aus PL44 bis PL46 laufen erst mit PL47e auch mit eigenen Datenbanken.

### PL47a — Datenbank je Mandant, Naht, 2026-10-30

- **Grundlage:** Hauptplan E137, E141; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (PL47 ganz bauen; Zugangsdaten nur in der Konfiguration; Installation zentral; Hintergrund je
  Mandant im selben Prozess; Bauplan PL47a bis PL47e freigegeben; Spiegelkonto für Admins).
- **Gebaut:**
  - `internal/db/router.go`: `Router` mit `Control()` (Datenbank der Installation),
    `Tenant(ctx)` (Datenbank des Mandanten der Anfrage), `For(tenant)` (ein Knoten unter dem
    Mandanten zählt als der Mandant; Wurzel und unbekannte Mandanten gehen an die
    Steuer-Datenbank), `Separate()`, `Pools()`. `Single(pool)` ist eine Datenbank für alles,
    `NewRouter` die Betriebsart mit eigenen Datenbanken (verdrahtet mit PL47d).
  - 17 Dienste halten statt eines Pools und einer festen `queries.Queries` einen Router. Gewählt
    wird je Zugriff:
    - **Steuer-Datenbank:** Definitionen und Typen, Kundenschicht, Merkmale, Rollen und
      Rechte, die Liste der Mandanten (`ListTenants`, `TenantPaths`), Beziehungsarten.
    - **Mandanten-Datenbank:** alles andere, über die Hilfen `tq(ctx)` und `cq()` in `auth`,
      `authz`, `files`, `notify`, `org`, `prefs`, `engine`, `seclog`, `webhooks`.
  - Die Schemaänderung der Kundenschicht läuft über `Pools()` in jeder Datenbank.
  - River bleibt bis PL47c an der Steuer-Datenbank.
  - Kommandozeile und `internal/tenant` reichen ihren Pool als `db.Single(pool)` weiter, bis
    PL47d sie auf mehrere Datenbanken stellt; 24 Testdateien ebenso.
- **Belegt:**
  - `db.TestRouter`: mit zwei Mandanten-Datenbanken wählt der Router für `root.a`,
    `root.a.x.y`, `root.b`, die Wurzel, einen leeren und einen unbekannten Mandanten richtig;
    `Single` antwortet überall mit sich.
  - `db.TestNoServiceHoldsAPool` durchsucht `internal/` nach Feldern vom Typ `*pgxpool.Pool`
    oder `*queries.Queries` außerhalb von `internal/db`; mit einer eingeschmuggelten Datei schlug
    er an, ohne sie ist er grün.
  - Ganze CI grün am 2026-10-30 im zweiten Lauf, in der WSL-Kopie: Go und Web (56
    Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,16 ms, LZ13
    7.810 Zeilen/s, also wie vor dem Umbau. Davor ein Lint-Hinweis (ST1023 in
    `jobs/retention.go`).
- **Nicht geprüft:**
  - Tabellen, deren Zeilen teils der Installation und teils einem Mandanten gehören (Konten,
    Identitäten, Regeln, API-Clients, Protokoll, Baum und Einstellungen, Zuweisungen), gehen
    noch alle an die Mandanten-Datenbank; die Aufteilung kommt mit PL47b. `GrantsForPerson`
    verbindet Zuweisungen mit Rollen und Rechten in einer Abfrage und muss dafür geteilt werden.
  - Funktionen mit einem Pool als Argument (Migration, `schema.Apply`, `meta.Store`,
    `tenant.Create`) wählen ihre Datenbank selbst; das ordnet PL47d.

### PL46 — Anlegen und Eindeutigkeit je Mandant, 2026-10-30

- **Grundlage:** Hauptplan E136, E140; architektur.md §5.1; deine Entscheidungen vom 2026-10-30
  (Standardknoten Kontext → Person → einziges Anlegerecht; Feld „Ablegen in“ im Formular;
  eindeutige Felder und externe Schlüssel immer je Mandant; Hilfsspalte `tenant` per Trigger
  statt Umbau der Indexe beim ersten Mandanten).
- **Gebaut:**
  - Migration 37: Funktion `tenant_of(ltree)` (der Mandant eines Knotens, sonst `root`),
    Trigger `set_tenant`, Spalte `tenant` an `list_entry` und jeder bestehenden Typtabelle; die
    Listen-Indexe aus PL45 gehen von `subltree` auf die Spalte. `schema.Apply` legt Spalte und
    Trigger für jede Typtabelle an und baut eindeutige Indexe ohne `tenant` um (`ensureUnique`,
    `ensureTrigger`); neue Tabellen haben 14 Systemspalten.
  - Eindeutige Felder: Index `(tenant, feld)` außerhalb des Papierkorbs; externer Schlüssel:
    `(tenant, source, external_key)`. Die Schlüsselsuche von `Sync` und der Stapel bleibt im
    Mandanten des Knotens der Zeile, sonst der Sitzung (`tenant = tenant_of($n)`).
  - Standardknoten (`authz.Access.CreateNode`, `CreateReach`; `PersonNode` kommt beim Laden der
    Rechte aus `rec_core_person`): der Rechtekontext (PL10); sonst der Knoten der Person, wenn
    sie dort anlegen darf; sonst der einzige Knoten mit Anlegerecht; bei mehreren 422 am Feld
    `org_path`, nie stillschweigend der Mandant (E136). `records.createNode` gilt für Anlegen,
    Sync und Stapel.
  - `POST …:field-states` ohne `id` liefert `node` (die Vorgabe, leer bei Wahl) und `nodes`
    (alle erlaubten Knoten mit Beschriftung). Das Anlegeformular zeigt bei mehr als einem Knoten
    das Pflichtfeld „Ablegen in“ (`form.node`) und schickt `org_path`; Formulare des Portals
    bleiben bei ihrem Knoten. `probe.probe` hat das eindeutige Feld `serial`.
  - Befund aus dem ersten CI-Lauf, behoben: `ListAudit` sortierte nach `at`; springt die Uhr
    zurück (WSL meldet „Time jumped backwards“, in der Test-Datenbank lagen bei vier Datensätzen
    Zeilen der Version 1 hinter höheren Versionen), stand eine Zeile der Version 1 am Ende und
    `stateAt` brach ab, bevor es etwas zurücknahm (`TestRolesAtTheRecord`, Version 1 zeigte den
    Stand von Version 3). Jetzt `ORDER BY version, at, id`; das trifft jede VM mit Zeitkorrektur.
- **Belegt:**
  - `authz.TestCreateNode`: acht Fälle (ein Unterbaum; Person innen und außen; zwei Knoten →
    Wahl; innere Reichweite verschluckt; Kontext; Rolle an der Wurzel im Mandanten; kein Recht).
  - `schema`: der Trigger zählt als Änderung, die Spalte `tenant` ist da, der Index gilt je
    Mandant.
  - `httpapi.TestTenantBoundaryOverHTTP` → `tenantCreateRules`: dieselbe Seriennummer in A und B
    je 201, zweimal in A 409; derselbe externe Schlüssel aus derselben Quelle macht je Mandant
    einen Datensatz; eine Person von A mit Rolle nur in `root.ta.unit` legt ohne Angabe dort an
    (vorher 403) und `field-states` nennt den Knoten; mit zweitem Knoten `root.ta.zwei` zwei
    Knoten und `node` leer, Anlegen ohne Angabe 422, mit Angabe 201.
  - Web: `Form.test` wählt bei zwei Knoten „Polizei Süd“ und schickt `org_path` (mit axe).
  - Angepasst: `TestRightsScopesOverHTTP` erwartete 403, wenn eine Person mit Rolle nur in
    `root.rights` ohne Knoten anlegt (der Datensatz landete an der Wurzel). Seit PL46 landet er
    in `root.rights`; der Test nennt für 403 jetzt die Wurzel ausdrücklich und prüft am Ende das
    neue Verhalten.
  - Ganze CI grün am 2026-10-30 im vierten Lauf, in der WSL-Kopie: Go und Web (56
    Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,26 ms, LZ13
    7.938 Zeilen/s. Davor: erster Lauf der alte Rechte-Test und `ListAudit` (oben); zweiter Lauf
    ein Stream-Job, dessen Kontext 3 ms nach dem Start abgelaufen war, zwischen zwei Sprüngen
    der WSL-Uhr; dritter Lauf alles grün, aber das Leistungsgate verweigerte die Messung (Last
    10 bis 13 durch eine parallele GLPI-Leistungsmessung).
  - Prüfweg mit eigener Datenbank auf 8093: SN-1 in Nord und Süd je 201, zweimal in Nord 409;
    `udo` mit Rolle nur in `root.nord.revier` legt ohne Angabe dort an, `tenant` = `root.nord`;
    `chef` bekommt beide Knoten von Nord angeboten.
- **Nicht geprüft:**
  - Der Knoten der Person steht im Rechte-Cache; zieht die Person um, gilt der neue Knoten nach
    bis zu 5 s (Hauptplan E83). `core.person` löst keine Invalidierung aus, damit ein
    LDAP-Abgleich keinen Sturm erzeugt.
  - Bedingungen an Rechten (CEL) zählen bei der Knotenwahl nicht: Ein Knoten kann angeboten
    werden, in dem die Bedingung den Datensatz dann ablehnt (403 beim Speichern).
  - Datensätze an der Wurzel (die Personen der Installations-Admins) bilden den Bereich `root`.
  - Kommandozeile und Jobs ohne Sitzung (`authz.System()`) legen ohne Knoten weiter an der
    Wurzel an.
  - Der Trigger kostet je Einfügen einen Schlüsselzugriff auf `org_node`.

### PL45 — Nebendaten je Mandant, 2026-10-30

- **Grundlage:** Hauptplan E136, E139; architektur.md §5.1; E62 dieses Plans; deine
  Entscheidungen vom 2026-10-30 (Listen als Kopie je Mandant; Regeln, Webhooks, Clients,
  Verzeichnisse und Anmelderegeln je Mandant, Regeln auch der Installation; Protokoll je Mandant
  plus Installation; Sicherheitseinstellungen bleiben an der Wurzel).
- **Gebaut:** Migration `00036_tenant_side.sql`:
  - **Regeln:** `rule.tenant`, leer bei einer Regel der Installation; der Name ist je Mandant
    eindeutig.
    - Eine Regel wirkt nur auf Datensätze ihres Mandanten, vorher wie nachher. Zeitregeln suchen
      nur dort.
    - `create` legt im Mandanten des Datensatzes an.
    - `PUT /rules/{name}` mit `installation` macht eine Regel der Installation; nur mit dem Recht
      an der Wurzel. Die Liste zeigt die eigenen Regeln und die der Installation.
  - **Webhooks:** `webhook.tenant`. Die Lieferung geht mit dem Zugriff des Besitzers in diesem
    Mandanten, Liste und Einzelabruf nur im eigenen.
  - **Ereignisse:** Lesen auf `core.event` endet an der Grenze. Das betrifft Liste, SSE und
    Webhooks; neu ist dafür `records.EventPath`.
  - **Fehlerliste des Verteilers:** `delivery_failure.org_path` stammt aus dem Ereignis. Die
    Abnehmer sieht in einem Mandanten nur, wer das Recht an der Wurzel hat.
  - **Benachrichtigungen:** Sie wirken nur auf Ereignisse ihres Mandanten. Empfänger aus einem
    anderen Mandanten fallen weg, auch die einer Regel.
  - **Jobs:** Export und Massenlauf tragen den Mandanten in den Argumenten und laufen dort.
    `/jobs` zeigt die Jobs des Mandanten, die des Systems nur mit dem Recht an der Wurzel.
  - **Ereignisprotokoll:** `security_event.tenant`. Eine Anmeldung gehört zum Mandanten des
    Kontos, eine Änderung zum Mandanten der Sitzung. Rollen, Berechtigungen und die Wurzel gehören
    zur Installation.
  - **Anmeldung:**
    - Ein Konto aus OIDC gehört zum Mandanten der ersten passenden Anmelderegel; seine Person
      entsteht oder wird dort gefunden.
    - Für ein Mandantenkonto zählen nur die Anmelderegeln seines Mandanten.
    - Anmelderegeln und Verzeichnisse gibt es nur im eigenen Mandanten. Ohne Knoten nehmen sie
      den Mandanten.
    - Ein API-Client gehört dem Mandanten, in dem er angelegt wurde; sein Konto trägt ihn.
  - **Auswahllisten (V28):** Der Schlüssel ist je Mandant eindeutig. `SeedLists` legt die
    Vorgaben in jedem Mandanten an. Ein neuer Mandant bekommt sie sofort (`SeedListsInto`), und
    `--adopt` gibt die vorhandenen dem ersten. Die Ausnahme „Listen an der Wurzel“ aus PL44
    entfällt.
  - **Neue Hilfen:** `org.TenantOf` und `org.TenantPaths`, `authz.SystemIn` (alle Rechte, nur in
    einem Mandanten).
- **Belegt, 2026-10-30:**
  - Einzeln (`internal/authz`): Listen nur im eigenen Mandanten; `SystemIn`.
  - Über HTTP (`TestTenantBoundaryOverHTTP`, eigene Datenbank):
    - Der Rundgang über jede Route prüft jetzt auch Ereignisse, Jobs, Protokoll, Webhooks,
      Dateien, Clients, Verzeichnisse und Anmelderegeln hart, ohne Ausnahmeliste.
    - Jeder Mandant hat seine eigene Kopie von `probe.color`.
    - Eine Regel von B setzt nur in B, die gleichnamige von A nur in A. Eine Regel der
      Installation wirkt in beiden; A darf keine anlegen und B keine ändern.
    - Der Webhook von B bekommt kein Ereignis aus A, der von A bekommt es.
    - Die Benachrichtigung von B schickt nur für B eine Mail. Eine Person von B, die A nennt,
      bekommt nichts.
    - Ein API-Client von A ist für B 404, und sein Token liest nur A.
    - B sieht die Mandantenwechsel des Admins nicht im Protokoll, der Admin sieht sie.
  - Ganze CI grün am 2026-10-30 im ersten Lauf, in der WSL-Kopie: Go und Web (55 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,04 ms, LZ13 8.773 Zeilen/s. Prüfweg mit eigener Datenbank: `--adopt` übernimmt 6 Listeneinträge und eine Person, der zweite Mandant bekommt 6 Einträge als Kopie.
- **Nicht geprüft:**
  - Die Ereignisse eines Mandanten laufen weiter über einen gemeinsamen Valkey-Kanal und werden
    erst je Abonnent gefiltert. Die IP-Sperre nach Fehlversuchen gilt über Mandanten hinweg.
    Beides gibt keine Daten preis.
  - Die Jobs der Regeln, Benachrichtigungen und LDAP-Läufe tragen keinen Mandanten. In einem
    Mandanten sieht sie nur, wer das Recht an der Wurzel hat.
  - `itsm client create` auf der Kommandozeile macht einen Client der Installation; es gibt noch
    kein `--tenant`.
  - Eine Benachrichtigung von A, die eine Person von B nennt, wird beim Speichern nicht
    abgelehnt, sondern bekommt beim Versand niemanden.
  - Kalender, SLAs und Formulare sind Datensätze und gehören ohnehin einem Mandanten. Vorgaben
    für sie aus Paketen gibt es noch nicht.
  - Behoben mit PL46: externe Schlüssel und eindeutige Felder gelten je Mandant. Die
    Listen-Indexe über `subltree` trennten ohne Mandanten die Knoten unter der Wurzel; sie
    nutzen jetzt die Spalte `tenant`.

### PL44 — Mandantengrenze, 2026-10-30

- **Grundlage:** Hauptplan E136, E138; architektur.md §5.1; E61 dieses Plans; deine
  Entscheidungen vom 2026-10-30 (die Wurzel ist der einzige Mandant bis `--adopt`; Spalte am
  Konto; V25 und V27 wie in der Beta; PL44 Datensätze und Verwaltung, der Rest mit PL45).
- **Gebaut:**
  - Migration `00035_tenant.sql`: `org_node.tenant` (nur direkt unter der Wurzel) und
    `account.tenant` (Verweis auf den Mandantenknoten).
  - `internal/authz`: die Grenze in `Filter` und `Permits` vor jeder Reichweite, auch `all`;
    eine Rolle an einem Knoten eines anderen Mandanten zählt nicht, eine an der Wurzel in jedem.
    `Enter` wählt den Mandanten der Anfrage, `Installation` prüft Rechte aus einer Rolle an der
    Wurzel, `Tenants` und `Home` stehen im Rechte-Cache. Auswahllisten an der Wurzel lesen alle
    Mandanten, bis PL45 sie kopiert.
  - `internal/tenant` und `records.MoveIntoTenant`: `itsm tenant create <name> [--label …]
    [--adopt [--admin <konto>]…]`, `itsm tenant list`; `itsm user create --tenant`,
    `itsm user tenant <konto> <mandant|->`; `itsm user grant` ohne `--node` gibt die Rolle im
    Mandanten des Kontos.
  - `internal/org`: sobald es einen Mandanten gibt, entstehen direkt unter der Wurzel nur noch
    Mandanten; ein Mandant mit Konten lässt sich nicht löschen.
  - API:
    - Die Sitzung trägt den Mandanten. `POST /api/v1/auth/context` mit `tenant` wechselt, Rolle
      und Knoten beginnen dann leer. Der Wechsel steht im Sicherheitsprotokoll (`tenant`), der
      letzte gilt bei der nächsten Anmeldung. `auth/me` nennt `tenant` und `tenants`.
    - Baum, Einstellungen, Zuweisungen und Gruppenmitglieder gibt es nur im eigenen Mandanten,
      sonst 404.
    - Rollen und Kundenschicht ändert nur, wer das Recht an der Wurzel hat.
    - Ein neuer Datensatz ohne Knoten landet im Mandanten statt an der Wurzel.
    - Gruppenmitglieder und das Verlassen der Präsenz prüfen jetzt den Datensatz; beide Lücken
      hat der Rundgang des Tests gefunden.
  - Oberfläche: die Auswahl „Mandant“ im Kopf erscheint für Konten mit mehreren Mandanten. Nach
    dem Wechsel beginnt sie auf der Startseite, ohne Daten des alten Mandanten im Cache.
- **Belegt, 2026-10-30:**
  - Einzeln (`internal/authz`):
    - die Grenze mit `all`;
    - die Rolle eines anderen Mandanten wirkungslos, die Rolle an der Wurzel in jedem Mandanten;
    - Auswahllisten;
    - ohne Mandanten keine Grenze;
    - `root.NONE` für ein Konto ohne Mandanten;
    - welche Mandanten ein Konto betreten darf.
  - Über HTTP (`TestTenantBoundaryOverHTTP`, eigene Datenbank):
    - Der erste Mandant übernimmt; Datensatz und Knoten von vorher liegen danach in ihm.
    - Zwei Mandanten haben je ein Konto mit der Rolle `admin` am Mandantenknoten und je einen
      Datensatz jedes Typs aller Pakete.
    - Ein Rundgang ruft jede Route der API mit den Kennungen des anderen Mandanten auf, die
      Datensatz-Routen mit jedem Typ. Keine Antwort zeigt eine dieser Kennungen, jeder
      Lesezugriff ist 404, nichts gelingt, und bei A ist danach nichts verändert.
    - Auswahllisten an der Wurzel lesen beide.
    - Ein Mandantenkonto kann nicht wechseln, keine Rollen und Typen ändern, nicht an der Wurzel
      anlegen und keine Zuweisung über die Grenze machen.
    - Der Admin sieht einen Mandanten zur Zeit und wechselt. Der Wechsel steht im
      Sicherheitsprotokoll, die nächste Sitzung beginnt dort, und Rollen ändert er in jedem
      Mandanten.
  - Vitest (`app/TenantSwitch.test.tsx`): Wechsel im Kopf, danach die Startseite, axe; mit nur
    einem Mandanten keine Auswahl.
  - Ganze CI grün am 2026-10-30 im achten Lauf, in der WSL-Kopie: Go und Web (55 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,05 ms, LZ13 7.835 Zeilen/s. Davor: zweimal Lint (`%w`, `GetSpec`); viermal die zeitkritischen Tests von `events` und `webhooks` sowie LZ13, weil WSL zwischen den Läufen herunterfuhr und Docker mit ihm, sodass jeder Lauf kalt und ohne Entwicklungscontainer begann (behoben mit einem wachhaltenden WSL-Prozess); einmal Kontrast in drei Playwright-Fällen, weil an der Wurzel der gemeinsamen Datenbank `ui.theme = "probe"` stand und das Grün dieses Themes zu hell war (`web/src/ui/tokens.css`, jetzt `oklch(0.45 0.14 150)`).
- **Nicht geprüft:**
  - Ereignisprotokoll (`/api/v1/audit/*`) und Ereignisstrom (`/api/v1/events`) zeigen noch
    Einträge anderer Mandanten. Der Rundgang meldet das als PL45; PL45 schließt es zusammen mit
    Jobs, Webhooks, Dateien, API-Clients, Verzeichnissen, Anmelderegeln und Valkey-Schlüsseln.
  - Startet ein Konto der Installation einen Export oder eine Massenaktion als Job, arbeitet der
    Job im ersten seiner Mandanten statt im gewählten. PL45 gibt dem Job den Mandanten mit.
  - Behoben mit PL46: eindeutige Felder und externe Schlüssel gelten je Mandant; ohne Angabe
    landet ein Datensatz im Knoten des Kontexts, der Person oder des einzigen Anlegerechts.
  - Versionen von vor `--adopt` nennen noch den alten Knoten; eine Wiederherstellung brächte den
    Datensatz an die Wurzel zurück.
  - Den Wechsel im Kopf prüft kein Browser-Test (Playwright): die gemeinsame Test-Datenbank darf
    keine Mandanten bekommen.
  - Ein `itsm tenant`-Befehl wirkt auf laufende Instanzen erst nach bis zu 5 s (Rechte-Cache,
    Hauptplan E83).

### PL43 — Kundenschicht und wirksame Definition, 2026-10-29

- **Grundlage:** Hauptplan E135, E37; architektur.md §4; E60 dieses Plans; deine
  Entscheidungen vom 2026-10-29 (PL43 vor UI25 einschieben; Liste von Änderungen; wie eine
  Erweiterung; Konflikt hält an, mit `--force`; eigene Felder mit Präfix `k_`).
- **Gebaut:**
  - `internal/meta/customer.go`: die Kundenschicht je Typ als Liste von Änderungen in
    `definition` (Schicht `customer`, Version für gleichzeitiges Schreiben). Operationen
    `add_field`, `change_field`, `add_options`, `hide_field`, `show_field`, `remove_field`,
    `move_field`. Eigene Felder heißen `k_…` und sind frei änderbar und entfernbar; an Feldern
    des Pakets nur Beschriftung, Beschreibung, Pflicht, Vorgabe, Reihenfolge, Auswahlwerte
    ergänzen und ausblenden. Ein Pflichtfeld ohne Vorgabe lässt sich nicht ausblenden, das
    Titelfeld auch nicht. Fehlt Englisch, gilt der deutsche Text. `ApplyCustomer` wendet die
    aktiven Änderungen der Reihe nach an; was nicht passt, ist ein Konflikt und wirkt nicht.
  - `meta.Types` liefert die wirksame Definition (Basis plus Kunde) mit `revision`, dem Hash der
    gespeicherten Schichten; `meta.BaseTypes` die Basis allein. Pakete dürfen das Präfix `k_`
    nicht verwenden.
  - `internal/customer`: Hinzufügen prüft gegen die wirksame Definition wie ein Paket und legt
    die Spalte sofort an; Zurücknehmen nur, wenn keine spätere Änderung sie braucht (ein
    eigenes Feld behält Spalte und Werte); Export aller Schichten; `UpdateConflicts` und
    `Still` für ein Paket-Update.
  - API mit dem Sonderrecht `customize` am Pseudotyp `core.type`:
    `GET`/`POST /api/v1/types/{type}/customer`,
    `DELETE /api/v1/types/{type}/customer/{id}?version=n`, `GET /api/v1/customer`.
  - Kommandozeile: `itsm package install [--force] <dir>` zeigt Konflikte mit der Kundenschicht
    und hält an, mit `--force` werden die Änderungen stillgelegt (sichtbar mit Grund, ohne
    Wirkung); `itsm package export-customer <dir>` schreibt `package.yaml` und
    `customer/<typ>.yaml`.
  - Oberfläche: ausgeblendete Felder fehlen in Formular und Liste.
  - Nachtrag 2026-11-02 (Befund aus UI25): `meta.ValidateCustomer` prüfte den geänderten Typ
    als Paket mit nur diesem Typ; Merkmale des Pakets und Summen aus anderen Typen des Pakets
    galten als unbekannt, ein Feld an `probe.probe` wurde abgelehnt. Jetzt prüft es den Typ mit
    den übrigen Basistypen und Merkmalen seines Pakets, wie bei der Installation; Beleg
    `TestCustomerLayerInsideItsPackage` (siehe UI-Plan, UI25).
  - Nachtrag 2026-11-02 (UI26.1, UI-Plan E30): die Kundenschicht ändert auch das Statusmodell.
    Operationen `add_state`, `change_state` (Beschriftung, Anfangsstatus wandert), `remove_state`
    (nur eigene, nicht der Anfangsstatus, nicht von einem Übergang benutzt, in keiner Datenbank
    mit Datensätzen: `records.InState` je Datenbank), `add_transition`, `change_transition`
    (Beschriftung, Von, Nach, Bedingung), `remove_transition` (nur eigene), `disable_transition`
    und `enable_transition` (`Transition.Disabled`: bleibt in der Definition, wird weder
    angeboten noch ausgeführt; 422 `disabled`). `ApplyCustomer` kopiert jetzt auch Status und
    Übergänge, `Validate` prüft danach wie bei der Installation (genau ein Anfangsstatus,
    bekannte Status, Bedingung kompiliert). Dazu `POST /api/v1/types/{t}:check-condition` für
    die Prüfung beim Tippen. Belegt: `TestApplyWorkflow` (meta), `TestCustomerLayerInsideItsPackage`
    (eigener Status und Übergang mit Bedingung an `probe.probe`, Datensatz läuft hindurch, Bedingung
    hält einen zweiten zurück, `finish` stillgelegt und wieder aktiv, Status mit Datensatz bleibt).
  - Nachtrag 2026-11-02 (UI26.2, UI-Plan E27, E31): `Type.Layout` (`map[string]Point`, nur
    JSON) und die Operation `set_layout` (Feld `layout`) ersetzen die Anordnung des Graphen
    als eine Änderung; jeder Name muss ein Status sein, `remove_state` nimmt den Status aus der
    Anordnung, leer entfernt sie, `validateStates` meldet veraltete Namen. Belegt:
    `TestApplyLayout` (meta), `set_layout` in `TestCustomerLayerInsideItsPackage`.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Einzeln (`internal/meta`): alle Operationen in Reihenfolge, stillgelegte übersprungen, ein
    Konflikt ändert nichts, die Basis bleibt unberührt; jede Ablehnung; das Präfix im Paket.
  - Über HTTP (`TestCustomerLayerOverHTTP`, eigenes Testpaket `customertest`): ohne `customize`
    403; ein Feld `k_dienststelle` steht sofort im Typ und in der Tabelle, ein Datensatz trägt
    es; zehn verbotene Änderungen mit Grund abgelehnt; Beschriftung, Option, Ausblenden und
    Verschieben wirken; alte Version 409; ein Feld, das eine spätere Änderung braucht, bleibt;
    der Export enthält genau die vier Änderungen; nach erneuter Installation bleibt die
    Schicht; ein Update ohne `color` meldet den Konflikt der Option, erzwungen ist sie
    stillgelegt und das eigene Feld bleibt; ein Paket mit `k_x` wird abgelehnt.
  - Vitest (`render/Form.test.tsx`): ein ausgeblendetes Feld fehlt im Formular und als Spalte.
  - Ganze CI grün am 2026-10-29 im zweiten Lauf (der erste scheiterte an vier Befunden von gosec in cmd/itsm/package.go, behoben), in der WSL-Kopie: Go und Web (53 Vitest-Fälle, 9 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,93 ms, LZ2 8,92 ms, LZ3 11,98 ms, LZ4 10,43 ms, LZ5 11.359 Zeilen/s, LZ13 7.966 Ereignisse/s, LZ9 358 ms.
- **Nicht geprüft:** Status, Übergänge und Feldzustände ändert die Kundenschicht noch nicht; das
  braucht UI26 und kommt dort dazu. Der Import des Exports kommt mit EX. Die wirksame
  Definition wird weiter bei jedem Zugriff aus der Datenbank gelesen, ohne eigenen Cache. Wer
  wann was geändert hat, steht in der Änderung (`at`, `by`), noch nicht im Ereignisprotokoll.
  Bei Mandanten in eigenen Datenbanken (PL47) zieht die Kundenschicht in die Steuer-Datenbank.

### PL42 — Zeitleiste am Datensatz, 2026-10-29

- **Grundlage:** Hauptplan E121, Inventar G40; UI3 im UI-Plan, die Folgemeldungen brauchte,
  die es nicht gab; deine Entscheidungen vom 2026-10-29 (vor UI3 einschieben; Einträge
  unveränderlich; eigenes Recht `comment`, privat zusätzlich `read_private`; Benachrichtigung
  auf `comment`, private nur an Berechtigte; die Version des Datensatzes bleibt); E57 dieses
  Plans.
- **Gebaut:** Kernmerkmal `timeline` (`internal/meta/traits.go`); Migration `00030_entry.sql`
  (Kerntabelle `entry`); `db/queries/entry.sql`; `internal/records/timeline.go` (`Timeline`,
  `AddEntry`, `GetEntry`, Rechte `comment` und `read_private`, Ereignis `comment`); die
  Einträge gehen beim endgültigen Löschen mit; API `GET` und `POST
  /api/v1/records/{typ}/{id}/timeline` (`internal/httpapi/timeline.go`, OpenAPI mit
  `TimelineItem`, `TimelineEntry`, `TimelineEntryCreate`, Pfad je Typ mit dem Merkmal);
  Benachrichtigungen: Ereignis `comment`, Platzhalter `{{entry}}`, private Einträge nur an
  Personen, deren aktives Konto `read_private` am Datensatz hat (`internal/notify`); der Typ
  `probe.probe` hat das Merkmal.
- **Belegt, 2026-10-29 im Werkzeug-Container:**
  - Prüfweg (`TestTimelineOverHTTP`): Drei Rollen auf `probe.probe` – Anforderer (lesen nur
    den Titel, `comment`), Bearbeiter (lesen, `comment`, `read_private`), Leser (nur lesen).
    Die Zeitleiste zeigt Verwaltung und Bearbeiter alle vier Einträge, Anforderer und Leser
    nur die zwei öffentlichen. Der Anforderer schreibt ohne `update` einen öffentlichen
    Eintrag (201); einen privaten (403) und eine Änderung am Datensatz (403) darf er nicht;
    der Leser schreibt keinen (403). Reihenfolge nach Zeit, zuerst das Anlegen, mit dem
    Namen des Kontos; die Änderung von `kind` steht drin, beim Anforderer (liest nur den
    Titel) nicht. Die Version des Datensatzes bleibt bei 2; vier Ereignisse `comment` ohne
    den Text. Abgelehnt mit 422: leerer Text, fehlender Text, Zahl statt Text, `private`
    kein Wahrheitswert, unbekanntes Feld, Text über 100.000 Zeichen. `core.person` hat keine
    Zeitleiste (404), die OpenAPI zeigt den Pfad nur für `probe.probe`. Nach dem endgültigen
    Löschen sind die Einträge weg.
  - Benachrichtigung (`TestTimelineNotifications`): eine `core.notification` auf `comment`
    an Anna (Bearbeiterrolle) und Ben (Anforderer) mit `{{actor}} schreibt: {{entry}}`. Ein
    öffentlicher Eintrag geht an beide mit Text und Namen, ein privater nur an Anna.
  - Einzeln (`internal/notify`): `{{entry}}` wird geprüft und ersetzt.
  - Angepasst: `TestTraitsOverHTTP` erwartet die Merkmale der Probe jetzt mit `timeline`.
  - Ganze CI grün am 2026-10-29 im ersten Lauf: Go und Web (23 Vitest-Fälle, 2 Playwright-Fälle mit axe), Leistungsgate bestanden; LZ1 p95 3,07 ms, LZ2 6,06 ms, LZ3 9,16 ms, LZ4 10,00 ms, LZ5 10.564 Zeilen/s, LZ13 8.694 Ereignisse/s, LZ9 107 ms.
- **Befund beim Bauen, behoben am 2026-10-29 auf deine Entscheidung:** `GET …/history`
  lieferte alle Feldänderungen, auch die von Feldern, die die Sitzung nicht lesen darf
  (Feldrechte aus PL9); die Versionen (PL19) und die Zeitleiste ließen sie weg. Jetzt filtert
  `records.History` mit derselben Prüfung (`readableSet`). Belegt in `TestTimelineOverHTTP`:
  der Anforderer (liest nur den Titel) sieht in `/history` die Änderung des Titels, nicht die
  von `kind`; die Verwaltung sieht beide. Die ganze CI lief mit UI3 grün.
- **Nicht geprüft:** Die Uhren und Regeln reagieren noch nicht auf Einträge (etwa „eine
  Antwort des Anforderers setzt die Uhr fort“); das Ereignis `comment` ist dafür da, ein
  Auslöser der Regeln fehlt. Mails aus `core.notification` stehen nicht in der Zeitleiste
  (nur die notify-Aktion der Regeln hinterlässt eine Verlaufszeile). Keine Seiten: die
  Zeitleiste kommt ganz, bei sehr langen Vorgängen ungemessen. Dateien an einem Eintrag gibt
  es nicht (Dateien hängen am Datensatz, PL18). Die Oberfläche zeigt die Zeitleiste erst mit
  UI3.

### PL41 — Verteiler in Stapeln, 2026-10-28

- **Grundlage:** Hauptplan E112, E39, E46, E87, LZ13; Befund des Rauchtests von WF4 (rund 375
  Ereignisse/s, ein Bulk-Lauf mit 100.000 Datensätzen verzögerte Webhooks und Regeln um gut vier
  Minuten); das Konzept vom 2026-10-28 mit deinen Antworten (Outbox bleibt, Stapel, eigene
  Positionen, Protokoll der endgültigen Fehlschläge); E56 dieses Plans.
- **Gebaut:** Migration 00025 (`event_consumer` mit Zustand und `durable`, die Abnehmer `live`,
  `webhooks`, `rules` statt `dispatcher`, Tabelle `delivery_failure`); `internal/events` mit
  Stapel-Abnehmern, Sperre je Zeile, Fehlerzustand, Warnung nach 5 Minuten, `Consumers`,
  `Failures`, `RecordFailure`, `SkipToEnd`; die Aufbewahrung wartet auf den langsamsten
  dauerhaften Abnehmer; `webhooks.HandleBatch` und `engine.HandleBatch`; Fehlschläge beim
  letzten Versuch oder Abbruch in beiden Workern; `jobs.EnqueueManyTx`; API
  `/api/v1/events/consumers` und `/api/v1/events/failures`; LZ13 im Lastwerkzeug; SSE nimmt
  Stapel entgegen.
- **Belegt, 2026-10-28 im Werkzeug-Container:**
  - Abnehmer (`TestConsumersKeepTheirPositions`): Ein Abnehmer, der scheitert, bleibt auf
    seiner Position, behält Fehler, Beginn und Zahl der Versuche, und was er im gescheiterten
    Durchgang geschrieben hat, ist zurückgerollt; der andere läuft weiter. Im nächsten Durchgang
    bekommt er dieselben Ereignisse, danach ist der Fehler weg. Hält eine andere Instanz die
    Zeile, lässt dieser Verteiler den Abnehmer in Ruhe und macht weiter, sobald sie frei ist;
    kein Stapel ist größer als 1.000.
  - Aufbewahrung (`TestEventRetention`): Ein dauerhafter Abnehmer hinter einem alten Ereignis
    hält es fest; haben alle es genommen, wird es gelöscht.
  - Fehlschläge über HTTP (`TestDeliveryFailuresOverHTTP`): Eine Regel, die beim Speichern
    gültig ist, aber zur Laufzeit abgelehnt wird (sie leert die Lösung eines gelösten
    Datensatzes, WF2), erscheint in `/api/v1/events/failures` mit Regel, Typ, Datensatz und
    Grund; der Datensatz bleibt unverändert. `/api/v1/events/consumers` zeigt Position und
    Rückstand; ohne `read` auf `core.job` beide 403.
  - Webhooks, Regeln, SSE und Strom laufen über den neuen Verteiler (`TestWebhooksOverHTTP`,
    `TestFanOutFiltersAndRights`, `TestRulesOverHTTP`, `TestEventsAPI`,
    `TestStreamDispatchAndRetention`); ein Abo, auf das ein Ereignis nicht passt, rückt jetzt
    mit dem Stapel vor, bekommt aber keine Zustellung.
  - Gefunden beim Bauen: Die Migration lief durch die Tests in die gemeinsame Datenbank, während
    dein Container noch den alten Code hatte; der suchte seine Zeile `dispatcher`, legte sie neu
    an und lief einmal durch den Strom (rund 160 Regel-Jobs). Der Container war bis zum Neubau
    angehalten, die Zeile `dispatcher` ist gelöscht (sie hätte die Aufbewahrung blockiert).
  - `scripts/ci.sh` komplett grün (Go, Lizenzen, Last mit Gate: LZ13 8.035 Ereignisse/s vom Start des Bulk-Laufs bis alle Abnehmer durch sind, Läufe 7.234/6.141/8.035/7.300, Ziel 5.000, vorher rund 375 Ereignisse/s im Verteiler allein; LZ5 10.272 Zeilen/s; LZ1 p95 2,66 ms, LZ2 6,92 ms, LZ3 6,79 ms, LZ4 8,82 ms; Web mit 23 Vitest- und 2 Browser-Tests, LZ9 133 ms).
- **Nicht geprüft:** Zwei Instanzen gleichzeitig nur über die Sperre im Test, nicht mit zwei
  laufenden Servern unter Last. Die Warnung „hängt“ steht im Log, nicht in einer Anzeige oder
  Kennzahl (BT). `delivery_failure` wird nicht aufgeräumt. Die Sichtbarkeit eines Ereignisses für
  den Besitzer eines Abos liest bei Treffern weiterhin den Datensatz einzeln. Ein Abnehmer, der
  dauerhaft scheitert, hält die Aufbewahrung auf; dann wächst die Tabelle `event`, bis jemand
  den Fehler behebt.
- **Nicht geprüft:** keine Begrenzung von Anmeldeversuchen und keine Zwei-Faktor-Anmeldung
  (PL28); keine API-Token für Maschinen (Middleware meldet sich vorerst wie ein Mensch an, der
  Token kommt mit PL9/EX); die Sitzung wird je Anfrage einmal aus Valkey gelesen, kein lokaler
  Cache; LDAP und OIDC (PL9, PL10) nutzen die Sitzungen, sind aber nicht gebaut. Das Konto
  `load` mit festem Kennwort gibt es nur auf der Testdatenbank der CI.
- **Nachgetragen am 2026-10-20, Abend:** Beim Übertragen nach WSL kamen `internal/meta/validate.go`
  und `internal/records/records.go` mit 0 Byte an, und zwar in beiden Kopien (Zeitstempel
  20:17:36 sekundengenau, also von außen geschrieben); der Docker-Build in WSL brach ab. Beide
  Dateien wurden aus dem Sitzungsprotokoll wiederhergestellt (letzter vollständiger Stand plus
  die drei nachfolgenden Änderungen in ihrer Reihenfolge), Build, Linter und Tests der Pakete
  `meta`, `records`, `httpapi`, `auth` und `cmd` grün, `values.go` und `records_test.go` byte-
  gleich mit der WSL-Kopie. Dabei wurde der Selbsttest des Gates stabiler: unter `-race`
  streuten zwei identische Server um 1,1 ms, der Test erlaubt jetzt 5 ms Boden und bremst
  mit 20 ms. Lehre: vor jeder Übertragung ein Commit (V5), dann ist ein Verlust ein `git checkout`.
