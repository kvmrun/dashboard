#!/bin/bash
set -eu

declare -rx GOOS="linux"
declare -rx GOBIN="$(pwd)/bin"
declare -rx CGO_ENABLED=1

declare -r USER_GROUP="$(stat --printf "%u:%g" .)"

install -d "$GOBIN"
trap "chown -R $USER_GROUP $GOBIN" 0

if [[ ! -f /usr/include/security/pam_appl.h ]] || ! command -v gcc >/dev/null 2>&1; then
    apt-get update
    apt-get install -y --no-install-recommends libpam-dev gcc
    rm -rf /var/lib/apt/lists/*
fi

go version

go fmt "./..."

go install \
    -v \
    -buildvcs=false \
    -ldflags "-s -w" \
        "./cmd/dashboard"

exit 0
