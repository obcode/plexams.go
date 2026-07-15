---
name: deploy-caddy-migration
description: "Deploy proxy migration nginx+acme.sh → Caddy (built-in ACME/EAB, forward_auth) — MERGED to main 2026-07-15 & live on host; key gotcha = HM EAB binds one ACME account, Caddy needs its OWN fresh EAB."
metadata:
  node_type: memory
  type: project
  originSessionId: 11e67bde-092b-48e8-a9a3-ef9b7ea61158
---

**MERGED to main 2026-07-15 (feat/deploy-caddy, Branch gelöscht) & live auf dem Host.** Ersetzt im
`deploy/` **nginx + acme.sh** durch **Caddy**: `forward_auth` statt `auth_request`
(oauth2-proxy unverändert), TLS via Caddys eingebautem ACME-Client mit `acme_ca` + `acme_eab`
gegen die HM-CA. Entfallen: `acme-setup.sh`, `acme.env`, `tls/`+`acme-webroot/`-Mounts,
self-signed-Bootstrap ("Henne-Ei"), Renew-Cron. Certs jetzt im `caddy-data`-Volume. Caddyfile
verwirft eingehende `X-Remote-*` und setzt sie autoritativ via `copy_headers`
(`X-Auth-Request-Email>X-Remote-User`, …); `route{}` ordnet auth vor Backend; websocket +
`request_body max_size 200MB` nativ. Smoketests (1-tls/2-login) + CI-Deploy-Job (synct
`Caddyfile` statt nginx-Templates) mitmigriert. Kein GUI-Release nötig. Lokal verifiziert
(`caddy validate` grün, adaptierte JSON: EAB + Header-Rename + Routen korrekt). Baut auf
[[auth-roles-shibboleth]] + [[deploy-push-cd]] auf (die noch nginx beschreiben — bei Merge
nachziehen).

**KERN-STOLPERFALLE — HM-EAB bindet GENAU EINEN ACME-Account.** Erststart scheiterte mit
`HTTP 401 urn:ietf:params:acme:error:unauthorized - The client lacks sufficient authorization`
am `new-account`. Ursache: das kid/hmac-Paar, das acme.sh schon benutzt hatte, ließ sich nicht
für Caddys **eigenen** neuen Account wiederverwenden. Lösung: **frische EAB-Credentials** bei
der Zentralen IT für den Caddy-Client anfordern (bestätigt — Oliver hat neue EAB besorgt, dann
lief die Ausstellung). Für einen Neuaufbau/weiteren ACME-Client immer eine eigene EAB anfragen.
Diagnose: `docker compose exec caddy sh -c 'echo $ACME_EAB_KID; echo ${#ACME_EAB_HMAC_KEY}'`
(nicht-leer + korrekte Länge). Echte Directory-URL = `https://acme.hm.edu/acme/acme/directory`
(nicht das `…/directory`-Beispiel).

**Cutover-Regel:** manuell mit `docker compose up -d --remove-orphans` (entfernt den alten
nginx-Container; sonst Port-80/443-Konflikt → Caddy kann kein Cert holen). Der CI-Auto-Deploy
macht nur `up -d` (ohne --remove-orphans), taugt also nicht für den einmaligen Umstieg.
