#!/usr/bin/env bash
# 启动确定性工作台；模型和对话配置由外部 coding agent 管理。
set -euo pipefail
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
export GOCACHE="${GOCACHE:-/tmp/interface-is-all-gocache}"
build_dir=$(mktemp -d /tmp/archdesign-run-XXXXXX)
cleanup() { rm -f "$build_dir/archdesign"; rmdir "$build_dir" 2>/dev/null || true; }
trap cleanup EXIT
cd "$repo_dir"
go build -buildvcs=false -o "$build_dir/archdesign" ./cmd/archdesign
"$build_dir/archdesign" serve "$@"
