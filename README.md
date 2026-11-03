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
Kalender, SLA-Uhren mit Eskalationsstufen, Wartegründe, Zeitregeln, Aufgaben mit Summen am Vorgang, wiederkehrende Vorgänge und Benachrichtigungen als Mail. Die Oberfläche zeigt Liste und Formular aus der Definition (Basis des Pakets und Kundenschicht mit eigenen Feldern `k_…`, die ein Admin im Typ- und Feld-Editor anlegt, umbenennt, ausblendet und verschiebt; im Workflow-Editor ergänzt er Status und Übergänge mit Bedingung, legt Übergänge des Pakets still, wählt den Anfangsstatus und sieht das Ganze als Graph mit React Flow, in dem er Übergänge markiert, Verbindungen zieht und die Anordnung speichert; im Rollen-Editor stellt er je Typ und Aktion die Reichweite ein, schränkt mit Bedingung und Feldrechten ein, vergibt Sonderrechte und weist die Rolle Personen und Gruppen an Knoten zu) und eine Detailseite mit Zeitleiste, Rollen, SLA-Uhren und Aufgaben; Verweise und Rollen wählt man per Namen; Listen lassen sich filtern, als Suche speichern und teilen und als CSV, XLSX oder ODS exportieren; gewählte Datensätze ändert man in einem Schritt; Einträge und markierte Textfelder sind formatierter Text mit Bildern; Listen und Detailseiten aktualisieren sich live und zeigen, wer gerade am Datensatz ist; Sprache, Zeitzone, Theme, hell oder dunkel und Vorbelegung stellt jede Person selbst ein, Vorgaben je Organisationseinheit; das Aussehen folgt dem Design-Kit „Klartext“ mit der Schrift IBM Plex, die das Werkzeug selbst ausliefert; die Navigation zeigt Bereiche und gespeicherte Suchen, die Befehlszeile (Strg K oder das Suchfeld im Kopf) findet Datensätze über alle Typen, legt neue an, wechselt die Darstellung und führt zu jeder Seite, die Liste hat eine Vorschau des gewählten Datensatzes, kompakte oder normale Zeilen und bei Typen mit SLA eine Spalte „Frist“, nach der man sortiert und filtert (überschritten, knapp, fällig bis). Nutzer ohne Bearbeiterrecht sehen ein schlankes Portal mit ihren Vorgängen und den Formularen, die im Baukasten mit Abschnitten, Fragen, Bedingungen und englischen Texten entstehen. Ein Typ kann eine laufende Nummer je Mandant haben (etwa `INC-24817`), die der Kern vergibt und die Suche findet. Der Service Desk beginnt mit dem Grundpaket `sd`: Kategorien als Baum, sichtbar je Organisationseinheit, Anfragequelle, Lösungsart und das Merkmal Vorgang mit Feldern und Rollen; Störung und Anfrage folgen.

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

Eindeutige Felder und externe Schlüssel gelten je Mandant (PL46). Ein neuer Datensatz landet im
Knoten des Rechtekontexts, der Person oder des einzigen Anlegerechts; bleibt eine Wahl, fragt das
Formular „Ablegen in“. *Unique fields and external keys are unique per tenant; a new record lands
in the node of the context, the person or the one right, else the form asks.*

```bash
docker compose run --rm itsm tenant create nord --label "Polizei Nord" --adopt --admin pascal
```

```bash
docker compose run --rm itsm tenant create sued --label "Polizei Süd"
```

Datenbank je Mandant (PL47d): Mit `ITSM_TENANT_MODE=database` liegt jeder Mandant in seiner
eigenen Datenbank; die Steuer-Datenbank (`ITSM_DATABASE_URL`) kennt ihn nur beim Namen. Der
Betrieb legt die leere Datenbank samt Benutzer an und trägt ihre URL ein; `tenant create` richtet
sie ein (Migrationen, Tabellen aller Pakete, Auswahllisten) und trägt den Mandanten zuletzt ins
Verzeichnis. Danach die Instanzen neu starten. `migrate` und `package install` wirken auf jede
Datenbank. Gesichert wird je Datenbank mit `pg_dump`, wiederhergestellt mit `pg_restore` in eine
neue, leere Datenbank, auf die dann die URL zeigt; `tenant check` prüft danach, ob sie zur
Steuer-Datenbank passt, und nennt, was fehlt (etwa Konten, die nach der Sicherung angelegt
wurden). *Database per tenant: the operator creates the empty database and sets its URL; tenant
create sets it up; back up and restore per database with pg_dump and pg_restore, then tenant
check.*

Auswertungen über Mandanten (PL48): `POST /api/v1/tenants:query` mit Typ, Mandanten und
JSON-Abfrage liefert die Zeilen jedes Mandanten mit der Spalte `tenant`. Dafür braucht ein Konto
der Installation das Recht `report_tenants` an `core.tenant`, ausdrücklich in einer Rolle an der
Wurzel; die Rolle admin allein reicht nicht. *Evaluations across tenants need the right
report_tenants on core.tenant, named explicitly in a role at the root.*

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord docker compose run --rm itsm tenant create nord --label "Polizei Nord"
```

```bash
pg_dump --format=custom --file=nord.dump postgres://nord:…@db/itsm_nord
```

```bash
pg_restore --no-owner --exit-on-error --dbname=postgres://nord:…@db/itsm_nord_neu nord.dump
```

```bash
ITSM_TENANT_MODE=database ITSM_TENANT_NORD_DATABASE_URL=postgres://nord:…@db/itsm_nord_neu docker compose run --rm itsm tenant check nord
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
scripts/ci-run.sh
```

Der Lauf bekommt einen eigenen PostgreSQL und Valkey (Profil `ci`, Daten im Arbeitsspeicher),
die danach entfernt werden: jeder Lauf beginnt mit demselben Stand, die Instanz auf 8092 bleibt
unberührt. Die CI bricht nicht beim ersten Fehler ab; am Ende stehen alle Schritte mit Ergebnis
und die Fehler. *Every run gets a fresh PostgreSQL and Valkey of its own, removed afterwards;
all steps run, the summary lists every failure.* Einzelne Tests von Hand brauchen die Server:
`docker compose --profile ci up -d --wait postgres-ci valkey-ci`, danach
`docker compose run --rm tools go test ./internal/events/`.

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
