#!/bin/sh

# this file can be removed once https://github.com/k3d-io/k3d/pull/1119 is merged

set -o errexit
set -o nounset

LOGFILE="/var/log/k3d-entrypoints_$(date "+%y%m%d%H%M%S").log"

touch "$LOGFILE"

echo "[$(date -Iseconds)] Running k3d entrypoints..." >> "$LOGFILE"

for entrypoint in /bin/k3d-entrypoint-*.sh ; do
  echo "[$(date -Iseconds)] Running $entrypoint"  >> "$LOGFILE"
  "$entrypoint"  >> "$LOGFILE" 2>&1 || exit 1
done

echo "[$(date -Iseconds)] Finished k3d entrypoint scripts!" >> "$LOGFILE"

/bin/k3s "$@" &
k3s_pid=$!

until kubectl uncordon $HOSTNAME; do sleep 3; done

function cleanup() {
  echo Draining node...
  kubectl drain $HOSTNAME --force --delete-emptydir-data
  echo Sending SIGTERM to k3s...
  kill -15 $k3s_pid
  echo Waiting for k3s to close...
  wait $k3s_pid
  echo Bye!
}

trap cleanup SIGTERM SIGINT SIGQUIT SIGHUP

wait $k3s_pid
echo Bye!