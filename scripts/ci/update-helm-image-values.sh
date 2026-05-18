#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
"${script_dir}/require-env.sh" VALUES_FILE RELEASE_TAG IMAGE_DIGEST

if [ ! -f "${VALUES_FILE}" ]; then
	echo "${VALUES_FILE} does not exist; skipping Helm values update."
	exit 0
fi

image_repository="${IMAGE_REPOSITORY:-}"
if [ -z "${image_repository}" ]; then
	"${script_dir}/require-env.sh" IMAGE_NAME

	image_registry="${DEPLOY_IMAGE_REGISTRY:-${IMAGE_REGISTRY:-}}"
	image_namespace="${DEPLOY_IMAGE_NAMESPACE:-${IMAGE_NAMESPACE:-}}"
	if [ -z "${image_registry}" ] || [ -z "${image_namespace}" ]; then
		echo "IMAGE_REPOSITORY, or IMAGE_REGISTRY and IMAGE_NAMESPACE, is required." >&2
		exit 2
	fi

	image_repository="${image_registry}/${image_namespace}/${IMAGE_NAME}"
fi

tmp_file="$(mktemp)"
awk \
	-v image_repository="${image_repository}" \
	-v release_tag="${RELEASE_TAG}" \
	-v image_digest="${IMAGE_DIGEST}" '
	/^image:/ {
		in_image = 1
		print
		next
	}
	in_image && $0 !~ /^    / {
		in_image = 0
	}
	in_image && $1 == "repository:" {
		print "    repository: " image_repository
		next
	}
	in_image && $1 == "tag:" {
		print "    tag: \"" release_tag "\""
		next
	}
	in_image && $1 == "digest:" {
		print "    digest: \"" image_digest "\""
		next
	}
	{ print }
' "${VALUES_FILE}" >"${tmp_file}"
mv "${tmp_file}" "${VALUES_FILE}"
