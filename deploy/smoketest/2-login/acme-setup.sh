#!/bin/sh
# Obtain and install the TLS certificate via acme.sh (HTTP-01 webroot) against the
# HM ACME CA, which uses External Account Binding (EAB). Run from the deploy/ dir on
# the host (acme.sh runs on the host, not in a container).
#
# Prerequisites:
#   - acme.sh installed:  curl https://get.acme.sh | sh -s email=you@hm.edu
#   - .env and acme.env filled in (see *.example)
#   - the stack is up (docker compose up -d) so nginx serves the challenge on :80,
#     and a (self-signed) cert already exists in tls/ so nginx could start — see
#     "First run" in README.md.
#
# acme.sh installs a daily renew cron itself; --reloadcmd reloads nginx after renewal.
set -eu

cd "$(dirname "$0")"

# .env is a docker-compose env-file, NOT a shell script: values may contain #, *, ^, @,
# spaces, ... that break `.`-sourcing in sh (e.g. a password would be run as a command).
# We only need SERVER_NAME from it, so extract just that instead of sourcing the file.
SERVER_NAME=$(sed -n 's/^SERVER_NAME=//p' ./.env | head -n1 | tr -d '\r')
[ -n "${SERVER_NAME:-}" ] || { echo "SERVER_NAME not set in ./.env" >&2; exit 1; }
# acme.env is our own shell-source file (simple KEY=value), so sourcing it is fine.
# shellcheck disable=SC1091
. ./acme.env

ACME="${ACME_SH:-$HOME/.acme.sh/acme.sh}"
if ! [ -x "$ACME" ] && ! command -v "$ACME" >/dev/null 2>&1; then
    echo "acme.sh not found (set ACME_SH or install via: curl https://get.acme.sh | sh)" >&2
    exit 1
fi

WEBROOT="$(pwd)/acme-webroot"
mkdir -p "$WEBROOT"

echo ">> Registering ACME account with EAB against $ACME_DIRECTORY_URL"
"$ACME" --register-account \
    --server "$ACME_DIRECTORY_URL" \
    --eab-kid "$ACME_EAB_KID" \
    --eab-hmac-key "$ACME_EAB_HMAC_KEY" \
    -m "$ACME_EMAIL"

echo ">> Issuing certificate for $SERVER_NAME (HTTP-01 via $WEBROOT)"
"$ACME" --issue \
    --server "$ACME_DIRECTORY_URL" \
    -d "$SERVER_NAME" \
    -w "$WEBROOT"

echo ">> Installing certificate into tls/ and wiring the reload hook"
"$ACME" --install-cert -d "$SERVER_NAME" \
    --fullchain-file "$(pwd)/tls/fullchain.pem" \
    --key-file       "$(pwd)/tls/privkey.pem" \
    --reloadcmd      "cd $(pwd) && docker compose exec -T nginx nginx -s reload"

echo ">> Done. Certificate installed; renewals are handled by the acme.sh cron."
