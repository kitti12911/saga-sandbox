#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
"${script_dir}/require-env.sh" REPOSITORY BRANCH_SYNC_TOKEN

repo_dir="${CI_PROJECT_DIR:-$(pwd)}"
cd "${repo_dir}"

stable_branch="${STABLE_BRANCH:-main}"
prerelease_branches="${PRERELEASE_BRANCHES:-uat develop}"

git config user.name "${GIT_USER_NAME:-homelab-branch-sync[bot]}"
git config user.email "${GIT_USER_EMAIL:-homelab-branch-sync[bot]@users.noreply.github.com}"
git remote set-url origin "https://x-access-token:${BRANCH_SYNC_TOKEN}@github.com/${REPOSITORY}.git"

# prerelease_branches is intentionally a space-separated list passed to git as
# multiple positional args; the same applies to the loop below.
# shellcheck disable=SC2086
git fetch origin "${stable_branch}" ${prerelease_branches}

previous_branch="${stable_branch}"
# shellcheck disable=SC2086
for branch in ${prerelease_branches}; do
	git checkout -B "${branch}" "origin/${branch}"
	if [ "${previous_branch}" = "${stable_branch}" ]; then
		merge_ref="origin/${previous_branch}"
	else
		merge_ref="${previous_branch}"
	fi
	git merge --ff-only "${merge_ref}"
	git push origin "${branch}"
	previous_branch="${branch}"
done
