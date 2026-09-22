#!/usr/bin/env bash
# 在隔离的示例副本中执行浏览器旅程；截图和日志只写入临时目录。
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/.." && pwd)
qa_dir=$(mktemp -d /tmp/interface-workbench-journey-XXXXXX)
session="interface-qa-$$"
server_pid=""
browser_args=()
if [[ -n "${WORKBENCH_BROWSER_CONFIG:-}" ]]; then
  browser_args+=("--config=$WORKBENCH_BROWSER_CONFIG")
fi
export GOCACHE="${GOCACHE:-/tmp/interface-is-all-gocache}"
export GOMODCACHE="${GOMODCACHE:-/tmp/interface-is-all-gomodcache}"
export XDG_CACHE_HOME="$qa_dir/cache"

cleanup() {
  playwright-cli -s="$session" close >/dev/null 2>&1 || true
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  printf '验收产物：%s\n' "$qa_dir"
}
trap cleanup EXIT

mkdir -p "$qa_dir/project"
mkdir -p "$qa_dir/中文 项目"
cp "$repo_dir/examples/shop/go.mod" "$qa_dir/project/"
cp -R "$repo_dir/examples/shop/order" "$repo_dir/examples/shop/payment" "$qa_dir/project/"
cp "$repo_dir/examples/shop/go.mod" "$qa_dir/中文 项目/"
cp -R "$repo_dir/examples/shop/order" "$repo_dir/examples/shop/payment" "$qa_dir/中文 项目/"
cd "$repo_dir"
go build -buildvcs=false -o "$qa_dir/archdesign" ./cmd/archdesign
"$qa_dir/archdesign" serve -project "$qa_dir/project" -addr 127.0.0.1:0 >"$qa_dir/server.log" 2>&1 &
server_pid=$!

# 等待服务明确报告已绑定端口；失败时保留日志并退出，不把超时视为通过。
for attempt in {1..100}; do
  if rg -q '^工作台已启动：' "$qa_dir/server.log"; then break; fi
  if ! kill -0 "$server_pid" 2>/dev/null; then cat "$qa_dir/server.log"; exit 1; fi
  sleep 0.1
done
url=$(sed -n 's/^工作台已启动：//p' "$qa_dir/server.log")
if [[ -z "$url" ]]; then cat "$qa_dir/server.log"; exit 1; fi

cd "$qa_dir"
playwright-cli -s="$session" open "$url" "${browser_args[@]}"
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/workbench-journey.js" | tee "$qa_dir/journey.log"
rg -q 'VERIFIED: CUJ-01' "$qa_dir/journey.log"
if rg -q '^### Error' "$qa_dir/journey.log"; then exit 1; fi
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/workbench-sync-regression.js" | tee "$qa_dir/sync-regression.log"
rg -q 'VERIFIED: 延迟轮询' "$qa_dir/sync-regression.log"
if rg -q '^### Error' "$qa_dir/sync-regression.log"; then exit 1; fi
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/workbench-project-journey.js" | tee "$qa_dir/project-journey.log"
rg -q 'VERIFIED: CUJ-05' "$qa_dir/project-journey.log"
if rg -q '^### Error' "$qa_dir/project-journey.log"; then exit 1; fi
