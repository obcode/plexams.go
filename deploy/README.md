# plexams.go — Server-Deployment (Docker Compose, nginx + oauth2-proxy, OIDC via HM)

Betreibt plexams.go hinter **nginx**, das die Authentifizierung **über OIDC gegen
`sso.hm.edu`** per **`oauth2-proxy`** macht (`auth_request`). nginx setzt die
verifizierte Identität autoritativ als Header `X-Remote-User`; das Backend vertraut
diesem Header und erzwingt die Autorisierung (Rollen) selbst. TLS via **acme.sh**
gegen die HM-ACME-CA (mit **EAB**).

```
Internet ──443/TLS──> nginx (auth_request → oauth2-proxy → sso.hm.edu)
                        ├── /oauth2/*    → oauth2-proxy (Login/Callback)
                        ├── /            → plexams.gui (statische dist)
                        └── /query,/upload,/download → plexams.go:8080
plexams.go / mongo / oauth2-proxy ── nur im compose-Netz (nicht veröffentlicht)
```

## ⚑ Was die Zentrale IT (IdP-Team) braucht

Für die OIDC-Client-Registrierung an `sso.hm.edu`:

| Feld | Wert |
|------|------|
| **Redirect / Callback URI** | `https://<DEIN-HOST>/oauth2/callback` |
| Grant type | `authorization_code` |
| Scopes | `openid profile email department` (`department` = client-spezifischer Custom-Scope → Claim `fhmDepartment`) |
| Post-Logout Redirect | — nicht nötig (Abmelden ist rein lokal, siehe unten) |

> Die **Redirect-URI ist exakt** `https://<DEIN-HOST>/oauth2/callback` (das ist der
> oauth2-proxy-Standardpfad — **nicht** `/redirect_uri`). `<DEIN-HOST>` = endgültiger
> DNS-Name (z. B. `https://plexams.cs.hm.edu/oauth2/callback`). Muss **zeichengenau**
> mit `OAUTH2_PROXY_REDIRECT_URL` (aus `SERVER_NAME`) übereinstimmen.

Zurück bekommst du **Client-ID** + **Client-Secret** → in `.env`.

Für **TLS/ACME** brauchst du außerdem von der IT die **ACME-Directory-URL** und die
**EAB-Zugangsdaten** (`kid` + `hmac-key`) — die HM-CA nutzt External Account Binding.

> **Tipp:** Erst das Fundament einzeln testen — [`smoketest/`](smoketest/) bringt in
> zwei Mini-Stacks nacheinander (1) TLS/ACME mit „Hello World" und (2) den
> OIDC-Login mit „Hallo + Attribute" zum Laufen, **ohne** plexams dahinter. Wenn beide
> grün sind, ist dieser echte Stack fast nur noch Config.

## Voraussetzungen

- Docker + Docker Compose auf dem (Alpine-)Server; `acme.sh` auf dem Host.
- DNS-Name, der auf den Server zeigt; Port 80 + 443 aus dem Netz der HM-CA erreichbar
  (für die HTTP-01-Challenge).
- OIDC-Client registriert (s. o.) — **frühzeitig anstoßen**, längste Vorlaufzeit.
- Der Build von **plexams.gui** (`npm run build` → `dist/`).

## Einrichtung

1. **Konfig anlegen** (nichts davon wird committet):
   ```bash
   cd deploy
   cp .env.example        .env            # Mongo-Creds, SERVER_NAME, OIDC-Client, Cookie-Secret
   cp acme.env.example    acme.env        # ACME-Directory-URL + EAB kid/hmac (von der IT)
   cp .plexams.yaml.example .plexams.yaml # db.uri, auth.seedusers (die zwei ADMINs), zpa/smtp/…
   ```
   `OAUTH2_PROXY_COOKIE_SECRET` z. B. mit `openssl rand -base64 24` (ergibt 32 Zeichen;
   oauth2-proxy verlangt 16/24/32 Zeichen, `-base64 32` liefert 44 und schlägt fehl). Denselben Host in
   `.env` `SERVER_NAME` ↔ `.plexams.yaml` `server.allowedorigins` verwenden, dieselbe
   Mongo-Passphrase in `.env` `MONGO_PASSWORD` ↔ `.plexams.yaml` `db.uri`.

2. **GUI-Build** bereitstellen:
   ```bash
   cp -r /pfad/plexams.gui/dist ./gui-dist
   ```

3. **Erststart (Henne-Ei beim Zertifikat):** nginx lädt beim Start ein Zertifikat,
   das es noch nicht gibt. **`tls/` und `acme-webroot/` VOR `docker compose up`
   anlegen** — sonst legt Docker die Bind-Mount-Verzeichnisse als `root` an und acme.sh
   kann den Challenge-Token nicht hineinschreiben. Dann ein selbstsigniertes
   Platzhalter-Zertifikat, damit nginx hochkommt und die HTTP-01-Challenge ausliefert:
   ```bash
   mkdir -p tls acme-webroot
   openssl req -x509 -newkey rsa:2048 -nodes -days 3 \
     -keyout tls/privkey.pem -out tls/fullchain.pem \
     -subj "/CN=$(grep '^SERVER_NAME=' .env | cut -d= -f2)"
   ```
   > Schon zu spät und `acme-webroot/` gehört root?
   > `sudo chown -R "$(id -un):$(id -gn)" tls acme-webroot`.

4. **Stack starten**:
   ```bash
   docker compose up -d --build
   ```

5. **Echtes Zertifikat holen** (acme.sh, HTTP-01 über nginx, EAB):
   ```bash
   ./acme-setup.sh
   ```
   Das registriert den ACME-Account mit EAB, holt das Zertifikat über
   `acme-webroot/`, installiert es nach `tls/` und lädt nginx neu. Renewals macht der
   acme.sh-Cron automatisch (mit `--reloadcmd`).

6. **Erst-Boot der App**: `auth.seedusers` legt die beiden Planer als `ADMIN` an (nur
   solange die `users`-Collection leer ist). Danach werden User über die GUI verwaltet
   (`setUser`/`removeUser`).

## Rollen & erweiterter Kreis

Ein User hat genau **eine** Rolle; Hierarchie **`ADMIN` ⊇ `PLANER` ⊇ `VIEWER`**:
- **`VIEWER`** — lesen + Validierungen, **keine** datenändernden Operationen (Backend).
- **`PLANER`** — volle Planung.
- **`ADMIN`** — wie PLANER **plus** Benutzerverwaltung (`setUser`/`removeUser`).

Zum Öffnen für einen größeren Kreis neue User mit `VIEWER` anlegen. Mindestens ein
geseedeter User muss `ADMIN` sein, damit später jemand User über die GUI verwalten
kann. Feinere Rechte später über eine `@requires`-Directive pro Feld.

## Sicherheits-Kernregeln

- **Backend nie veröffentlichen.** `plexams` (und `mongo`, `oauth2-proxy`) haben
  bewusst kein `ports:` — nur über nginx erreichbar. Sonst könnte jeder den
  `X-Remote-User`-Header selbst setzen.
- nginx setzt `X-Remote-User` per `proxy_set_header` autoritativ (überschreibt jeden
  Client-Wert) aus dem verifizierten `email`-Claim.
- `server.production: true` schaltet Playground + Introspection ab.
- Secrets nur in `.env` / `acme.env` / `.plexams.yaml` (alle gitignored), nie in der
  DB, nie im Image.

## Lokale Entwicklung (unverändert)

Ohne `auth.enabled: true` läuft plexams.go wie bisher: ein voll berechtigter (ADMIN)
Dev-User wird injiziert, nichts wird abgewiesen. Kein nginx, kein OIDC nötig.

## Firewall (awall) + Docker

Auf Alpine teilen sich **awall** und Docker die iptables. Jedes `awall activate` baut
die Tabellen neu auf und entfernt dabei **Dockers eigene Ketten** (`DOCKER-FORWARD`,
`DOCKER-USER`, …). Symptom beim nächsten `docker compose up`:

```
Failed to Setup IP tables: ... iptables ... -A DOCKER-FORWARD ...:
iptables: No chain/target/match by that name.
```

Fix: den Docker-Daemon neu starten, damit er seine Ketten neu anlegt:
```sh
sudo rc-service docker restart
docker compose up -d
```

**Merksatz:** Reihenfolge immer `awall activate` → `rc-service docker restart` →
`docker compose up -d`. Deine awall-INPUT-Regeln (22/80/443) bleiben dabei erhalten;
der Docker-Restart legt nur die Docker-Ketten wieder obendrauf. Beim Booten unkritisch
(Docker startet nach der Firewall), nur bei manueller Neu-Aktivierung nötig.

## Betrieb

- Logs: `docker compose logs -f nginx` / `... oauth2-proxy` / `... plexams`.
- Zertifikat-Renew testen: `~/.acme.sh/acme.sh --renew -d <host> --force`.
- Mongo-Backup: Volume `mongo-data` sichern oder das Semester über die GUI als ZIP dumpen.
