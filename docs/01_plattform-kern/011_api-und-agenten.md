# Plan: API und Agenten (AP)

**Stand:** 2026-11-16, erste Fassung auf Zuruf (deine Zustimmung vom 2026-11-16, E1). Teil von
[Plattform-Kern](../01_plattform-kern.md), Etappe 2 (Meilenstein 1) bis 4. Geprüft gegen `main`
bei `37aba66`. Grundlage: [Architektur](../architektur.md) §6, Hauptplan E11, E20, E71 und E90,
die Leistungsziele im [Hauptplan](../0_maintask.md) §2.5 und die Messung H14 (Hauptplan §7). Die
Befunde stehen in §2 dieses Plans; eine eigene Bewertung gibt es nicht. Nichts davon ist gebaut.

> **Worum es geht:** prüfen und sichern, dass die API das erste Produkt ist: vollständig
> beschrieben, vollständig getestet, gegen eine Flut geschützt und von einem Agenten ohne Wissen
> aus dem Code bedienbar. Der MCP-Server selbst bleibt im Nebenplan 09 Integration (Hauptplan
> V18); AP18 ist seine Abnahmeliste.

**Status:** ✅ abgenommen · 🟢 fertig, wartet auf die Abnahme · 🔨 in Arbeit · ⬜ offen ·
🔒 blockiert

**Für jede Aufgabe gilt:** Sie ist erst fertig, wenn `scripts/ci.sh` grün ist und die
Leistungsmessung aus Hauptplan H11 keinen Zielwert verfehlt und nichts um mehr als 10 %
langsamer macht (E32). Der Prüfweg nennt nur, was dazukommt. Alle Befehle laufen in WSL in
`~/police/itsm`.

---

## Aufgaben

### A — Prüfen

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **AP1** | Flut-Szenario im Lastwerkzeug: eine Stufe mit fester Rate statt fester Arbeiter; fünf Fälle (ohne Anmeldung, Login, Token, eine Sitzung, ein Zugang); daneben misst ein normaler Nutzer LZ1 und LZ4; 2xx, 429 und 503 als eigene Spalten statt als Fehler; kein Teil der CI (§2.3) | ⬜ | 1 d | `docker compose run --rm tools go run ./test/load run --stage=flood --rate=1000 --target=http://itsm:8080` → je Fall eine Zeile mit dem Anteil 2xx, 429 und 503 und dem p95 des normalen Nutzers; der Bericht steht im Umsetzungsstand |
| **AP2** | Vertragstest über alle Operationen: jede Antwort des Mandanten-Rundgangs (PL44), auch Fehler und Schreiben, wird gegen die OpenAPI geprüft (§2.4) | ⬜ | 1,5 d | `docker compose run --rm tools go test ./internal/httpapi -run TestContract -v` meldet 157 von 157 Operationen; eine absichtlich falsche Antwort in einem Probe-Zweig macht ihn rot |
| **AP3** | Fehlerfälle: 405 unter `/api`, Body über 1 MiB an normalen Endpunkten, unbekannte Felder an Login und Bulk, Seitengrenzen (`limit` 0, negativ, über 500, kaputter `after`), Token nach Ablauf, eine echte langsame Abfrage bis zum 503, zwei PATCH mit derselben Version zugleich (§2.4) | ⬜ | 1 d | die neuen Tests in `internal/httpapi` sind grün; bei den zwei PATCH genau eine 200 und eine 409; jede Antwort ist `application/problem+json` |
| **AP4** | Fuzzing für JSON-Abfrage, Filter, Sortierung und Cursor, dazu Einschleus-Zeichenketten; der gefundene Korpus läuft als normaler Test in der CI (§2.4) | ⬜ | 1 d | `docker compose run --rm tools go test ./internal/records -fuzz FuzzQuery -fuzztime 60s` endet ohne Fund; `scripts/ci.sh` führt den Korpus aus |
| **AP5** | `govulncheck` als Schritt der CI; woher die Liste der Schwachstellen ohne Netz kommt, klärt die Aufgabe. Dazu ein Test der Sicherheitskopfzeilen (CSP, Frame, HSTS) und von CORS (§2.4) | ⬜ | 0,5 d | `docker compose run --rm tools scripts/ci.sh` zeigt den Schritt „vuln“ grün; eine Abhängigkeit mit bekannter Schwachstelle in einem Probe-Zweig macht ihn rot |

### B — Limits

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **AP6** | Schutz vor der Anmeldung: Zähler je Adresse in Valkey an Login und Token, geprüft vor Argon2; Sperre auch am Token-Endpunkt; Obergrenze gleichzeitiger Argon2-Rechnungen je Instanz (§2.3, Schicht 1) | 🔒 V1 | 1 d | AP1, Fälle Login und Token: über der Grenze 429 mit `Retry-After` ohne Hash-Rechnung; der normale Nutzer hält daneben LZ1 |
| **AP7** | Lastabwurf je Instanz: Obergrenze gleichzeitiger Anfragen nahe der Poolgröße, kurze Warteschlange, danach sofort 503 mit `Retry-After`; eigene Budgets für Suche, Export und Aggregate; `ReadTimeout`, `IdleTimeout` und `MaxHeaderBytes` am Server (§2.3, Schicht 2) | 🔒 V1 | 1,5 d | AP1, Fall Sitzung mit 1.000 Suchen je Sekunde: der normale Nutzer hält LZ1 und LZ4, Überzähliges bekommt 503 in unter 100 ms |
| **AP8** | Rate für jeden Aufrufer, auch Sitzungen, und je Mandant: ein Zähler für alle, Prüfung ohne Datenbank (Zugang und Einstellung im Speicher wie bei PL58), gleitendes Fenster statt Minutenfenster (§2.3, Schicht 3) | 🔒 V1 | 1,5 d | AP1, Fall Zugang: abgewiesene Anfragen erzeugen keine Abfrage in PostgreSQL (`pg_stat_statements` vor und nach dem Lauf); an der Minutengrenze geht nicht mehr die doppelte Menge durch |
| **AP9** | Metriken im Server: Anfragen, 429, 503, Wartezeit am Pool (Architektur §11, „Sichtbar machen“); Dashboards und Alarme bleiben beim Betrieb (BT) | 🔒 V1 | 1 d | `curl -s localhost:8092/metrics` zeigt die Zähler; nach einem Lauf von AP1 sind 429 und 503 gestiegen |

### C — API-first

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **AP10** | OpenAPI vervollständigen: Bearer und Client Credentials als Sicherheitsschema, `security` an jeder Operation, 401, 429 und 503 mit `Retry-After`, `If-Match` als Pflicht, die Pfade je Typ für Übergang, Batch, Export und Vorschlag in der Beschreibung der Laufzeit, Beispiele für die 20 wichtigsten Operationen, ein Schema für `TypeDefinition` (§2.5) | ⬜ | 1,5 d | ein Test über die Beschreibung: jede Operation hat `security`, 401 und 429; `curl -s localhost:8092/api/v1/openapi.json` zeigt `/api/v1/records/incident.incident/{id}:transition` |
| **AP11** | Gleichstand von Oberfläche, API und Kommandozeile: die Liste dessen, was nur die Kommandozeile kann (§2.5), je Zeile entschieden: in die API oder bewusst nur Betrieb; ein CI-Schritt prüft, dass `web/src` nur `/api/v1` ruft | ⬜ | 0,5 d | die Tabelle steht in §2.5, jede Zeile mit Entscheidung; der CI-Schritt wird rot bei einem `fetch` auf einen anderen Pfad in einem Probe-Zweig |

### D — Agentic-first

| # | Aufgabe | Status | Aufwand | So prüfst du es |
| --- | --- | --- | --- | --- |
| **AP12** | Die Anforderungen an einen Agenten als Entscheidungen festhalten: Zugang nur zum Lesen, Probelauf, Idempotenz, Protokoll, „Was darf ich“ (§2.6); jede mit deiner Antwort | ⬜ | 0,5 d | die Entscheidungen stehen in §1; AP14 bis AP17 sind danach frei oder entfallen |
| **AP13** | Der Zugang im Protokoll: die Sitzung eines Tokens trägt den Zugang; Verlauf, Ereignisprotokoll und Anfrageprotokoll nennen ihn (§2.6, Punkt 4) | ⬜ | 1 d | eine Änderung per Token → `GET …/{id}/history` nennt den Zugang; die Zeile im Anfrageprotokoll trägt `client` |
| **AP14** | `Idempotency-Key` am Anlegen, am Übergang und am Batch, wie heute am Bulk (§2.6, Punkt 3) | 🔒 AP12 | 1 d | zweimal POST mit demselben Schlüssel → ein Datensatz, die zweite Antwort gleich der ersten und mit `Idempotent-Replayed: true` |
| **AP15** | Probelauf beim Schreiben: prüft Rechte, Eingabe, Regeln und Übergang, schreibt nicht (§2.6, Punkt 2) | 🔒 AP12 | 1 d | POST mit `?dry_run=true` → 200 oder 422 wie beim echten Aufruf; die Zahl der Datensätze, der Verlauf und der Ereignisstrom bleiben gleich |
| **AP16** | Zugang nur zum Lesen: ein Merkmal am Zugang oder ein Scope am Token (§2.6, Punkt 1) | 🔒 AP12 | 1 d | ein Token nur zum Lesen: GET 200, POST, PATCH und DELETE 403 |
| **AP17** | „Was darf ich“ je Typ und Aktion; `If-Match` an Papierkorb, Wiederherstellen und endgültigem Löschen; die fehlenden Titel der Problemtypen (§2.6, Punkte 5 und 6) | 🔒 AP12 | 1 d | `curl -s localhost:8092/api/v1/auth/permissions` nennt je Typ die erlaubten Aktionen; DELETE ohne `If-Match` → 428; ein 429 trägt einen deutschen Titel |
| **AP18** | Agenten-Probe: ein Programm arbeitet nur mit der OpenAPI der Laufzeit und einem Dienstkonto: Typ finden, Störung anlegen, einen Fehler anhand der Meldung beheben, Übergang auslösen; ohne fremden Dienst und ohne Sprachmodell (§2.6) | 🔒 AP10 | 1 d | `docker compose run --rm tools go test ./test/agent -v` grün; der Test kennt keinen Pfad und kein Feld außer aus `/api/v1/openapi.json` und `/api/v1/types` |

**Summe:** 18,5 d (Block A 5 d, B 5 d, C 2 d, D 6,5 d), geschätzt vor dem Entwurf.

**Reihenfolge:** AP1 zuerst, damit die Flut gemessen ist, bevor Limits gebaut werden (V2: wo).
Danach AP6, weil Login und Token der schwerste Fall sind (§2.3) und vor Meilenstein 1 geschützt
sein sollen, dann AP2 und AP3. Block B vor C und D; AP10 vor AP18. AP12 ist nur eine Runde mit
dir und kann jederzeit laufen. Aufgaben, die Code ändern, baut der eine Agent, der am Code
arbeitet (Hauptplan E149), nie zwei zugleich.

---

## 1. Entscheidungen

Die übergreifenden Entscheidungen stehen im Hauptplan §1 und in [`adr/`](../adr/): E11 (eine
REST-API, kein zweiter Weg an der Oberfläche vorbei), E20 (Valkey für Ratenbegrenzung), E71
(Fehler nach RFC 9457), E90 (API-Zugänge).

| # | Entscheidung | Seit | Quelle |
| --- | --- | --- | --- |
| **E1** | Die Prüfung von API-first und Agentic-first ist ein Unterplan des Plattform-Kerns mit dem Kurzzeichen `AP`, nicht eine Reihe von PL-Aufgaben und kein zehnter Nebenplan (Standard §2.2: 01 bis 09 sind vergeben) | 2026-11-16 | Pascal |

## 2. Begründung und Befunde

### 2.1 Grundlage und Grenzen

- **Gelesen, nicht gemessen:** Alle Aussagen in §2.2 bis §2.6 kommen aus dem Code auf `main` bei
  `37aba66` und aus den Plänen. Für diesen Plan ist nichts ausgeführt worden, kein Test und keine
  Messung. Zahlen zur Geschwindigkeit stammen aus Hauptplan §7 H14.
- **Stichproben:** Zwei Lese-Agenten haben das Test-Inventar und die Stellen zu API-first
  zusammengetragen. Am Original nachgesehen sind die Fundstellen in §2.2, §2.3 und §2.6 sowie in
  §2.4 und §2.5 die Zeilen von `coverage_test.go`, `tenant_test.go`, `api_test.go`,
  `bulk_test.go`, `scripts/ci.sh`, `api/openapi.yaml` und `api.go`. Nicht einzeln nachgesehen:
  `rights_test.go`, `rights2_test.go`, `auth_test.go`, `client_test.go` und die Tests zu
  Ereignissen und Webhooks.
- **Nicht geprüft:**
  - Was bei einer Flut wirklich passiert; §2.3 ist abgeleitet. Das misst AP1.
  - Was eine Argon2id-Rechnung je Anfrage kostet.
  - Die Poolgröße: nachgesehen ist nur, dass `internal/db/db.go:30-35` sie nicht setzt, und was
    der Umsetzungsstand von PL58 dazu sagt.
  - Ereignisstrom, Export und Hochladen über ihre Ausnahme von der Zeitgrenze hinaus.
  - Die Aufwände sind Schätzungen ohne Entwurf.
- **Nicht in AP:** der MCP-Server (Nebenplan 09, Hauptplan V18); Limits im Proxy oder Ingress,
  Dashboards, Alarme und Härtung des Betriebs (Nebenplan 08); ein Pooler vor PostgreSQL
  (Hauptplan V22); das Captcha und die Rate der anonymen Formulare (SD15).

### 2.2 Authentifizierung heute

- **Browser:** Sitzung in Valkey, Cookie `itsm_session`; jede schreibende Anfrage braucht
  `X-Requested-With: itsm`, sonst 403 (`internal/httpapi/auth.go:29-33, 68-76`). Anmelden lokal
  mit Argon2id (19 MiB, `internal/auth/password.go:25-28`), über OIDC und über LDAP. Ohne
  Anmeldung offen sind nur Version, Login, Token und die OIDC-Pfade (`auth.go:37-44`).
- **Maschinen:** OAuth 2 Client Credentials an `POST /api/v1/auth/token`
  (`internal/httpapi/clients.go:54-97`); das Token ist ein Zufallswert in Valkey, Vorgabe 3600 s
  (`internal/authn/client.go:45`), der Aufruf trägt `Authorization: Bearer` (`auth.go:64-67`).
  Ein Zugang ist ein Konto ohne Kennwort mit Person und Rollen, dazu IP-Bereiche, Rate und
  Token-Dauer (Hauptplan E90; Umsetzungsstand PL13).
- **Nicht vorhanden:** API-Schlüssel, JWT, Scopes, ein Token nur zum Lesen.
- **Mandant und Rechte je Anfrage:** Die Middleware lädt die Rechte des Kontos
  (`internal/authz/authz.go:949`), tritt in den Mandanten der Sitzung ein und engt auf aktive
  Rolle und Knoten ein (`auth.go:113-152`). Der Abfragebau hängt die Rechtebedingung immer an
  (Architektur §6). Ein Test läuft mit fremden Kennungen über jede Operation der Beschreibung
  (`internal/httpapi/tenant_test.go:280-356`).
- **Auffällig:**
  - Die OpenAPI nennt nur das Cookie (`api/openapi.yaml:2981-2985`); Bearer fehlt dort, obwohl
    gebaut → AP10.
  - Die Sitzung eines Tokens trägt kein Kennzeichen des Zugangs
    (`internal/authn/client.go:102-108, 507`) → AP13.
  - `X-Forwarded-For`: nur der erste Eintrag, keine Liste vertrauter Proxys
    (`internal/httpapi/clients.go:32-39`; steht schon im Umsetzungsstand von PL13). Davon hängt
    jeder Zähler je Adresse ab → AP6.

### 2.3 Flut und Limits

**Was es gibt:**

- Rate je API-Zugang: Vorgabe 600 je Minute, festes Minutenfenster in Valkey, darüber 429 mit
  `Retry-After` (`internal/authn/client.go:44, 493-506`; `internal/httpapi/clients.go:109-112`).
- Anmeldesperre: 5 Fehlversuche je Konto, 20 je Adresse in 15 Minuten
  (`internal/auth/policy.go:70`).
- Zeitgrenze 10 s je Anfrage, danach 503 (PL56, `internal/httpapi/timelimit.go:33-47`); Listen
  höchstens 500 Zeilen (`internal/records/records.go:277`); JSON-Body 1 MiB
  (`internal/httpapi/api.go:27`).

**Was es nicht gibt:** kein Limit für Browser-Sitzungen, keines je Mandant, keines je Adresse,
keine Obergrenze gleichzeitiger Anfragen, kein Lastabwurf. Der HTTP-Server setzt nur
`ReadHeaderTimeout` (`internal/httpapi/router.go:236-240`). Der Pool bleibt bei der Vorgabe von
`pgx` (`internal/db/db.go:30-35`). Metriken stehen in der Architektur (§11), aber weder
Prometheus noch OpenTelemetry in `go.mod`: eine Flut wäre heute nur im Anfrageprotokoll zu
sehen. Das Lastwerkzeug misst nur mit festen Arbeitern, die nacheinander anfragen
(`test/load/main.go:76-77`, `test/load/internal/measure/measure.go:46-48`); 429 und 503 zählen
dort als Fehler. Es zeigt, wie schnell der Server ist, nicht, was bei festem Zustrom über der
Kapazität passiert.

**Was bei 1.000 Anfragen je Sekunde nach dem Code passiert** (abgeleitet; der Durchsatz zum
Vergleich ist der aus H14 auf `ki-lab` mit 32 Arbeitern):

| Wer flutet | Was passiert |
| --- | --- |
| Ohne Anmeldung auf geschützte Pfade | 401 nach einem Lesen in Valkey. Billig |
| Ein Zugang mit Token | Ab der 601. Anfrage der Minute 429. Aber jede abgewiesene Anfrage liest vorher den Zugang und die Einstellung aus PostgreSQL (`internal/authn/client.go:486, 493`): das Limit schützt die Handler, nicht die Datenbank. An der Minutengrenze geht die doppelte Menge durch |
| Eine Browser-Sitzung | Kein Limit. Lesen (31.852/s) und Anlegen (2.199/s) liegen über 1.000 je Sekunde; die Warteschlange eines Teams (344/s) und die Suche (365/s) liegen weit darunter. Dann warten Anfragen auf eine Verbindung des Pools, auch die aller anderen Nutzer, und enden nach 10 s mit 503. Vorher wird nichts abgewiesen |
| `/auth/login` oder `/auth/token` ohne Anmeldung | **Der schwerste Fall.** Jede Anfrage rechnet Argon2id mit 19 MiB, auch bei gesperrtem Konto oder gesperrter Adresse (`internal/auth/auth.go:314-317`) und bei unbekanntem Zugang (`internal/authn/client.go:426`). Der Token-Endpunkt hat keine Sperre. 1.000 je Sekunde dort binden CPU und Speicher der Instanz |
| Export, Bulk-Strom, Ereignisstrom, Hochladen | Von der Zeitgrenze ausgenommen (`internal/httpapi/timelimit.go:26-30`); eine Obergrenze gleichzeitiger Exporte oder Ströme habe ich nicht gefunden |

**Empfehlung: Limits in vier Schichten** (V1):

1. **Vor der Anmeldung** (AP6): Zähler je Adresse an Login und Token, geprüft vor Argon2; Sperre
   auch am Token-Endpunkt; wenige gleichzeitige Argon2-Rechnungen je Instanz.
2. **Lastabwurf je Instanz** (AP7): Obergrenze gleichzeitiger Anfragen, kurze Warteschlange,
   danach sofort 503 mit `Retry-After`; eigene Budgets für teure Abfragen.
3. **Je Aufrufer und je Mandant** (AP8): derselbe Zähler für Zugänge und Sitzungen, geprüft ohne
   Datenbank, mit gleitendem Fenster.
4. **Davor im Proxy oder Ingress:** Verbindungs- und Adresslimits, Nebenplan 08. Sie ersetzen 1
   bis 3 nicht, weil eine Middleware hinter einer einzigen Adresse steht.

Was die Pläne dazu schon sagen: Valkey für Ratenbegrenzung (Hauptplan E20), „Ratenbegrenzung je
Zugang“ (Architektur §6), PL13 abgenommen mit dem Vermerk „nicht je Adresse oder für
Browser-Sitzungen“, die Rate der anonymen Formulare in SD15.

### 2.4 API-Tests

**Vorhanden:** 100 Tests in 79 Dateien unter `internal/httpapi`, gegen echtes PostgreSQL und
Valkey, in der CI mit Race-Detector.

- Vertrag: `TestAPICoverage` prüft die 200-Antworten der GETs auf Typen, Datensätze und
  Organisationsbaum gegen die OpenAPI der Laufzeit (`internal/httpapi/coverage_test.go:41-67`).
  Die CI bricht ab, wenn der erzeugte Code von der Beschreibung abweicht (`scripts/ci.sh:83-89`).
- Mandantengrenze über alle 157 Operationen (`internal/httpapi/tenant_test.go:280-356`).
- Rechte (`rights_test.go`, `rights2_test.go`), Sitzung und CSRF (`auth_test.go`), Zugänge mit
  429, IP-Bereich und Widerruf (`client_test.go`), 428 und 409 bei `If-Match`
  (`api_test.go:527-538`), Idempotenz im Bulk (`bulk_test.go:90-95`), die Größengrenze im
  Bulk-Strom (`bulkstream_test.go:198-207`), Ereignisse, Webhooks.
- 17 Dateien mit Browser-Tests (Playwright); die Lastmessung aus H11 und H14.

**Fehlt** (gesucht und nicht gefunden):

| Lücke | Aufgabe |
| --- | --- |
| Vertragsprüfung für alles außer diesen GETs: keine Fehlerantwort, kein POST, PATCH oder DELETE wird gegen die Beschreibung geprüft | AP2 |
| Fehlerfälle: 405, zu großer Body an normalen Endpunkten, unbekannte Felder, Seitengrenzen, Token nach Ablauf, eine echte langsame Abfrage, zwei PATCH mit derselben Version zugleich | AP3 |
| Fuzzing (`func Fuzz` kommt im Repository nicht vor) und Einschleus-Versuche in Filter, Sortierung und Abfrage | AP4 |
| `govulncheck` in der CI; Tests der Sicherheitskopfzeilen und von CORS | AP5 |
| Flut: kein Szenario mit fester Rate, keines über dem Limit | AP1 |

### 2.5 API-first

**Im Kern erfüllt.**

- Der Server entsteht aus der Beschreibung (`api/oapi-codegen.yaml:7-10`,
  `internal/httpapi/api.go:34`): 157 Operationen, jede mit `operationId` und `summary`.
- Die Oberfläche nutzt den erzeugten Client; die drei rohen Aufrufe gehen alle auf `/api/v1`
  (`web/src/App.tsx:20`, `web/src/app/live.tsx:56`, `web/src/render/Export.tsx:138`). Das
  verlangt Hauptplan E11.
- Typen, Feldzustände und mögliche Übergänge sind zur Laufzeit abfragbar
  (`api/openapi.yaml:85, 98, 179, 471`); die OpenAPI der Laufzeit ergänzt die Pfade je Typ
  (`internal/httpapi/api.go:295-329`).

**Lücken in der Beschreibung** (alle AP10):

- Sicherheit: nur das Cookie, kein Bearer, kein `security` an den Operationen
  (`api/openapi.yaml:2981-2985`).
- 429, `Retry-After` und der 503 der Zeitgrenze kommen nicht vor; 401 steht an 19 Stellen bei
  157 Operationen.
- Kein Beispiel; `TypeDefinition` ohne Schema (`api/openapi.yaml:3840-3843`); `If-Match` am
  Ändern ist nicht als Pflicht markiert, obwohl erzwungen (`internal/httpapi/api.go:187-191`).
- Die Beschreibung der Laufzeit lässt je Typ Übergang, Batch, Export und Vorschlag aus
  (`internal/httpapi/api.go:305`).

**Nur über die Kommandozeile** (AP11 entscheidet je Zeile):

| Was | Fundstelle | In die API? |
| --- | --- | --- |
| Paket installieren | `cmd/itsm/package.go:60-69` | offen |
| Mandant anlegen | `cmd/itsm/tenant.go:69-130` | offen |
| Lokale Konten anlegen und verwalten | `cmd/itsm/user.go:72-191` | offen |
| Migration der Datenbank | `cmd/itsm/main.go` | offen; Vorschlag: bleibt Betrieb |

### 2.6 Agentic-first

**Noch nicht erfüllt. Die Grundlage trägt, sechs Dinge fehlen.**

- **MCP:** kein Code. Geplant im Nebenplan 09 als „MCP-Server (nur lokal)“, Hauptplan V18 schlägt
  Meilenstein 3 vor; dazu die Regel „KI nur mit lokalem Modell“ (Hauptplan §1, übernommen aus dem
  GLPI-Vorhaben). `docs/legacy/` gibt es nicht.
- **Brauchbar für einen Agenten:** Fehler je Feld mit Code (`internal/records/errors.go:26-30`),
  Übergänge mit `allowed` (`api/openapi.yaml:3259-3262`), Seiten per Schlüssel, `ETag` und
  `If-Match`, Bulk und Batch mit Ergebnis je Zeile, Dienstkonten mit eigenen Rollen, IP-Bereich
  und Rate, Ereignisstrom und Webhooks.
- **Es fehlt:**
  1. Ein Zugang nur zum Lesen: das Token kennt nur Konto und Ablauf
     (`internal/authn/client.go:102-108`) → AP16.
  2. Ein Probelauf: `dry_run` kommt in `api/openapi.yaml` nicht vor → AP15.
  3. `Idempotency-Key` nur am Bulk (`api/openapi.yaml:270`,
     `internal/httpapi/bulk.go:167-228`), nicht am Anlegen, am Übergang, am Batch → AP14.
  4. Ein Protokoll dessen, was ein Agent tat: der Verlauf kennt die Konto-UUID ohne den Zugang
     (`db/migrations/00004_audit.sql:18`), das Anfrageprotokoll keinen Aufrufer
     (`internal/httpapi/router.go:217-223`), Lesen wird nicht protokolliert → AP13.
  5. „Was darf ich“: kein Endpunkt mit den wirksamen Rechten je Typ und Aktion → AP17.
  6. Papierkorb, Wiederherstellen und endgültiges Löschen ohne `If-Match`
     (`internal/httpapi/api.go:213-223`); für `rate_limited`, `timeout` und `gone` fehlt der
     Titel (`internal/httpapi/problem.go:38-51`, benutzt in `clients.go:111` und
     `problem.go:77, 96`) → AP17.

## 3. Offene Fragen

| # | Frage | Vorschlag | Hängt daran |
| --- | --- | --- | --- |
| **V1** 🟠 | Limits in den vier Schichten aus §2.3 bauen, den Schutz vor der Anmeldung (AP6) vor Meilenstein 1? | ja | AP6 bis AP9 |
| **V2** 🟡 | Wo läuft die Flut-Messung aus AP1? | auf `ki-lab` (Hauptplan E184), nicht auf dem Laptop: dort liegt der Bestand in Prod-Größe, und der Laptop bleibt frei | AP1 |
| **V3** 🟡 | Block D vor Meilenstein 3 bauen, obwohl der MCP-Server nach Hauptplan V18 erst dann kommt? | ja: eine Middleware braucht dieselben Dinge (Idempotenz, Zugang im Protokoll, Zugang nur zum Lesen), und AP18 ist die Abnahmeliste für den MCP-Server | AP12 bis AP18 |

Die Fragen an die Fachseite stehen im Hauptplan: V18 (MCP-Server) und V22 (Pooler) berühren
diesen Plan.

## 4. Umsetzungsstand

Noch keine Aufgabe begonnen.
