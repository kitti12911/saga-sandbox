#!/usr/bin/env sh
set -eu

repo_dir="${CI_PROJECT_DIR:-$(pwd)}"
cd "${repo_dir}"

rm -rf gen/grpc

# Generate protobuf code only when .proto files exist. A freshly generated
# project ships an empty proto/ directory; add your .proto files and this
# step starts producing code under gen/grpc/.
if [ -n "$(find proto -name '*.proto' 2>/dev/null)" ]; then
	buf generate
fi
