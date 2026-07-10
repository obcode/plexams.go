# plexams.go — Server-Deployment (Docker Compose, OIDC via HM Shibboleth)

Betreibt plexams.go hinter einem Apache-Reverse-Proxy, der die Authentifizierung
**über OIDC gegen `sso.hm.edu`** (HM Shibboleth-IdP mit OIDC-OP, via
`mod_auth_openidc`) macht. Der Proxy setzt die verifizierte Identität autoritativ als
Header `X-Remote-User`; das Backend vertraut diesem Header und erzwingt die
Autorisierung (Rollen) selbst.

```
Internet ──443/TLS──> apache (mod_auth_openidc + mod_proxy[_wstunnel])
                        ├── /            → plexams.gui (statische dist)
                        └── /query,/upload,/download → plexams.go:8080
plexams.go  ── nur im compose-Netz (nicht veröffentlicht)
mongo       ── nur im compose-Netz (nicht veröffentlicht), Named Volume
```

## ⚑ Was die Zentrale IT (IdP-Team) braucht

Für die OIDC-Client-Registrierung an `sso.hm.edu` gib an:

| Feld | Wert |
|------|------|
| **Redirect / Callback URI** | `https://<DEIN-HOST>/redirect_uri` |
| Grant type | `authorization_code` |
| Scopes | `openid email profile` |
| Post-Logout Redirect (optional) | `https://<DEIN-HOST>/` |

> Die **Redirect-URI ist exakt** `https://<DEIN-HOST>/redirect_uri` — ersetze
> `<DEIN-HOST>` durch den endgültigen DNS-Namen (z. B.
> `https://plexams.cs.hm.edu/redirect_uri`). Der Pfad `/redirect_uri` wird von
> `mod_auth_openidc` intern behandelt (er muss auf keine echte Datei zeigen). Muss
> **zeichengenau** mit `OIDCRedirectURI` in `apache/plexams.conf` bzw. `SERVER_NAME`
> übereinstimmen.

Zurück bekommst du **Client-ID** und **Client-Secret** → in `.env`
(`OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`).

## Voraussetzungen

- Docker + Docker Compose auf dem (Alpine-)Server.
- DNS-Name, der auf den Server zeigt, + TLS-Zertifikat (`fullchain.pem` +
  `privkey.pem`, z. B. via certbot).
- OIDC-Client bei der Zentralen IT registriert (s. o.) — **frühzeitig anstoßen**,
  das hat die längste Vorlaufzeit.
- Der Build von **plexams.gui** (`npm run build` → `dist/`).

## Einrichtung

1. **Konfig anlegen** (nichts davon wird committet):
   ```bash
   cd deploy
   cp .env.example .env                       # Mongo-Creds, SERVER_NAME, OIDC-Client, Crypto-Passphrase
   cp .plexams.yaml.example .plexams.yaml      # db.uri, auth.seedusers (die zwei Planer), zpa/smtp/…
   ```
   In beiden Dateien dieselbe Mongo-Passphrase verwenden (`.env` `MONGO_PASSWORD`
   ↔ `.plexams.yaml` `db.uri`) und denselben Host (`.env` `SERVER_NAME` ↔
   `.plexams.yaml` `server.allowedorigins` / apache `OIDCRedirectURI`).

2. **TLS-Zertifikat** ablegen:
   ```bash
   mkdir -p tls && cp /pfad/fullchain.pem /pfad/privkey.pem tls/
   ```

3. **GUI-Build** bereitstellen:
   ```bash
   # aus dem plexams.gui-Repo: npm run build  →  dist/
   cp -r /pfad/plexams.gui/dist ./gui-dist
   ```

4. **Starten**:
   ```bash
   docker compose up -d --build
   ```

5. **Erst-Boot**: `auth.seedusers` legt die beiden Planer als `PLANER` in der
   `users`-Collection an (nur solange die Collection leer ist). Danach werden User
   über die GUI verwaltet (`setUser`/`removeUser`); die Seed-Liste wird ignoriert.

## Zugriff für einen erweiterten Kreis (eingeschränkte Rechte)

Alles ist dafür schon vorbereitet:
- Neuen User mit Rolle **`VIEWER`** anlegen (GUI `setUser`, oder in `auth.seedusers`
  vor dem allerersten Boot). `VIEWER` darf **lesen und Validierungen laufen lassen**,
  aber **keine** datenändernden Operationen — erzwungen im Backend.
- `PLANER` = Vollzugriff.
- Feinere Rollen später über eine `@requires`-Directive pro Feld (Enum ist bewusst
  erweiterbar).

## Sicherheits-Kernregeln

- **Backend nie veröffentlichen.** In der Compose-Datei hat `plexams` bewusst kein
  `ports:` — es ist nur über apache erreichbar. Sonst könnte jeder den
  `X-Remote-User`-Header selbst setzen und sich als beliebiger Planer ausgeben.
- Apache **strippt** clientseitige Header und setzt sie autoritativ neu
  (`RequestHeader unset` vor `set`).
- `server.production: true` schaltet Playground + Introspection ab.
- Secrets nur in `.env` / `.plexams.yaml` (beide gitignored), nie in der DB, nie im Image.

## Lokale Entwicklung (unverändert)

Ohne `auth.enabled: true` läuft plexams.go wie bisher: es wird ein voll berechtigter
Dev-User injiziert, nichts wird abgewiesen. Optional `auth.devuser` setzen, damit das
`mutation_log` deine Identität zeigt. Kein Apache, kein OIDC nötig.

## Betrieb

- Logs: `docker compose logs -f apache` (Auth/Proxy), `... logs -f plexams` (Backend).
- Mongo-Backup: Named Volume `mongo-data` sichern (z. B. `docker compose exec mongo
  mongodump …`) oder das ganze Semester über die GUI als ZIP dumpen.
- Zertifikat erneuern: `tls/`-Dateien austauschen, `docker compose restart apache`.
