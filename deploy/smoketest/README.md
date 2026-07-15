# Smoke test — Server, TLS und Login schrittweise aufsetzen (ohne plexams)

Zwei kleine, eigenständige Docker-Compose-Stacks, um das Fundament zu validieren,
**bevor** plexams dahinter kommt. Jeder Schritt ist für sich lauffähig.

- **`1-tls/`** — nur **SSL/ACME**. Caddy liefert ein statisches „Hello World" über
  HTTPS und holt das Zertifikat selbst (ACME/EAB). Beweist DNS-Name + Zertifikat +
  Reverse-Proxy-Host.
- **`2-login/`** — zusätzlich **OIDC-Login** (Caddy `forward_auth` + oauth2-proxy gegen
  sso.hm.edu). Nach erfolgreichem Login zeigt der Server „Hallo &lt;email&gt;" und die
  übertragenen Attribute. Bewusst unkritisch: jeder, den der IdP anmeldet, darf rein und
  sieht nur seine eigenen Daten (keine Allow-Liste).

Beide brauchen von der Zentralen IT nur, was auch der echte Stack braucht (für ACME:
**ACME-Directory-URL + EAB `kid`/`hmac`**; für Schritt 2 zusätzlich den **OIDC-Client**
mit Redirect-URI `https://<host>/oauth2/callback`).

---

## Schritt 1 — Hello World über SSL (`1-tls/`)

```bash
cd smoketest/1-tls
cp .env.example .env            # SERVER_NAME + ACME_* (Directory-URL + EAB kid/hmac) eintragen
docker compose up -d
```

**Caddy holt das Zertifikat beim ersten Start selbst** — registriert den ACME-Account mit
EAB gegen die HM-CA, beantwortet die HTTP-01-Challenge auf Port 80 und bedient danach
`:443`. Kein selbstsigniertes Platzhalter-Cert, kein `acme-setup.sh`, kein Renew-Cron.
Cert + ACME-Registrierung liegen im `caddy-data`-Volume.

```bash
docker compose logs -f caddy    # ACME-Registrierung / Ausstellung verfolgen
```

> Scheitert `docker compose up` mit `Failed to Setup IP tables … DOCKER-FORWARD …
> No chain/target/match`? Dann hat ein vorheriges `awall activate` Dockers iptables-
> Ketten entfernt. Fix: `sudo rc-service docker restart`, dann erneut hochfahren.
> Reihenfolge merken: `awall activate` → `rc-service docker restart` → `compose up`.
> (Details in [../README.md](../README.md#firewall-awall--docker).)

Jetzt `https://<dein-host>/` im Browser öffnen → **„Hello World 🔒"** ohne
Zertifikatswarnung. Damit steht TLS.

```bash
docker compose down             # aufräumen, wenn du zu Schritt 2 gehst
```

---

## Schritt 2 — Login-Test mit Shibboleth/OIDC (`2-login/`)

Voraussetzung: der OIDC-Client ist bei der IT registriert (Redirect-URI
`https://<host>/oauth2/callback`), du hast Client-ID + Secret.

```bash
cd smoketest/2-login
cp .env.example .env            # SERVER_NAME + ACME_* + OIDC-Client + Cookie-Secret (openssl rand -base64 24 → 32 Zeichen)
docker compose up -d            # Caddy holt das Zertifikat wieder selbst
```

`https://<dein-host>/` öffnen → Weiterleitung zu sso.hm.edu → nach dem Login zeigt die
Seite **„Hallo &lt;deine-email&gt;"** samt `email` / `sub` / `preferred_username` /
`department` (Claim `fhmDepartment` über den groups-Slot). Die vollständigen
Session-Claims als JSON gibt es unter `/oauth2/userinfo`, dazu ein fertiger `curl` gegen den
IdP-UserInfo-Endpoint für die volle Attributliste.

> Hinweis: oauth2-proxy stellt standardmäßig `email`, `sub`, `preferred_username` und
> `groups` bereit — das reicht, um zu bestätigen, dass die Anmeldung funktioniert und
> welche Identität ankommt. `copy_headers` in der Caddyfile reicht genau diese Header (plus
> das Diagnose-Access-Token) an die Begrüßungsseite weiter.

Wenn beide Schritte laufen, ist alles bereit für den echten Stack in [`../`](../) —
dort kommt plexams dazu und die `users`-Allow-Liste macht aus „jeder darf rein" ein
kontrolliertes „nur bekannte User".
