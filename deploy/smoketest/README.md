# Smoke test — Server, TLS und Login schrittweise aufsetzen (ohne plexams)

Zwei kleine, eigenständige Docker-Compose-Stacks, um das Fundament zu validieren,
**bevor** plexams dahinter kommt. Jeder Schritt ist für sich lauffähig.

- **`1-tls/`** — nur **SSL/ACME**. nginx liefert ein statisches „Hello World" über
  HTTPS. Beweist DNS-Name + Zertifikat (acme.sh/EAB) + Reverse-Proxy-Host.
- **`2-login/`** — zusätzlich **OIDC-Login** (nginx + oauth2-proxy gegen sso.hm.edu).
  Nach erfolgreichem Login zeigt der Server „Hallo &lt;email&gt;" und die übertragenen
  Attribute. Bewusst unkritisch: jeder, den der IdP anmeldet, darf rein und sieht nur
  seine eigenen Daten (keine Allow-Liste).

Beide brauchen von der Zentralen IT nur, was auch der echte Stack braucht (für ACME:
**ACME-Directory-URL + EAB `kid`/`hmac`**; für Schritt 2 zusätzlich den **OIDC-Client**
mit Redirect-URI `https://<host>/oauth2/callback`).

---

## Schritt 1 — Hello World über SSL (`1-tls/`)

```bash
cd smoketest/1-tls
cp .env.example .env            # SERVER_NAME eintragen
cp acme.env.example acme.env    # ACME-Directory-URL + EAB kid/hmac (von der IT)
```

**Wichtig: `tls/` und `acme-webroot/` VOR `docker compose up` anlegen** — sonst legt
Docker die Bind-Mount-Verzeichnisse als `root` an und acme.sh (als dein User) kann den
Challenge-Token nicht hineinschreiben (`Permission denied`):
```bash
mkdir -p tls acme-webroot
```

**Henne-Ei:** nginx braucht beim Start ein Zertifikat, das es noch nicht gibt. Zuerst
ein selbstsigniertes Platzhalter-Zertifikat anlegen (Browser zeigt dann kurz eine
Warnung — das ersetzt Schritt 4):
```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 3 \
  -keyout tls/privkey.pem -out tls/fullchain.pem \
  -subj "/CN=$(grep '^SERVER_NAME=' .env | cut -d= -f2)"
```

> Ist es schon zu spät und `acme-webroot/` gehört root? Dann einmalig
> `sudo chown -R "$(id -un):$(id -gn)" tls acme-webroot`.

Starten und echtes Zertifikat holen:
```bash
docker compose up -d
./acme-setup.sh                 # acme.sh: EAB-Account + HTTP-01 + install + nginx-reload
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
cp .env.example .env            # SERVER_NAME + OIDC-Client + Cookie-Secret (openssl rand -base64 24 → 32 Zeichen)
cp acme.env.example acme.env
```
Verzeichnisse anlegen (wie in Schritt 1, vor `docker compose up`), dann Zertifikat:
entweder das aus Schritt 1 nach `2-login/tls/` kopieren, oder den Henne-Ei-Bootstrap
+ `./acme-setup.sh` hier wiederholen.

```bash
mkdir -p tls acme-webroot
docker compose up -d
./acme-setup.sh                 # falls hier ein frisches Zertifikat nötig ist
```

`https://<dein-host>/` öffnen → Weiterleitung zu sso.hm.edu → nach dem Login zeigt die
Seite **„Hallo &lt;deine-email&gt;"** samt `email` / `sub` / `preferred_username` /
`groups`. Die vollständigen Session-Claims als JSON gibt es unter `/oauth2/userinfo`,
Abmelden unter `/oauth2/sign_out`.

> Hinweis: oauth2-proxy stellt standardmäßig `email`, `sub`, `preferred_username` und
> `groups` bereit — das reicht, um zu bestätigen, dass die Anmeldung funktioniert und
> welche Identität ankommt. Braucht ihr die *komplette* Liste der vom IdP freigegebenen
> Attribute, kann man oauth2-proxy zusätzlich das ID-Token durchreichen lassen.

Wenn beide Schritte laufen, ist alles bereit für den echten Stack in [`../`](../) —
dort kommt plexams dazu und die `users`-Allow-Liste macht aus „jeder darf rein" ein
kontrolliertes „nur bekannte User".
