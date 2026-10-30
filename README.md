# itsm (Arbeitstitel)

Ein ITSM-Werkzeug nach ITIL 4 als Ablösung von GLPI: eine datengetriebene Plattform, auf der jede
Practice ein Paket ist. Produkt der group24 AG, Kern unter AGPL-3.0-only.

*An ITSM platform following ITIL 4, built to replace GLPI: a data-driven core on which every
practice is a package. A product of group24 AG; the core is licensed under AGPL-3.0-only.*

## Stand / Status

In Arbeit, Stand 2026-10-29 (Einzelheiten im Hauptplan `docs/0_maintask.md`): Der Plattform-Kern
steht mit Meta-Modell, Datensätzen, REST-API und OpenAPI, Organisationsbaum, Rollen und Rechten,
OIDC, LDAP, API-Zugängen, Abfragen, Beziehungen, Listen, Dateien, Versionen, Kosten, Übersetzungen,
Formeln, Jobs, Ereignissen mit Verteiler in Stapeln, Webhooks, Volltextsuche, Sicherheitseinstellungen,
Bulk und der Zeitleiste am Datensatz (öffentliche und private Einträge). Die Workflow-Engine hat Statusmodell, Feldzustände, Rollen am Datensatz, Regeln,
Kalender, SLA-Uhren mit Eskalationsstufen, Wartegründe, Zeitregeln, Aufgaben mit Summen am Vorgang, wiederkehrende Vorgänge und Benachrichtigungen als Mail. Die Oberfläche zeigt Liste und Formular aus der Definition (Basis des Pakets und Kundenschicht mit eigenen Feldern `k_…`) und eine Detailseite mit Zeitleiste, Rollen, SLA-Uhren und Aufgaben; Verweise und Rollen wählt man per Namen; Listen lassen sich filtern, als Suche speichern und teilen und als CSV, XLSX oder ODS exportieren; gewählte Datensätze ändert man in einem Schritt; Einträge und markierte Textfelder sind formatierter Text mit Bildern; Listen und Detailseiten aktualisieren sich live und zeigen, wer gerade am Datensatz ist; Sprache, Zeitzone, Theme und Vorbelegung stellt jede Person selbst ein, Vorgaben je Organisationseinheit. Nutzer ohne Bearbeiterrecht sehen ein schlankes Portal mit ihren Vorgängen und den Formularen, die im Baukasten mit Abschnitten, Fragen, Bedingungen und englischen Texten entstehen. Kein Service Desk bisher.

*Work in progress (details in the main plan): the platform core, the first part of the workflow
engine and list and form in the web interface are built; no service desk yet.*

## Starten / Run

Alle Befehle in WSL im Verzeichnis des Repositorys; auf dem Rechner braucht es nur Docker
(Hauptplan E52). *All commands from the repository root; only Docker is required.*

```bash
docker compose up -d --build
```

Danach antworten / then these answer:

- http://localhost:8092/ — die Oberfläche / the web interface
- http://localhost:8092/readyz — Datenbank und Valkey / database and Valkey
- http://localhost:8092/api/v1/version — die Version / the version

Anmelden (PL5): Alles unter `/api/v1` außer `version` und `auth/login` braucht eine Sitzung.
Ein lokales Konto legt `itsm user create` an, das Kennwort kommt auf stdin (mindestens 8
Zeichen mit Groß-, Kleinbuchstabe, Ziffer und Sonderzeichen); schreibende Aufrufe tragen den
Header `X-Requested-With: itsm`. *Sign in: everything under `/api/v1` except `version` and
`auth/login` needs a session; create a local account with the password on stdin.*

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/core
```

```bash
docker compose run --rm tools go run ./cmd/itsm package install packages/probe
```

```bash
printf 'Secret-1!' | docker compose run --rm -T itsm user create pascal --display-name "Pascal"
```

Ohne Rolle sieht ein Konto nichts (PL8); die Rolle `admin` darf alles:

```bash
docker compose run --rm itsm user grant pascal admin
```

Mandanten (PL44): Ohne Mandanten ist die Wurzel der einzige. Der erste übernimmt mit `--adopt`
alles Bestehende; die mit `--admin` genannten Konten bleiben Konten der Installation und wechseln
den Mandanten im Kopf der Oberfläche. Die Übernahme lässt sich nicht zurücknehmen. *Tenants: the
first one adopts what is there (for good); named administrators stay with the installation and
switch tenants in the header.*

```bash
docker compose run --rm itsm tenant create nord --label "Polizei Nord" --adopt --admin pascal
```

```bash
docker compose run --rm itsm tenant create sued --label "Polizei Süd"
```

```bash
curl -s -c cookies.txt -H 'Content-Type: application/json' -H 'X-Requested-With: itsm' \
  -d '{"username":"pascal","password":"Secret-1!"}' http://localhost:8092/api/v1/auth/login
```

```bash
curl -s -b cookies.txt http://localhost:8092/api/v1/auth/me
```

Bauen, prüfen und testen im Werkzeug-Container / build, lint and test in the tool container:

```bash
docker compose run --rm tools scripts/ci.sh
```

Mails der Benachrichtigungen ansehen (Mailpit, Profil `mail`) / see the mails of the
notifications:

```bash
docker compose --profile mail up -d
```

- http://localhost:8026/ — Mailpit (8025 ist der Mailpit der GLPI-Umgebung)

Leistung messen (Hauptplan H11): Die CI misst bei jedem Lauf den neuen Stand im Wechsel mit
dem freigegebenen Basis-Binary aus dem Volume `load-baseline`; `test/load/baseline.json` ist das
Protokoll der Freigabe. Ist der Rechner ausgelastet, bricht die Messung ab (Rückgabe 3). Die volle
Stufe und der Datensatz in Prod-Größe:

```bash
docker compose run --rm tools go run ./test/load generate --size=prod --out=/tmp/data
```

## Aufbau / Layout

| Pfad | Inhalt |
| --- | --- |
| `cmd/itsm/` | das eine Binary mit `serve`, `migrate`, `package`, `user`, `ldap`, `client`, `calendar`, `healthcheck`, `version` (`itsm` ohne Argument zeigt alle) |
| `internal/` | die Bausteine aus `docs/architektur.md` §2 |
| `db/migrations/`, `db/queries/` | Migrationen (`goose`) und Abfragen (`sqlc`) der Kerntabellen |
| `web/` | die Oberfläche (React, TypeScript, Vite) |
| `tools/` | der Werkzeug-Container |
| `scripts/` | CI und Prüfskripte |
| `test/load/` | Lastwerkzeug, Datensatzgenerator, Basislinie |
| `docs/` | Hauptplan, Architektur, Entscheidungen (`docs/adr/`) |

## Konfiguration / Configuration

Nur über Umgebungsvariablen / environment variables only:

| Variable | Pflicht | Vorgabe | Zweck |
| --- | --- | --- | --- |
| `ITSM_DATABASE_URL` | ja | — | PostgreSQL |
| `ITSM_VALKEY_ADDR` | ja | — | Valkey, `host:port` |
| `ITSM_ROLE` | nein | `all` | `api`, `worker` oder `all` |
| `ITSM_HTTP_ADDR` | nein | `:8080` | Adresse des HTTP-Servers |
| `ITSM_WEB_DIR` | nein | leer | Verzeichnis der gebauten Oberfläche |
| `ITSM_FILES_DIR` | nein | leer, im Image `/data/files` | Ablage der Dateien (PL18) und der NDJSON-Ströme des asynchronen Bulk (PL32); leer schaltet beides ab (503) |
| `ITSM_LOG_LEVEL` | nein | `info` | `debug`, `info`, `warn`, `error` |
| `ITSM_SHUTDOWN_TIMEOUT` | nein | `15s` | Zeit zum geordneten Beenden |
| `ITSM_WORKERS_INTERACTIVE` | nein | `8` | Worker je Instanz für die Warteschlange `interactive` (Rolle `worker`) |
| `ITSM_WORKERS_DEFAULT` | nein | `8` | Worker je Instanz für `default` |
| `ITSM_WORKERS_BULK` | nein | `2` | Worker je Instanz für `bulk`; `0` lässt eine Warteschlange auf dieser Instanz liegen |
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
| `ITSM_SMTP_HOST` | nein | leer | Mailserver der Benachrichtigungen (WF15); leer = keine Mail. Compose: `mailpit` (Profil `mail`) |
| `ITSM_SMTP_PORT` | nein | `25` | Port des Mailservers (Mailpit: `1025`) |
| `ITSM_SMTP_FROM` | mit Host | — | Absender, etwa `ITSM <itsm@example.org>` |
| `ITSM_SMTP_USER` | nein | leer | Anmeldung am Mailserver; nur mit STARTTLS |
| `ITSM_SMTP_PASSWORD` | nein | leer | Kennwort dazu |
| `ITSM_SMTP_STARTTLS` | nein | `false` | `true` verschlüsselt die Verbindung vor der Anmeldung |
| `ITSM_TRUST_PROXY` | nein | `false` | `true` hinter einem Reverse-Proxy: die Adresse des Aufrufers kommt aus `X-Forwarded-For` (IP-Bereiche der API-Zugänge) |
| `ITSM_LDAP_PASSWORD_<NAME>` | je Verzeichnis | — | Bind-Kennwort des LDAP-Verzeichnisses `<name>` (Großbuchstaben, `-` als `_`); die Verzeichnisse selbst stehen unter `/api/v1/directories` |

## Lizenz / License

Copyright (C) 2026 group24 AG. AGPL-3.0-only, siehe / see [LICENSE](LICENSE). Beiträge nur mit
Rechteübertragung an die group24 AG (CLA). *Contributions require a CLA with group24 AG.*

Abhängigkeiten, die itsm ausliefert, haben nur permissive Lizenzen oder MPL-2.0; die CI prüft
das in jedem Lauf gegen `test/licenses/licenses.txt` (Hauptplan E104). *Shipped dependencies
are permissively licensed or MPL-2.0, checked by the CI.*
