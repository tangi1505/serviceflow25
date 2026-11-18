# ServiceFlow25

Ein ITSM-Werkzeug nach ITIL 4 als Ablösung von GLPI: eine datengetriebene Plattform, auf der jede
Practice ein Paket ist. Produkt der group24 AG, Kern unter AGPL-3.0-only.
*English summary [below](#english).*

- **Repository:** <https://github.com/tangi1505/serviceflow25>
- **Name:** ServiceFlow25 ist der vorgeschlagene Produktname (Hauptplan V1, noch ohne
  Markenprüfung). Die Oberfläche heißt ohne eigene Einstellung so und zeigt ohne Logo die Marke
  „sf“ (UI56). Im Code heißt das Werkzeug weiter `itsm`: Binary, Modulpfad `group24.de/itsm`,
  Variablen `ITSM_…`.
- **Pläne und Entscheidungen:** [Hauptplan](docs/0_maintask.md),
  [Architektur](docs/architektur.md), [Entscheidungen](docs/adr/).

## Stand

Stand 2026-11-18, in Arbeit auf Meilenstein 1 („Service Desk läuft“). Was hier steht, ist gebaut
und selbst geprüft; abgenommen wird je Aufgabe, der Stand dazu steht im Hauptplan.

| Bereich | Gebaut |
| --- | --- |
| Plattform-Kern | Typen und Felder aus Paketen, vom Kunden erweiterbar; REST-API mit OpenAPI; Organisationsbaum und Mandanten; Rollen und Rechte; Anmeldung lokal, über OIDC und LDAP; API-Zugänge; JSON-Abfrage, Volltextsuche, Beziehungen, Dateien, Versionen; Jobs, Ereignisse, Webhooks; Bulk-API |
| Workflow-Engine | Statusmodell, Feldzustände, Regeln; Kalender und SLA-Uhren mit Eskalation; Wartegründe, Aufgaben, wiederkehrende Vorgänge; Genehmigungen in Stufen; Benachrichtigungen per Mail und im Posteingang |
| Oberfläche | Listen mit Filtern, gespeicherten Suchen, Export und Massenaktionen; Detailseite mit Zeitleiste, Bezügen und Fassungen; Editoren für Typen, Workflow und Rollen; Formular-Baukasten; Dashboards; Befehlszeile (Strg K); hell und dunkel; Erscheinungsbild der Organisation |
| Portal | Startseite mit Kacheln und „Aktuell“, „Meine Vorgänge“, Genehmigungen, Servicekatalog, Wissen |
| Service Desk | Pakete `sd`, `incident`, `request`, `catalog`, `knowledge`: Störung mit Priorität, Fristen und Major Incident; Anfrage mit Genehmigung und Erfüllungsaufgaben; Servicekatalog; Wissensartikel mit Freigabe |

Offen bis Meilenstein 1 sind unter anderem der Mail-Eingang, Umfragen und anonyme Formulare; sie
warten auf Antworten der Fachseite (Hauptplan §6.2).

## Schnellstart

Alle Befehle in WSL im Verzeichnis des Repositorys. Auf dem Rechner braucht es nur Docker.

```bash
docker compose up -d --build
```

Danach antworten:

- <http://localhost:8092/> — die Oberfläche
- <http://localhost:8092/readyz> — Datenbank und Valkey
- <http://localhost:8092/api/v1/version> — die Version

Pakete installieren, das Kernpaket zuerst, die übrigen unter `packages/` ebenso:

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/core
```

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/probe
```

Ein lokales Konto anlegen. Das Kennwort kommt auf stdin: mindestens 8 Zeichen mit Groß- und
Kleinbuchstabe, Ziffer und Sonderzeichen.

```bash
printf 'Secret-1!' | docker compose run --rm -T itsm user create pascal --display-name "Pascal"
```

Ohne Rolle sieht ein Konto nichts; die Rolle `admin` darf alles:

```bash
docker compose run --rm itsm user grant pascal admin
```

Über die API anmelden. Alles unter `/api/v1` außer `version` und `auth/login` braucht eine
Sitzung; schreibende Aufrufe tragen den Header `X-Requested-With: itsm`.

```bash
curl -s -c cookies.txt -H 'Content-Type: application/json' -H 'X-Requested-With: itsm' \
  -d '{"username":"pascal","password":"Secret-1!"}' http://localhost:8092/api/v1/auth/login
```

```bash
curl -s -b cookies.txt http://localhost:8092/api/v1/auth/me
```

## Mandanten

Ohne Mandanten ist die Wurzel der einzige. Der erste übernimmt mit `--adopt` alles Bestehende;
das lässt sich nicht zurücknehmen. Die mit `--admin` genannten Konten bleiben Konten der
Installation und wechseln den Mandanten im Kopf der Oberfläche. Eindeutige Felder und externe
Schlüssel gelten je Mandant.

```bash
docker compose run --rm itsm tenant create nord --label "Polizei Nord" --adopt --admin pascal
```

```bash
docker compose run --rm itsm tenant create sued --label "Polizei Süd"
```

**Datenbank je Mandant:** Mit `ITSM_TENANT_MODE=database` liegt jeder Mandant in seiner eigenen
Datenbank; die Steuer-Datenbank (`ITSM_DATABASE_URL`) kennt ihn nur beim Namen.

1. Der Betrieb legt die leere Datenbank samt Benutzer an und trägt ihre URL ein.
2. `tenant create` richtet sie ein und trägt den Mandanten ins Verzeichnis ein.
3. Die Instanzen neu starten. `migrate` und `package install` wirken auf jede Datenbank.

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord docker compose run --rm itsm tenant create nord --label "Polizei Nord"
```

**Sichern und wiederherstellen** je Datenbank: mit `pg_dump` sichern, mit `pg_restore` in eine
neue, leere Datenbank zurückspielen, die URL darauf zeigen lassen. `tenant check` prüft danach,
ob sie zur Steuer-Datenbank passt, und nennt, was fehlt.

```bash
pg_dump --format=custom --file=nord.dump postgres://nord:…@db/itsm_nord
```

```bash
pg_restore --no-owner --exit-on-error --dbname=postgres://nord:…@db/itsm_nord_neu nord.dump
```

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord_neu docker compose run --rm itsm tenant check nord
```

**Auswertungen über Mandanten:** `POST /api/v1/tenants:query` liefert die Zeilen jedes Mandanten
mit der Spalte `tenant`. Das Konto braucht das Recht `report_tenants` an `core.tenant`,
ausdrücklich in einer Rolle an der Wurzel; die Rolle `admin` allein reicht nicht.

## Entwickeln und prüfen

Gebaut, geprüft und getestet wird im Werkzeug-Container. Die ganze CI läuft vor jedem Commit:

```bash
scripts/ci-run.sh
```

Während der Arbeit reicht die Teil-CI: nur was sich seit dem letzten Commit geändert hat und was
davon abhängt, ohne Lastmessung.

```bash
scripts/ci-run.sh --part
```

- Jeder Lauf bekommt einen eigenen PostgreSQL und Valkey, die danach entfernt werden. Die Instanz
  auf 8092 bleibt unberührt.
- Die CI bricht nicht beim ersten Fehler ab; am Ende stehen alle Schritte mit Ergebnis.
- Einzelne Tests von Hand brauchen die Server:
  `docker compose --profile ci up -d --wait postgres-ci valkey-ci`, danach
  `docker compose run --rm tools go test ./internal/events/`.

Mails der Benachrichtigungen ansehen, mit Mailpit unter <http://localhost:8026/>:

```bash
docker compose --profile mail up -d
```

## Leistung messen

Die CI misst bei jedem Lauf den neuen Stand im Wechsel mit dem freigegebenen Basis-Binary;
`test/load/baseline.json` ist das Protokoll der Freigabe. Ist der Rechner ausgelastet, bricht die
Messung ab (Rückgabe 3). Die Ziele stehen im Hauptplan §2.5.

Auf einem geteilten Server laufen die gemessenen Server und das Lastwerkzeug auf festen Kernen
(PL71): `compose/ci-kilab.yaml` setzt PostgreSQL und Valkey der CI auf 8 Threads (4 Kerne mit
je zwei Threads, wie eine VM mit 8 virtuellen CPUs) mit Speichergrenze und gibt `scripts/ci.sh` die Kerne für `taskset` mit (`ITSM_LOAD_SERVER_CPUS`,
`ITSM_LOAD_CLIENT_CPUS`). Ohne die Variablen läuft alles auf allen Kernen.

```bash
COMPOSE_FILE=compose.yaml:compose/ci-kilab.yaml scripts/ci-run.sh --all
```

Den Datensatz in Prod-Größe erzeugen:

```bash
docker compose run --rm tools go run ./test/load generate --size=prod --out=/tmp/data
```

In Prod-Größe messen: 70.000 Personen und 18,25 Mio. Störungen über die Bulk-API einer laufenden
Instanz laden, dann die volle Stufe darauf. Die Instanz braucht so viele Bulk-Worker
(`ITSM_WORKERS_BULK`) wie `--parallel`. Einzelne Ziele wählt `--only=LZ2,LZ4`.

```bash
go run ./test/load fill --size=prod --target=http://itsm:8080 --database=postgres://… --chunk=500 --parallel=8
```

```bash
go run ./test/load run --stage=full --data=prod --target=http://itsm:8080 --out=full.json
```

## Aufbau

| Pfad | Inhalt |
| --- | --- |
| `cmd/itsm/` | das eine Binary mit `serve`, `migrate`, `package`, `user`, `tenant`, `ldap`, `client`, `calendar`, `healthcheck`, `version` (`itsm` ohne Argument zeigt alle) |
| `internal/` | die Bausteine aus `docs/architektur.md` §2 |
| `packages/` | die mitgelieferten Pakete: Kern, Service Desk, Störung, Anfrage, Katalog, Wissen, Probe |
| `db/migrations/`, `db/queries/` | Migrationen (`goose`) und Abfragen (`sqlc`) der Kerntabellen |
| `web/` | die Oberfläche (React, TypeScript, Vite) |
| `tools/` | der Werkzeug-Container |
| `scripts/` | CI und Prüfskripte |
| `test/load/` | Lastwerkzeug, Datensatzgenerator, Basislinie |
| `docs/` | Hauptplan, Nebenpläne, Architektur, Entscheidungen (`docs/adr/`) |

## Konfiguration

Nur über Umgebungsvariablen.

| Variable | Pflicht | Vorgabe | Zweck |
| --- | --- | --- | --- |
| `ITSM_DATABASE_URL` | ja | — | PostgreSQL; mit eigenen Datenbanken je Mandant die Steuer-Datenbank |
| `ITSM_TENANT_MODE` | nein | `shared` | `shared`: alle Mandanten in einer Datenbank; `database`: jeder Mandant in seiner eigenen (PL47d, Hauptplan E144) |
| `ITSM_TENANT_<NAME>_DATABASE_URL` | mit `database` | — | Datenbank des Mandanten `<name>`, z. B. `ITSM_TENANT_NORD_DATABASE_URL`; fehlt sie für einen Mandanten, startet itsm nicht |
| `ITSM_VALKEY_ADDR` | ja | — | Valkey, `host:port` |
| `ITSM_ROLE` | nein | `all` | `api`, `worker` oder `all` |
| `ITSM_HTTP_ADDR` | nein | `:8080` | Adresse des HTTP-Servers |
| `ITSM_WEB_DIR` | nein | leer | Verzeichnis der gebauten Oberfläche |
| `ITSM_FILES_DIR` | nein | leer, im Image `/data/files` | Ablage der Dateien (PL18) und der NDJSON-Ströme des asynchronen Bulk (PL32); leer schaltet beides ab (503) |
| `ITSM_LOG_LEVEL` | nein | `info` | `debug`, `info`, `warn`, `error` |
| `ITSM_SHUTDOWN_TIMEOUT` | nein | `15s` | Zeit zum geordneten Beenden |
| `ITSM_REQUEST_TIMEOUT` | nein | `10s` | Zeitgrenze einer API-Anfrage samt ihrer Abfragen in PostgreSQL (PL56); `0` für keine; Export, Bulk-Strom, Ereignisstrom und Hochladen ausgenommen |
| `ITSM_EVALUATIONS` | nein | `4` | so viele Auswertungen von Dashboard-Widgets rechnen zugleich je Instanz (PL67); weitere warten auf einen Platz, längstens bis zur Zeitgrenze; `0` für keine Grenze |
| `ITSM_WORKERS_INTERACTIVE` | nein | `8` | Worker je Instanz für die Warteschlange `interactive` (Rolle `worker`) |
| `ITSM_WORKERS_DEFAULT` | nein | `8` | Worker je Instanz für `default` |
| `ITSM_WORKERS_BULK` | nein | `2` | Worker je Instanz für `bulk`; `0` lässt eine Warteschlange auf dieser Instanz liegen |
| `ITSM_WORKERS_DAYS` | nein | `1` | Worker je Instanz für `days`: die Tageswerte der Dashboards jede Minute (PL70), damit sie nicht hinter einer Flut in `default` warten |
| `ITSM_BULK_QUEUE` | nein | `bulk` | Warteschlange des asynchronen Bulk (PL32); die Laststufe der CI nimmt `bulkload`, damit eine andere Instanz auf derselben Datenbank ihre Jobs nicht nimmt |
| `ITSM_EVENT_RETENTION` | nein | `168h` | Aufbewahrung der Ereignisse (Hauptplan E46), nächtlicher Job `event.retention` |
| `ITSM_PUBLIC_URL` | nein | `http://localhost:8080` | Adresse des Servers aus Sicht des Browsers (Rückweg vom Identitätsanbieter) |
| `ITSM_OIDC_ISSUER` | nein | leer | OpenID-Connect-Anbieter (Aussteller); leer = keine Anmeldung über einen Anbieter |
| `ITSM_OIDC_CLIENT_ID` | mit Issuer | — | Client-ID beim Anbieter |
| `ITSM_OIDC_CLIENT_SECRET` | nein | leer | Client-Geheimnis |
| `ITSM_OIDC_NAME` | nein | `SSO` | Beschriftung des Knopfs auf der Anmeldeseite |
| `ITSM_OIDC_INTERNAL_URL` | nein | leer | Anbieter aus Sicht des Containers, wenn anders als für den Browser (Compose: `http://keycloak:8080`) |
| `ITSM_OIDC_SCOPES` | nein | `openid profile email` | Scopes, durch Leerzeichen getrennt |
| `ITSM_OIDC_GROUPS_CLAIM` | nein | `groups` | Name des Claims mit den Gruppen (nur für das Protokoll; Regeln sehen alle Claims) |
| `ITSM_SMTP_HOST` | nein | leer | Mailserver der Benachrichtigungen (WF15); leer = keine Mail, der Posteingang füllt sich trotzdem (WF19). Compose: `mailpit` (Profil `mail`) |
| `ITSM_SMTP_PORT` | nein | `25` | Port des Mailservers (Mailpit: `1025`) |
| `ITSM_SMTP_FROM` | mit Host | — | Absender, etwa `ITSM <itsm@example.org>` |
| `ITSM_SMTP_USER` | nein | leer | Anmeldung am Mailserver; nur mit STARTTLS |
| `ITSM_SMTP_PASSWORD` | nein | leer | Kennwort dazu |
| `ITSM_SMTP_STARTTLS` | nein | `false` | `true` verschlüsselt die Verbindung vor der Anmeldung |
| `ITSM_TRUST_PROXY` | nein | `false` | `true` hinter einem Reverse-Proxy: die Adresse des Aufrufers kommt aus `X-Forwarded-For` (IP-Bereiche der API-Zugänge) |
| `ITSM_LDAP_PASSWORD_<NAME>` | je Verzeichnis | — | Bind-Kennwort des LDAP-Verzeichnisses `<name>` (Großbuchstaben, `-` als `_`); die Verzeichnisse selbst stehen unter `/api/v1/directories` |

## Lizenz

Copyright (C) 2026 group24 AG. AGPL-3.0-only, siehe [LICENSE](LICENSE). Beiträge nur mit
Rechteübertragung an die group24 AG (CLA).

Abhängigkeiten, die das Werkzeug ausliefert, haben nur permissive Lizenzen oder MPL-2.0; die CI
prüft das in jedem Lauf gegen `test/licenses/licenses.txt` (Hauptplan E104).

## English

ServiceFlow25 is an ITSM platform following ITIL 4, built to replace GLPI: a data-driven core on
which every practice is a package. A product of group24 AG; the core is licensed under
AGPL-3.0-only. ServiceFlow25 is the proposed product name; in the code the tool is still called
`itsm`.

- **Status:** work in progress towards milestone 1. The platform core, the workflow engine, the
  web interface with its portal and the service desk packages (incident, request, catalogue,
  knowledge) are built; details in the [main plan](docs/0_maintask.md) (German).
- **Run:** only Docker is required. Run `docker compose up -d --build`, install the packages,
  create a local account with the password on stdin and grant it the role `admin`, as shown
  under [Schnellstart](#schnellstart). The web interface answers at <http://localhost:8092/>.
- **Tenants:** the first tenant adopts what is there, for good. With
  `ITSM_TENANT_MODE=database` every tenant has a database of its own, backed up and restored
  with `pg_dump` and `pg_restore`.
- **Develop:** `scripts/ci-run.sh` builds, lints and tests in the tool container with a fresh
  PostgreSQL and Valkey per run; `scripts/ci-run.sh --part` checks only what changed.
- **Configuration:** environment variables only, listed under [Konfiguration](#konfiguration).
- **Licence:** AGPL-3.0-only, see [LICENSE](LICENSE). Contributions require a CLA with
  group24 AG. Shipped dependencies are permissively licensed or MPL-2.0, checked by the CI.
