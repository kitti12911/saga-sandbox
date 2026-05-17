#!/usr/bin/env sh
set -eu

repo_dir="${CI_PROJECT_DIR:-$(pwd)}"
cd "${repo_dir}"

rm -rf gen/grpc
mkdir -p gen

# Generate protobuf code when there is input: either local .proto files or a
# remote module configured in buf.gen.yaml (a git_repo input). saga-sandbox
# consumes its contract remotely (proto-sandbox @ tag), so buf generate runs.
if [ -n "$(find proto -name '*.proto' 2>/dev/null)" ] ||
	grep -qE '^[[:space:]]*-[[:space:]]*git_repo:' buf.gen.yaml 2>/dev/null; then
	buf generate
fi

# Keep the CI "generated-code" artifact non-empty using a non-hidden marker
# (upload-artifact excludes hidden files by default), so the lint/test/
# security jobs that download it do not fail on a project with no protos yet.
[ -e gen/keep ] || : >gen/keep
