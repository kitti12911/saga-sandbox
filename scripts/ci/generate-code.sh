#!/usr/bin/env sh
set -eu

repo_dir="${CI_PROJECT_DIR:-$(pwd)}"
cd "${repo_dir}"

# saga-sandbox consumes its proto contract remotely (see buf.gen.yaml:
# git_repo proto-sandbox @ tag). buf generate always has input, so it runs
# unconditionally — there is no local proto/ directory.
rm -rf gen/grpc
buf generate
