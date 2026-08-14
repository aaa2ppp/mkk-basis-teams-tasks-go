#!/bin/sh

MIGRATIONS_DIR=${MIGRATIONS_DIR:-"./migrations"}

if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "$(basename $0): $MIGRATIONS_DIR dir not found" >&2
    exit 1
fi

host=${DB_ADDR%:*}
port=${DB_ADDR#*:}
[ "$port" = "$host" ] && port=

: ${host:=localhost}
: ${port:=3306}

dbname=${DB_NAME:-app-db}
user=${DB_USER:-app-user}
password=${DB_PASSWORD?DB_PASSWORD is not set. Please run: source dev-env}

export GOOSE_DRIVER=mysql
export GOOSE_DBSTRING="$user:$password@tcp($host:$port)/$dbname?parseTime=true"
export GOOSE_MIGRATION_DIR=$MIGRATIONS_DIR
goose "$@"
