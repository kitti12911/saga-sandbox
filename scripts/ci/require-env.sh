#!/usr/bin/env sh
set -eu

for name in "$@"; do
	eval "value=\${${name}:-}"
	if [ -z "${value}" ]; then
		echo "${name} is required" >&2
		exit 2
	fi
done
