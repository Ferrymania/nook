#!/bin/sh
set -eu

export NOOK_APP_PORT="${NOOK_APP_PORT:-${MEBOX_APP_PORT:-8080}}"
export NOOK_APP_DATA_DIR="${NOOK_APP_DATA_DIR:-${MEBOX_APP_DATA_DIR:-/data}}"
export NOOK_APP_WEB_DIR="${NOOK_APP_WEB_DIR:-${MEBOX_APP_WEB_DIR:-/app/web/dist}}"
export NOOK_CACHE_CACHE_DIR="${NOOK_CACHE_CACHE_DIR:-${MEBOX_CACHE_CACHE_DIR:-/cache}}"
export NOOK_LOGGING_LEVEL="${NOOK_LOGGING_LEVEL:-${MEBOX_LOGGING_LEVEL:-info}}"

run_uid="${PUID:-$(id -u nook 2>/dev/null || echo 1000)}"
run_gid="${PGID:-$(id -g nook 2>/dev/null || echo 1000)}"

case "$run_uid" in
  ''|*[!0-9]*)
    echo "PUID must be a numeric uid, got: $run_uid" >&2
    exit 1
    ;;
esac

case "$run_gid" in
  ''|*[!0-9]*)
    echo "PGID must be a numeric gid, got: $run_gid" >&2
    exit 1
    ;;
esac

if [ "$run_uid" = "0" ]; then
  exec nook
fi

chown -R "$run_uid:$run_gid" /data /cache 2>/dev/null || true
chown "$run_uid:$run_gid" /media 2>/dev/null || true

exec su-exec "$run_uid:$run_gid" nook
