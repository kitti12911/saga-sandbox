#!/usr/bin/env sh
set -eu

repo_dir="$(pwd)"
cd "${repo_dir}"

rm -rf gen/grpc
buf generate
