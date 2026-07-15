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
   `.env`: `PUBLIC_PLEXAMS_SERVER=https://<SERVER_NAME>/query` (Browser) und
   `PLEXAMS_SERVER=http://plexams:8080/query` (SSR, intern) — **zwei verschiedene URLs**,
   weil nur der Browser-Hop das OIDC-Cookie trägt (Details unten,
   [SSR-Identität](#ssr-identität-warum-zwei-backend-urls)). Die GUI liest beide zur
   Laufzeit (`$env/dynamic/*`), also genügt der `environment:`-Block am `gui`-Service.

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

   > **Zwei Stolperfallen:**
   > - **Nicht das Cert aus dem Smoketest kopieren, sondern hier neu ausstellen.**
   >   acme.sh merkt sich pro Domain den Webroot des erfolgreichen Issues (`Le_Webroot`).
   >   Stellst du im [`smoketest/`](smoketest/) aus und kopierst dann nur die Dateien nach
   >   `deploy/tls/`, zeigt `Le_Webroot` weiter auf das Smoketest-Verzeichnis — der
   >   automatische Renew schreibt die Challenge dann dorthin (nicht in das in den
   >   Produktiv-nginx gemountete `deploy/acme-webroot/`) und **scheitert**. Prüfen:
   >   `grep -i webroot ~/.acme.sh/<host>_ecc/<host>.conf` muss `.../deploy/acme-webroot`
   >   zeigen; falls nicht, `./acme-setup.sh` bzw. `acme.sh --issue --force -w "$(pwd)/acme-webroot"`
   >   erneut aus `deploy/` laufen lassen.
   > - **Den Renew-Cron sicherstellen.** Auf einem frischen (Alpine-)Host existiert evtl.
   >   noch kein Crontab (`crontab -l` → BusyBox „can't open ..."). Dann einmalig
   >   `~/.acme.sh/acme.sh --install-cronjob` und prüfen, dass ein Cron-Daemon läuft
   >   (`pgrep crond`; sonst `sudo rc-update add crond default && sudo rc-service crond start`).
   >   Einen **separaten Cronjob für den Reload brauchst du nicht** — der hängt am
   >   `--reloadcmd`. Der harmlose `Cannot parse _ssldate2time`-Hinweis kommt vom
   >   BusyBox-`date` und betrifft nur die Datumsanzeige, nicht das Cert.

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

## SSR-Identität (warum zwei Backend-URLs)

Die GUI erreicht das Backend über **zwei verschiedene URLs**, weil nur einer der beiden
Hops das OIDC-Session-Cookie hat:

| GUI-Teil | läuft in | URL | Identität |
|----------|----------|-----|-----------|
| Client-Bundle (Browser-Queries, `wss://`-Subscriptions) | **Browser** | `PUBLIC_PLEXAMS_SERVER` = `https://<SERVER_NAME>/query` | Cookie → oauth2-proxy → nginx injiziert `X-Remote-User` |
| SSR-Node (`+page.server.ts` `load()`, `hooks.server.js`, `/api`-Proxys) | **`gui`-Container** | `PLEXAMS_SERVER` = `http://plexams:8080/query` (intern) | relayter `X-Remote-User` |

**Warum nicht beide über die öffentliche URL?** Der SSR-Node macht Server-zu-Server-Calls
**ohne** Browser-Cookie. Ginge er über `https://<SERVER_NAME>/query`, würde ihn oauth2-proxy
zur `sso.hm.edu`-Loginseite umleiten; die GUI bekäme HTML statt GraphQL-JSON und
`graphql-request` scheitert mit *„Invalid execution result: result is not object or array"*
→ **500** auf `GET /`. Genau dieser Fehler tritt auf, wenn `PLEXAMS_SERVER` fälschlich auf
die öffentliche URL zeigt.

**Wie die SSR-Calls dann autorisiert werden:** nginx injiziert `X-Remote-User` (validiert,
autoritativ) auch in den Seiten-Request an den `gui`-Container (`location /`), und die GUI
**reicht diesen Header auf ihren internen Backend-Calls weiter**. So kennt das Backend den
User auch bei SSR — ohne dass die öffentliche URL involviert ist.

> **Voraussetzung `auth.enabled: true`:** Die GUI muss den `X-Remote-User` auf dem
> `gui`→`plexams:8080`-Hop tatsächlich weiterreichen (SSR-`load()`s, `hooks.server.js`,
> alle `/api`-Proxys). Solange das GUI-Release das noch **nicht** tut, weist das Backend die
> anonymen internen SSR-Calls fail-closed ab (JSON-401). Übergangs­weise dann
> `auth.enabled: false` fahren: die SSO-Wand bleibt an nginx, alle eingeloggten Nutzer
> agieren als ADMIN (kein Rollen-Split, kein per-User-Audit) — sobald das GUI-Forwarding
> live ist, `auth.enabled: true` schalten, ohne weitere Deploy-Änderung.

## Sicherheits-Kernregeln

- **Backend nie veröffentlichen.** `plexams` und `oauth2-proxy` haben bewusst kein
  `ports:` — nur über nginx **und** den internen `gui`-Container erreichbar. Sonst könnte
  jeder den `X-Remote-User`-Header selbst setzen. `mongo` ist ausschließlich auf
  `127.0.0.1` veröffentlicht (nur Host-Loopback, für den SSH-Tunnel oben) — **nie** auf
  `0.0.0.0`.
- nginx setzt `X-Remote-User` per `proxy_set_header` autoritativ (überschreibt jeden
  Client-Wert) aus dem verifizierten `email`-Claim — sowohl auf `/query`/`/upload`/
  `/download` als auch auf `/` (für die SSR-Calls des `gui`-Containers, s. o.).
- **`gui` relayt nur, was nginx validiert hat.** Der `gui`-Container darf `plexams:8080`
  intern erreichen und `X-Remote-User` weiterreichen; er kann ihn aber nicht fälschen, weil
  nginx den Header vor Erreichen des `gui`-Containers autoritativ überschreibt.
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

**Runner einrichten (einmalig, auf dem Host, als User `plexams`):** Der Runner läuft als
**Container** (`gh-runner`-Service in der `docker-compose.yml`, Profil `runner`) — nicht als
nativer Host-Dienst. Grund: der Host ist Alpine (musl-libc, kein systemd), das native
.NET-Runner-Binary von GitHub braucht aber glibc und `svc.sh` erzeugt eine systemd-Unit;
das glibc-Image umgeht beides.

```sh
# 1) PAT anlegen: GitHub → Settings → Developer settings → Personal access tokens.
#    Classic mit Scope `repo` + `workflow`, ODER Fine-grained mit Repository-Permission
#    "Administration: Read and write" auf obcode/plexams.go. Damit erneuert der Container
#    seine kurzlebigen Registration-Tokens selbst (übersteht Neustarts ohne Token-Tausch).
# 2) In /home/plexams/plexams.go/deploy/.env eintragen:
#    GH_RUNNER_PAT=ghp_...
# 3) Nur den Runner starten (Profil "runner"; der Default-Stack bleibt unberührt):
cd /home/plexams/plexams.go/deploy
docker compose --profile runner up -d gh-runner
docker compose logs -f gh-runner        # Registrierung prüfen
```
Danach taucht der Runner unter *Repo → Settings → Actions → Runners* mit Label
`plexams-deploy` auf — genau was `runs-on: [self-hosted, plexams-deploy]` im Workflow
erwartet. Der `plexams`-User muss in der `docker`-Gruppe sein (Socket-Zugriff).

> **Warum ein eigenes Profil?** Der Deploy-Job macht ein blankes `docker compose up -d`.
> Ein Service unter `profiles: [runner]` wird davon **komplett ignoriert** (weder gestartet
> noch gestoppt) und läuft ungestört weiter — sonst würde der Runner sich **mitten im
> eigenen Deploy-Job selbst neu starten** und den Job abbrechen. Deshalb steckt er zwar in
> derselben Compose-Datei, gehört aber nicht zum Default-Stack.

- **Freischalten:** Der `deploy`-Job ist hinter der Repo-Variable **`AUTO_DEPLOY`** gegated
  (Settings → Secrets and variables → Actions → Variables → `AUTO_DEPLOY=true`). Erst danach
  läuft er — vorher wird das Image weiterhin gebaut+gepusht, der Deploy-Job aber übersprungen
  (kein hängender Queue-Job, solange der Runner fehlt).
- Das on-host Deploy-Verzeichnis ist standardmäßig `/home/plexams/plexams.go/deploy` (überschreibbar
  über die Repo-Variable `DEPLOY_DIR`). Dort liegen die Secrets (`.env`, `.plexams.yaml`,
  `tls/`) — der Job fasst sie **nie** an, er synct nur compose-Datei + nginx-Templates.
- Für **plexams.gui** einen **zweiten** Runner-Container mit eigenem `REPO_URL`/`RUNNER_NAME`
  danebenstellen (der `myoung34`-Container ist an genau ein Repo gebunden) und im GUI-Repo
  ebenfalls `AUTO_DEPLOY=true` setzen — beide Workflows arbeiten auf demselben Stack und
  fassen jeweils nur ihren eigenen Service an. Alternativ den PAT/Runner org-weit
  registrieren (`RUNNER_SCOPE=org`), dann bedient ein Container beide Repos.
- **Sicherheit:** der Runner führt Workflow-Code auf dem Prod-Host aus → Trigger strikt auf
  `release` beschränkt (kein PR-Code), Runner nur für diese beiden Repos.

**Rollback:** in `/home/plexams/plexams.go/deploy/.env` `PLEXAMS_TAG` (bzw. `GUI_TAG`) auf eine ältere
Version setzen und `docker compose up -d plexams` (bzw. `gui`) — die ghcr-Images sind
versioniert vorhanden. **Die Image-Tags tragen ein `v`-Präfix** (z. B. `PLEXAMS_TAG=v3.21.0`,
nicht `3.21.0`) — so taggt `docker/metadata-action` (roher Git-Ref); ohne `v` läuft der Pull
in ein `not found`. `latest` zeigt auf den jeweils neuesten Release.

## Betrieb

- Logs: `docker compose logs -f nginx` / `... oauth2-proxy` / `... plexams` / `... gui`.
- Manuelles Update ohne CI: `docker compose pull && docker compose up -d`.
- Zertifikat-Renew testen: `~/.acme.sh/acme.sh --renew -d <host> --force`.

### Von außen an die MongoDB (SSH-Tunnel)

Mongo ist bewusst **nur auf dem Host-Loopback** veröffentlicht
(`127.0.0.1:27017:27017` in `docker-compose.yml`) — **nicht** auf `0.0.0.0`. Damit ist der
Port aus dem Netz **nicht** erreichbar (die Firewall braucht keinen offenen 27017), aber
lokal auf dem Host schon. Der einzige Weg von außen ist deshalb ein **SSH-Tunnel**:

```sh
# Lokaler Port 27017 → Host-Loopback → Mongo-Container
ssh -N -L 27017:127.0.0.1:27017 plexams@plexams.cs.hm.edu
```

Dann in **MongoDB Compass** (oder `mongosh`) auf `localhost` verbinden:

```
mongodb://<MONGO_USER>:<MONGO_PASSWORD>@localhost:27017/?authSource=admin
```

(`MONGO_USER`/`MONGO_PASSWORD` aus `.env`; `authSource=admin`, weil es der Root-User ist.)
Läuft schon lokal etwas auf 27017, einen anderen lokalen Port wählen, z. B.
`-L 27018:127.0.0.1:27017` und dann `localhost:27018`.

> **Warum das sicher ist:** Der `127.0.0.1`-Bind gilt auch dann, wenn Docker die
> awall-INPUT-Regeln umgeht — Dockers DNAT greift nur für Pakete mit Ziel `127.0.0.1`, und
> die kann von außen niemand erzeugen. Ein `0.0.0.0`-Bind (oder ein `ports:`-Eintrag ohne
> `127.0.0.1:`-Präfix) würde Mongo dagegen an der Firewall vorbei ins Netz stellen — **nie
> tun.**

### MongoDB-Backup

Zwei komplementäre Ebenen:

1. **Lokale rotierende Dumps (Host).** [`backup/mongo-backup.sh`](backup/mongo-backup.sh)
   dumpt via `docker compose exec mongo mongodump` **alle** Datenbanken (jedes Semester +
   die globale `plexams`-DB) in ein gzip-Archiv nach `/home/plexams/backups/` und rotiert
   (Default 14 täglich + 8 wöchentlich). Als User `plexams` per busybox-`crond` einplanen:
   ```sh
   crontab -e   # als plexams:
   30 2 * * *  /home/plexams/plexams.go/deploy/backup/mongo-backup.sh >> /home/plexams/backups/backup.log 2>&1
   ```
   Restore-Kommando steht im Skript-Kopf. Schützt gegen versehentliches Löschen / kaputte
   Restores, **nicht** gegen Hostverlust — für off-site am Skriptende ein `scp`/`rclone` anhängen.
2. **Planer-ZIP über die GUI.** Der Planer kann jederzeit einen Semester-Dump als ZIP
   herunterladen (`/download/...`); die GUI bietet das prominent an, sobald sich seit dem
   letzten Dump etwas in der DB geändert hat. Gut vor riskanten Aktionen / zum Mitnehmen.
