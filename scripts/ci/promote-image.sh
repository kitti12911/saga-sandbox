#!/usr/bin/env sh
set -eu

staging_image_ref="${STAGING_IMAGE_REF:?STAGING_IMAGE_REF is required}"
arch_image_ref="${ARCH_IMAGE_REF:?ARCH_IMAGE_REF is required}"

set -- docker buildx imagetools create \
	--tag "${arch_image_ref}"

if [ -n "${SHA_ARCH_IMAGE_REF:-}" ]; then
	set -- "$@" --tag "${SHA_ARCH_IMAGE_REF}"
fi

set -- "$@" "${staging_image_ref}"

"$@"
