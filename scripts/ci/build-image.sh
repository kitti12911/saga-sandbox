#!/usr/bin/env sh
set -eu

context="${IMAGE_CONTEXT:?IMAGE_CONTEXT is required}"
platform="${IMAGE_PLATFORM:?IMAGE_PLATFORM is required}"
tag="${IMAGE_TAG:?IMAGE_TAG is required}"

set -- docker buildx build \
	--platform "${platform}" \
	--tag "${tag}"

if [ -n "${TOOLCHAIN_IMAGE:-}" ]; then
	set -- "$@" --build-arg "TOOLCHAIN_IMAGE=${TOOLCHAIN_IMAGE}"
fi

if [ -n "${BUILD_CACHE_FROM:-}" ]; then
	set -- "$@" --cache-from "${BUILD_CACHE_FROM}"
fi

if [ -n "${BUILD_CACHE_TO:-}" ]; then
	set -- "$@" --cache-to "${BUILD_CACHE_TO}"
fi

if [ -n "${BUILD_OUTPUT:-}" ]; then
	set -- "$@" --output "${BUILD_OUTPUT}"
fi

if [ "${BUILD_LOAD:-false}" = "true" ]; then
	set -- "$@" --load
fi

if [ -n "${BUILD_PROVENANCE:-}" ]; then
	set -- "$@" --provenance="${BUILD_PROVENANCE}"
fi

if [ -n "${BUILD_SBOM:-}" ]; then
	set -- "$@" --sbom="${BUILD_SBOM}"
fi

set -- "$@" "${context}"

"$@"
