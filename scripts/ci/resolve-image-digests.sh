#!/usr/bin/env sh
set -eu

image_ref="${IMAGE_REF:?IMAGE_REF is required}"
release_tag="${RELEASE_TAG:?RELEASE_TAG is required}"
arches="${IMAGE_ARCHES:-arm64}"

digest="$(docker buildx imagetools inspect "${image_ref}:${release_tag}" --format '{{.Manifest.Digest}}')"
echo "digest=${digest}"

if [ -n "${CI_OUTPUT_FILE:-}" ]; then
	echo "digest=${digest}" >>"${CI_OUTPUT_FILE}"
fi

for arch in ${arches}; do
	arch_digest="$(docker buildx imagetools inspect "${image_ref}:${release_tag}-${arch}" --format '{{.Manifest.Digest}}')"
	echo "${arch}_digest=${arch_digest}"
	if [ -n "${CI_OUTPUT_FILE:-}" ]; then
		echo "${arch}_digest=${arch_digest}" >>"${CI_OUTPUT_FILE}"
	fi
done
