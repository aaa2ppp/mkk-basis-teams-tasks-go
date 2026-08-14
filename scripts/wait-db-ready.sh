#!/bin/sh

: ${DB_PASSWORD?DB_PASSWORD is not set. Please run: source dev-env}
: ${DB_SERVICE:=db}
: ${DB_USER:=app-user}
: ${DB_NAME:=app-db}
: ${DB_CHECK_TIMEOUT:=30}
: ${DB_CHECK_INTERVAL:=2}
: ${DOCKER_COMPOSE:=docker-compose}

check_database() {
  ${DOCKER_COMPOSE} exec -T ${DB_SERVICE} \
    sh -c 'mariadb -u $MYSQL_USER -p$MYSQL_PASSWORD -e "select 1;" $MYSQL_DB >/dev/null 2>&1'
}

echo "Waiting for database to be ready (timeout: ${DB_CHECK_TIMEOUT}s)..." >&2

end=$(( $(date +%s) + DB_CHECK_TIMEOUT ))
while ! check_database; do
  if [ $(date +%s) -ge $end ]; then
    echo "DB not ready after ${DB_CHECK_TIMEOUT}s" >&2
    exit 1
  fi
  echo "Waiting ${DB_CHECK_INTERVAL}s..." >&2
  sleep $DB_CHECK_INTERVAL
done

echo "DB is ready!" >&2
exit 0
