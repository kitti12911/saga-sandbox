#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
"${script_dir}/require-env.sh" GIT_WORKDIR GIT_ADD_PATH GIT_COMMIT_MESSAGE

push_remote="${GIT_PUSH_REMOTE:-origin}"
push_ref="${GIT_PUSH_REF:-HEAD:main}"
rebase_branch="${GIT_REBASE_BRANCH:-main}"
attempts="${GIT_PUSH_ATTEMPTS:-3}"

cd "${GIT_WORKDIR}"

if git diff --quiet; then
	echo "${GIT_WORKDIR} has no changes to commit."
	exit 0
fi

git config user.name "${GIT_USER_NAME:-ci[bot]}"
git config user.email "${GIT_USER_EMAIL:-ci[bot]@example.invalid}"
git add "${GIT_ADD_PATH}"

if git diff --cached --quiet; then
	echo "${GIT_ADD_PATH} has no staged changes to commit."
	exit 0
fi

git commit -m "${GIT_COMMIT_MESSAGE}"

attempt=1
while [ "${attempt}" -le "${attempts}" ]; do
	if git push "${push_remote}" "${push_ref}"; then
		exit 0
	fi

	if [ -n "${rebase_branch}" ]; then
		git fetch "${push_remote}" "${rebase_branch}"
		git rebase "${push_remote}/${rebase_branch}"
	fi

	attempt=$((attempt + 1))
done

echo "Unable to push changes after ${attempts} attempts." >&2
exit 1
