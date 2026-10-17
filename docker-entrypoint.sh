#!/bin/sh
set -e

if [ "$#" -eq 0 ]; then
	set -- server
fi

# Apply migrations before the server starts. Any other CLI command
# (db:migrate, repl, admin:grant, ...) runs on its own. Without a DSN the
# server starts with no database, as it did before this entrypoint existed.
if [ "$1" = "server" ] && [ -n "${AIRWAY_DSN:-}${DSN:-}" ]; then
	/app/app db:migrate
fi

exec /app/app "$@"
