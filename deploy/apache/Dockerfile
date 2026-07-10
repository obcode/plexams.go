# Apache reverse proxy that terminates TLS, authenticates users against the HM
# Shibboleth IdP via OIDC (mod_auth_openidc), and injects the trusted X-Remote-User
# header into the plexams.go backend. Also serves the plexams.gui static build and
# tunnels the GraphQL subscription websocket (mod_proxy_wstunnel).
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        apache2 \
        libapache2-mod-auth-openidc \
        ca-certificates \
    && a2enmod ssl proxy proxy_http proxy_wstunnel rewrite headers auth_openidc \
    && a2dissite 000-default \
    && rm -rf /var/lib/apt/lists/*

COPY plexams.conf /etc/apache2/sites-available/plexams.conf
RUN a2ensite plexams

EXPOSE 80 443

# Run in the foreground so the container stays alive; apache reads ${SERVER_NAME},
# ${OIDC_CLIENT_ID}, ${OIDC_CLIENT_SECRET}, ${OIDC_CRYPTO_PASSPHRASE} from the
# environment (passed by docker compose) at config-parse time.
CMD ["apache2ctl", "-D", "FOREGROUND"]
