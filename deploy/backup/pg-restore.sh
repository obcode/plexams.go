#!/bin/sh
# Restore a whole-database archive produced by pg-backup.sh.
#
#     ./pg-restore.sh /home/plexams/backups/plexams-daily-20260804-0230.dump.gz
#
# This REPLACES the current contents of the database -- every semester, not just one.
# All semesters share one schema keyed by semester_id, so there is no way to restore a
# single semester here; that is a row-level operation and comes later (see
# ../README.md "Backup & Restore").
#
# WHY THIS RUNS ON THE HOST AND NOT AS A ROUTE IN THE SERVER:
# pg_restore --clean drops and recreates the objects it restores. It cannot do that
# while plexams.go holds a connection pool on the database -- the DROPs block on the
# open connections, and objects the app is mid-query on would vanish underneath it.
# So the app is stopped for the duration and started again afterwards. A route inside
# the running server would have to shut down its own pool and reconnect, which is
# strictly more moving parts for an operation that is rare and already manual.
#
# The plexams container is DOWN while this runs -- the GUI will show errors. That is
# expected and the reason for the confirmation prompt.

set -eu

DEPLOY_DIR="${DEPLOY_DIR:-/home/plexams/plexams.go/deploy}"
COMPOSE="docker compose -f ${DEPLOY_DIR}/docker-compose.yml"

archive="${1:-}"
if [ -z "${archive}" ]; then
    echo "usage: $0 <archive.dump.gz>" >&2
    echo "" >&2
    echo "available:" >&2
    ls -1t "${BACKUP_DIR:-/home/plexams/backups}"/plexams-*.dump.gz 2>/dev/null | head -10 >&2
    exit 2
fi
if [ ! -s "${archive}" ]; then
    echo "ERROR: ${archive} does not exist or is empty" >&2
    exit 1
fi

# --- Confirmation ---------------------------------------------------------------------
# Skip with FORCE=1 for an unattended rebuild of a throwaway instance.
if [ "${FORCE:-0}" != "1" ]; then
    echo "About to REPLACE the entire plexams database from:"
    echo "    ${archive}  ($(du -h "${archive}" | cut -f1))"
    echo ""
    echo "Every semester in the database is replaced. plexams.go will be stopped"
    echo "during the restore and started again afterwards."
    printf 'Type "restore" to continue: '
    read -r answer
    [ "${answer}" = "restore" ] || { echo "aborted."; exit 1; }
fi

# --- Safety net -----------------------------------------------------------------------
# Take a dump of the CURRENT state first, so a restore of the wrong archive is itself
# undoable. This is the cheapest possible insurance and has already been the difference
# between an inconvenience and a lost afternoon elsewhere.
echo "==> dumping the current state first (safety net)"
BACKUP_DIR="${BACKUP_DIR:-/home/plexams/backups}" "${DEPLOY_DIR}/backup/pg-backup.sh"

# --- Stop the application --------------------------------------------------------------
# Only plexams: postgres must keep running (we restore INTO it), and caddy/gui stay up so
# the outage is a clear backend error rather than a dead host.
echo "==> stopping plexams"
# shellcheck disable=SC2086
${COMPOSE} stop plexams

restore_status=0

# --- Restore ---------------------------------------------------------------------------
# Drop and recreate the database rather than pg_restore --clean into the existing one.
# --clean drops objects one by one, and with --exit-on-error any benign "cannot drop"
# aborts the whole restore; without it a half-failed restore looks like it worked. A
# fresh database has neither problem: afterwards the archive is either fully in or the
# restore failed loudly.
#
# --force (PG 13+) terminates leftover connections. plexams is stopped above, so this
# should find none -- it is there for the stray psql session someone forgot.
# dropdb/createdb read no stdin, so the pipe reaches pg_restore intact.
echo "==> restoring"
# shellcheck disable=SC2086
if gunzip -c "${archive}" | ${COMPOSE} exec -T postgres sh -c '
    db="${POSTGRES_DB:-plexams}" &&
    dropdb   --username "$POSTGRES_USER" --force --if-exists "$db" &&
    createdb --username "$POSTGRES_USER" "$db" &&
    pg_restore \
        --username "$POSTGRES_USER" \
        --dbname "$db" \
        --no-owner --no-privileges \
        --exit-on-error
'; then
    echo "==> restore ok"
else
    restore_status=$?
    echo "ERROR: restore failed (exit ${restore_status})." >&2
    echo "The database was dropped and recreated, so it is now empty or partial --" >&2
    echo "do NOT let the planners work on it. The safety-net dump taken above is the" >&2
    echo "newest plexams-daily-*.dump.gz in ${BACKUP_DIR:-/home/plexams/backups};" >&2
    echo "re-run this script with it to get back to where you started." >&2
fi

# --- Start the application again ---------------------------------------------------------
# Unconditionally, even after a failed restore: leaving the backend down helps nobody,
# and its own startup log is the next useful diagnostic.
echo "==> starting plexams"
# shellcheck disable=SC2086
${COMPOSE} start plexams

exit "${restore_status}"
