#!/usr/bin/env sh
set -eu

repo_dir="${CI_PROJECT_DIR:-$(pwd)}"
cd "${repo_dir}"

git config --global --add safe.directory "${repo_dir}" 2>/dev/null || true
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
	echo "semantic-release must run from a Git checkout, but ${repo_dir} is not a Git repository." >&2
	exit 1
fi

release_branch="${RELEASE_BRANCH:-$(git branch --show-current 2>/dev/null || true)}"
log_file="${SEMANTIC_RELEASE_LOG:-${RUNNER_TEMP:-/tmp}/semantic-release.log}"

if [ -n "${CI_OUTPUT_FILE:-}" ]; then
	{
		echo "published=false"
		echo "env_tag=${release_branch}"
	} >>"${CI_OUTPUT_FILE}"
fi

set +e
npx --yes \
	--package semantic-release@25.0.3 \
	--package @semantic-release/commit-analyzer@13.0.1 \
	--package @semantic-release/release-notes-generator@14.1.1 \
	--package @semantic-release/github@12.0.8 \
	--package @semantic-release/gitlab@13.3.2 \
	--package conventional-changelog-conventionalcommits@9.3.1 \
	semantic-release "$@" >"${log_file}" 2>&1
status="$?"
set -e

cat "${log_file}"

if [ "${status}" -ne 0 ]; then
	exit "${status}"
fi

version="$(sed -nE \
	-e 's/.*Published release ([0-9A-Za-z.+-]+).*/\1/p' \
	-e 's/.*The next release version is ([0-9A-Za-z.+-]+).*/\1/p' \
	"${log_file}" | tail -n 1)"

if [ -z "${version}" ]; then
	version="$(git tag --points-at HEAD --sort=-version:refname | sed -nE 's/^v([0-9A-Za-z.+-]+)$/\1/p' | head -n 1)"
fi

if [ -z "${version}" ]; then
	echo "published=false"
	echo "env_tag=${release_branch}"
	exit 0
fi

git_tag="v${version}"

echo "published=true"
echo "version=${version}"
echo "git_tag=${git_tag}"
echo "env_tag=${release_branch}"

if [ -n "${CI_OUTPUT_FILE:-}" ]; then
	{
		echo "published=true"
		echo "version=${version}"
		echo "git_tag=${git_tag}"
		echo "env_tag=${release_branch}"
	} >>"${CI_OUTPUT_FILE}"
fi
