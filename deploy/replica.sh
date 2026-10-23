#!/bin/sh
# On an empty volume, the replica copies the primary first.
set -e
if [ ! -s "$PGDATA/PG_VERSION" ]; then
  PGPASSWORD=replicator pg_basebackup -h postgres -U replicator -D "$PGDATA" -S replica -R
fi
exec docker-entrypoint.sh "$@"
