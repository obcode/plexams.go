#!/bin/sh
# Local rotating MongoDB backup for the plexams.go deployment.
#
# Dumps ALL databases (every semester DB + the global `plexams` DB) from the compose
# `mongo` service into a single gzip'd archive on the host, then prunes old backups.
# No offsite copy — this guards against accidental data loss / bad restores, not host
# loss. For an off-host copy add an scp/rclone step at the end (see README).
#
# Run as the deploy user (`plexams`) from cron. Reads Mongo creds from the deploy .env.
# Schedule (busybox crond on Alpine), e.g. daily 02:30 — `crontab -e` as plexams:
#     30 2 * * *  /home/plexams/plexams.go/deploy/backup/mongo-backup.sh >> /home/plexams/backups/backup.log 2>&1
#
# Restore an archive (creds read from the mongo container's own env, like the dump):
#     gunzip -c plexams-YYYYMMDD-HHMM.archive.gz \
#       | docker compose -f /home/plexams/plexams.go/deploy/docker-compose.yml exec -T mongo \
#           sh -c 'mongorestore --archive --gzip --drop \
#             --username "$MONGO_INITDB_ROOT_USERNAME" --password "$MONGO_INITDB_ROOT_PASSWORD" \
#             --authenticationDatabase admin'
#   (--drop replaces existing collections; omit to merge. Test into a throwaway host first.)

set -eu

# --- Config (override via environment before calling) --------------------------------
DEPLOY_DIR="${DEPLOY_DIR:-/home/plexams/plexams.go/deploy}"
BACKUP_DIR="${BACKUP_DIR:-/home/plexams/backups}"
KEEP_DAILY="${KEEP_DAILY:-14}"    # keep this many most-recent daily archives
KEEP_WEEKLY="${KEEP_WEEKLY:-8}"   # plus this many weekly (Monday) archives

COMPOSE="docker compose -f ${DEPLOY_DIR}/docker-compose.yml"

# --- Mongo credentials ---------------------------------------------------------------
# Do NOT source the deploy .env here: it is docker-compose format, not shell. Strong
# passwords contain #, *, ^, @, spaces … which `. .env` would try to execute (that is
# exactly what broke this script). Instead we let mongodump read the credentials from
# the mongo container's OWN environment (MONGO_INITDB_ROOT_USERNAME/PASSWORD, set by
# compose from the .env). The password never touches the host shell, `ps`, or this log.

mkdir -p "${BACKUP_DIR}"

# --- Dump -----------------------------------------------------------------------------
# mongodump --archive (no path) writes the archive to stdout; -T keeps exec output clean.
stamp="$(date +%Y%m%d-%H%M)"
weekday="$(date +%u)"                     # 1 = Monday
out="${BACKUP_DIR}/plexams-daily-${stamp}.archive.gz"
tmp="${out}.part"

# The single-quoted inner script is expanded by the container's shell, so the creds are
# resolved inside mongo and never appear in the host process list or environment.
# shellcheck disable=SC2086
${COMPOSE} exec -T mongo sh -c '
    mongodump \
        --username "$MONGO_INITDB_ROOT_USERNAME" \
        --password "$MONGO_INITDB_ROOT_PASSWORD" \
        --authenticationDatabase admin \
        --archive --gzip
' > "${tmp}"

# Guard against a truncated/empty dump before committing the file.
if [ ! -s "${tmp}" ]; then
    echo "ERROR: mongodump produced an empty archive; keeping nothing." >&2
    rm -f "${tmp}"
    exit 1
fi
mv "${tmp}" "${out}"
[ "${weekday}" = "1" ] && cp "${out}" "${BACKUP_DIR}/plexams-weekly-${stamp}.archive.gz"
echo "$(date '+%Y-%m-%d %H:%M') backup ok: ${out} ($(du -h "${out}" | cut -f1))"

# --- Rotation -------------------------------------------------------------------------
# Keep the newest KEEP_DAILY daily archives and KEEP_WEEKLY weekly ones; delete the rest.
# The daily/weekly name prefixes keep the two globs disjoint.
prune() {
    pattern="$1"; keep="$2"
    # List matching files newest-first, skip the first $keep, remove the rest.
    ls -1t ${pattern} 2>/dev/null | tail -n +"$((keep + 1))" | while IFS= read -r f; do
        rm -f "$f" && echo "  pruned $f"
    done
}
prune "${BACKUP_DIR}/plexams-daily-*.archive.gz" "${KEEP_DAILY}"
prune "${BACKUP_DIR}/plexams-weekly-*.archive.gz" "${KEEP_WEEKLY}"
