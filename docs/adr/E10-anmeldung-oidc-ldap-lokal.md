# E10: Anmeldung über OIDC/SAML, LDAP-Sync und lokale Konten

- **Status:** angenommen am 2026-10-20, Pascal; Hauptplan §1, E10
- **Architektur:** [`architektur.md`](../architektur.md) §2, §5

## Kontext

70.000 Nutzer aus dem Verzeichnis, Single Sign-on über den Identitätsanbieter des Kunden, dazu ein Notfallzugang.

## Entscheidung

Anmeldung über **OIDC/SAML** (Keycloak, ADFS, Entra), **Nutzer- und Gruppensync aus LDAP/AD**, dazu **lokale Konten** für Admin und Notfall; Rollen und Rechte bleiben im Werkzeug

## Verworfen

nur LDAP; nur OIDC

## Folgen

Rollen und Rechte bleiben im Werkzeug; Regeln weisen sie bei der Anmeldung aus Gruppen zu. Mail-Anmeldung und Webserver-Variablen entfallen (E27, E30).
