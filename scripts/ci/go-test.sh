#!/usr/bin/env sh
set -eu

coverage_profile="${GO_COVERAGE_PROFILE:-coverage.out}"
race="${GO_TEST_RACE:-true}"
cgo="${GO_TEST_CGO:-${race}}"

if [ "${cgo}" = "true" ] && ! command -v gcc >/dev/null 2>&1; then
	echo "CGO-enabled tests require a C compiler in the toolchain image." >&2
	exit 2
fi

package_dirs="$(go list -buildvcs=false -f '{{.Dir}}' ./...)"
packages="$(printf '%s\n' "${package_dirs}" | awk -v pwd="${PWD}" '$0 !~ /\/gen\// { sub("^" pwd, "."); print }')"
if [ -z "${packages}" ]; then
	echo "no Go packages to test"
	exit 0
fi

set -- go test -buildvcs=false -coverprofile="${coverage_profile}" -covermode=atomic

if [ "${race}" = "true" ]; then
	set -- "$@" -race
fi

if [ "${cgo}" = "true" ]; then
	cgo_enabled=1
else
	cgo_enabled=0
fi

# Package paths are emitted by go list and do not contain whitespace in this repository.
# shellcheck disable=SC2086
env CGO_ENABLED="${cgo_enabled}" "$@" ${packages}

# GO_COVERAGE_EXCLUDE_REGEX is an optional awk regex applied to the file:line
# column of the coverage profile. Matching entries are dropped before % is
# computed, so plumbing files (handlers, repositories, main, DB setup) can be
# excluded and the reported coverage reflects only the logic packages worth
# testing. The default `^$` never matches anything.
exclude_regex="${GO_COVERAGE_EXCLUDE_REGEX:-^$}"

tmp_profile="$(mktemp)"
awk -v excl="${exclude_regex}" \
	'NR == 1 || ($1 !~ /\/gen\// && $1 !~ /(_gen|_generated)\.go:/ && $1 !~ excl)' \
	"${coverage_profile}" >"${tmp_profile}"
mv "${tmp_profile}" "${coverage_profile}"

go tool cover -func="${coverage_profile}" | tail -n 1
