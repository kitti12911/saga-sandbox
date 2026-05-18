#!/usr/bin/env sh
set -eu

registry="${REGISTRY:?REGISTRY is required}"
username="${REGISTRY_USERNAME:?REGISTRY_USERNAME is required}"
password="${REGISTRY_PASSWORD:?REGISTRY_PASSWORD is required}"

printf '%s' "${password}" | docker login "${registry}" --username "${username}" --password-stdin
