#!/usr/bin/env bash

set -o errexit
set -o pipefail

trap cleanup INT

function cleanup() {
    rm -f profile.*
    exit
}

PKGS=$(go list ./... | grep -vE 'prymitive/karma/internal/mapper/v017/(client|models)')
COVERPKG=$(echo "$PKGS" | tr '\n' ',')

go test \
  -coverpkg="$COVERPKG" \
  -c \
  -tags testrunmain \
  ./cmd/karma 2>&1 \
    | (grep -v 'warning: no packages being tested depend on matches for pattern' || true)

(
  ALERTMANAGER_URI=http://localhost \
  ALERTMANAGER_INTERVAL=1s \
  LISTEN_ADDRESS=127.0.0.1 \
  LOG_LEVEL=fatal \
  LOG_CONFIG=false \
  ./karma.test \
  -test.run "^TestRunMain$" \
  -test.coverprofile=profile.main.1 2>&1 \
    | grep -v 'warning: no packages being tested depend on matches for pattern' \
    | sed s/'of statements in .*'/''/g &)

sleep 5
killall karma.test

rm karma.test
