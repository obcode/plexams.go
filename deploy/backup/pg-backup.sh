#!/bin/sh
# Local rotating PostgreSQL backup for the plexams.go deployment.
#
# Dumps the WHOLE database (every semester -- they share one schema, keyed by
# semester_id) from the compose `postgres` service into a gzip'd custom-format
# archive on the host, then prunes old backups. No offsite copy -- this guards
# against accidental data loss / bad restores, not host loss. For an off-host copy
# add an scp/rclone step at the end (see README).
#
# There is deliberately no per-semester dump: taking one semester out is a
# row-level operation across ~64 tables, and it is the rare case. See
# ../README.md "Backup & Restore".
#
# Run as the deploy user (`plexams`) from cron. Schedule (busybox crond on Alpine),
# e.g. daily 02:30 -- `crontab -e` as plexams:
#     30 2 * * *  /home/plexams/plexams.go/deploy/backup/pg-backup.sh >> /home/plexams/backups/backup.log 2>&1
#
# Restore: ./pg-restore.sh <archive>   (stops plexams first -- see that script).

set -eu

# --- Config (override via environment before calling) --------------------------------
DEPLOY_DIR="${DEPLOY_DIR:-/home/plexams/plexams.go/deploy}"
BACKUP_DIR="${BACKUP_DIR:-/home/plexams/backups}"
KEEP_DAILY="${KEEP_DAILY:-14}"    # keep this many most-recent daily archives
KEEP_WEEKLY="${KEEP_WEEKLY:-8}"   # plus this many weekly (Monday) archives

COMPOSE="docker compose -f ${DEPLOY_DIR}/docker-compose.yml"

# --- Credentials ---------------------------------------------------------------------
# Do NOT source the deploy .env here: it is docker-compose format, not shell. Strong
# passwords contain #, *, ^, @, spaces … which `. .env` would try to execute (that is
# exactly what broke the predecessor of this script). Instead pg_dump reads the
# credentials from the postgres container's OWN environment (POSTGRES_USER/_DB, set by
# compose from the .env). Inside the container no password is needed at all: the
# connection is local and trusted, so nothing ever reaches the host process list.

mkdir -p "${BACKUP_DIR}"

# --- Dump -----------------------------------------------------------------------------
# -Fc is the custom format: compressed, and the only one pg_restore can use selectively
# (--clean, single tables, parallel). Plain SQL would restore only as a whole.
stamp="$(date +%Y%m%d-%H%M)"
weekday="$(date +%u)"                     # 1 = Monday
out="${BACKUP_DIR}/plexams-daily-${stamp}.dump.gz"
tmp="${out}.part"

# The single-quoted inner script is expanded by the container's shell, so the
# credentials are resolved inside postgres and never appear in the host process list.
# shellcheck disable=SC2086
${COMPOSE} exec -T postgres sh -c '
    pg_dump \
        --username "$POSTGRES_USER" \
        --dbname "${POSTGRES_DB:-plexams}" \
        --format=custom \
        --no-owner \
        --no-privileges
' | gzip > "${tmp}"

# Guard against a truncated/empty dump before committing the file. Note that this
# checks the gzip'd result, so it also catches a pg_dump that died mid-stream: the
# pipeline would still have produced a small but non-empty file, hence the size floor.
if [ ! -s "${tmp}" ] || [ "$(wc -c < "${tmp}")" -lt 1024 ]; then
    echo "ERROR: pg_dump produced an empty or truncated archive; keeping nothing." >&2
    rm -f "${tmp}"
    exit 1
fi
mv "${tmp}" "${out}"
[ "${weekday}" = "1" ] && cp "${out}" "${BACKUP_DIR}/plexams-weekly-${stamp}.dump.gz"
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
prune "${BACKUP_DIR}/plexams-daily-*.dump.gz" "${KEEP_DAILY}"
prune "${BACKUP_DIR}/plexams-weekly-*.dump.gz" "${KEEP_WEEKLY}"
