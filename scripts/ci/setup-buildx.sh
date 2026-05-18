#!/usr/bin/env sh
set -eu

builder="${BUILDX_BUILDER:-saga-sandbox-builder}"

if docker buildx inspect "${builder}" >/dev/null 2>&1; then
	docker buildx use "${builder}"
else
	docker buildx create --name "${builder}" --use
fi

docker buildx inspect --bootstrap
