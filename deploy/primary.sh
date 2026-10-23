#!/bin/sh
# Runs once, when the primary starts on an empty volume.
echo "host replication replicator all scram-sha-256" >> "$PGDATA/pg_hba.conf"
psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "create role replicator with replication login password 'replicator'" \
  -c "select pg_create_physical_replication_slot('replica')" \
  -c "select pg_reload_conf()"
