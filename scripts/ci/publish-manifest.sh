#!/usr/bin/env sh
set -eu

image_ref="${IMAGE_REF:?IMAGE_REF is required}"
release_tag="${RELEASE_TAG:?RELEASE_TAG is required}"
env_tag="${ENV_TAG:-}"
release_sha="${RELEASE_SHA:-}"
release_branch="${RELEASE_BRANCH:-}"
arches="${IMAGE_ARCHES:-arm64}"

set -- docker buildx imagetools create \
	--tag "${image_ref}:${release_tag}"

if [ -n "${env_tag}" ]; then
	set -- "$@" --tag "${image_ref}:${env_tag}"
fi

if [ -n "${release_sha}" ]; then
	set -- "$@" --tag "${image_ref}:${release_sha}"
fi

if [ "${release_branch}" = "main" ]; then
	set -- "$@" --tag "${image_ref}:latest"
fi

for arch in ${arches}; do
	set -- "$@" "${image_ref}:${release_tag}-${arch}"
done

"$@"
