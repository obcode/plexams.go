# plexams.go — Server-Deployment (Docker Compose, nginx + oauth2-proxy, OIDC via HM)

Betreibt plexams.go hinter **nginx**, das die Authentifizierung **über OIDC gegen
`sso.hm.edu`** per **`oauth2-proxy`** macht (`auth_request`). nginx setzt die
verifizierte Identität autoritativ als Header `X-Remote-User`; das Backend vertraut
diesem Header und erzwingt die Autorisierung (Rollen) selbst. TLS via **acme.sh**
gegen die HM-ACME-CA (mit **EAB**).

```
Internet ──443/TLS──> nginx (auth_request → oauth2-proxy → sso.hm.edu)
                        ├── /oauth2/*    → oauth2-proxy (Login/Callback)
                        ├── /            → gui (plexams.gui-Container)
                        └── /query,/upload,/download → plexams.go:8080
plexams.go / gui / mongo / oauth2-proxy ── nur im compose-Netz (nicht veröffentlicht)
```

Backend **und** GUI laufen als **fertige Images von `ghcr.io`** (`plexams.go` bzw.
`plexams.gui`); es wird nichts mehr lokal gebaut. Neue Releases werden von einem
**Self-hosted GitHub-Runner auf dem Host** automatisch ausgerollt (s. u.
[Automatischer Deploy](#automatischer-deploy-ci--self-hosted-runner)).

## ⚑ Was die Zentrale IT (IdP-Team) braucht

Für die OIDC-Client-Registrierung an `sso.hm.edu`:

| Feld | Wert |
|------|------|
| **Redirect / Callback URI** | `https://<DEIN-HOST>/oauth2/callback` |
| Grant type | `authorization_code` |
| Scopes | `openid profile email department` (`department` = client-spezifischer Custom-Scope → Claim `fhmDepartment`) |
| Post-Logout Redirect | — nicht nötig (kein Logout im Stack) |

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
- Zugriff auf die ghcr-Images `ghcr.io/obcode/plexams.go` + `ghcr.io/obcode/plexams.gui`
  (öffentlich → kein Login nötig; privat → einmalig `docker login ghcr.io`).

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

2. **Image-Tags** in `.env` setzen: `PLEXAMS_TAG` / `GUI_TAG` (Default `latest`, oder
   eine konkrete Release-Version für einen reproduzierbaren Erststart). Der automatische
   Deploy pinnt sie später auf die jeweils veröffentlichte Version.

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

4. **Stack starten** (zieht die Images und startet alle Services):
   ```bash
   docker compose pull
   docker compose up -d
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

## Automatischer Deploy (CI + Self-hosted Runner)

Der Server ist nur im VPN erreichbar, GitHub kann also **nicht** hineinpushen. Statt­dessen
läuft ein **Self-hosted GitHub Actions Runner auf dem Deploy-Host**, der ausgehend zu
GitHub pollt und bei einem Release lokal `docker compose` ausführt.

**Ablauf pro Release:** Conventional-Commit → semantic-release schneidet ein GitHub-Release
→ der Image-Build (`.github/workflows/docker.yml`) pusht nach ghcr.io → der `deploy`-Job
im selben Workflow läuft auf dem Runner, spiegelt die Nicht-Secret-Infra (compose-Datei +
nginx-Templates) in das on-host Deploy-Verzeichnis, pinnt `PLEXAMS_TAG` in `.env` auf die
Release-Version und macht `docker compose pull plexams && up -d`. plexams.gui hat im eigenen
Repo einen spiegelbildlichen Workflow, der `GUI_TAG` pinnt und den `gui`-Service neu zieht.

**Runner einrichten (einmalig, auf dem Host, als User `plexams`):**
```sh
# plexams muss in der docker-Gruppe sein:
mkdir -p ~/actions-runner && cd ~/actions-runner       # → /home/plexams/actions-runner
# Runner-Tarball von GitHub → Repo → Settings → Actions → Runners → New self-hosted runner
./config.sh --url https://github.com/obcode/plexams.go \
            --token <RUNNER-TOKEN> --labels plexams-deploy --unattended
sudo ./svc.sh install plexams && sudo ./svc.sh start
```
- **Freischalten:** Der `deploy`-Job ist hinter der Repo-Variable **`AUTO_DEPLOY`** gegated
  (Settings → Secrets and variables → Actions → Variables → `AUTO_DEPLOY=true`). Erst danach
  läuft er — vorher wird das Image weiterhin gebaut+gepusht, der Deploy-Job aber übersprungen
  (kein hängender Queue-Job, solange der Runner fehlt).
- Das on-host Deploy-Verzeichnis ist standardmäßig `/home/plexams/deploy` (überschreibbar
  über die Repo-Variable `DEPLOY_DIR`). Dort liegen die Secrets (`.env`, `.plexams.yaml`,
  `tls/`) — der Job fasst sie **nie** an, er synct nur compose-Datei + nginx-Templates.
- Für **plexams.gui** denselben Runner/Label wiederverwenden (Runner zusätzlich auf das
  GUI-Repo registrieren) und dort ebenfalls `AUTO_DEPLOY=true` setzen — beide Workflows
  arbeiten auf demselben Stack und fassen jeweils nur ihren eigenen Service an.
- **Sicherheit:** der Runner führt Workflow-Code auf dem Prod-Host aus → Trigger strikt auf
  `release` beschränkt (kein PR-Code), Runner nur für diese beiden Repos.

**Rollback:** in `/home/plexams/deploy/.env` `PLEXAMS_TAG` (bzw. `GUI_TAG`) auf eine ältere
Version setzen und `docker compose up -d plexams` (bzw. `gui`) — die ghcr-Images sind
versioniert vorhanden.

## Betrieb

- Logs: `docker compose logs -f nginx` / `... oauth2-proxy` / `... plexams` / `... gui`.
- Manuelles Update ohne CI: `docker compose pull && docker compose up -d`.
- Zertifikat-Renew testen: `~/.acme.sh/acme.sh --renew -d <host> --force`.
- Mongo-Backup: Volume `mongo-data` sichern oder das Semester über die GUI als ZIP dumpen.
