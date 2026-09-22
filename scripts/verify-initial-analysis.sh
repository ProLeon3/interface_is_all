#!/usr/bin/env bash
# 真实浏览器配合页面内固定替身扮演 coding agent，验证源码分析请求导出、提案导入、确认与重新分析的程序链路；产物明确标记为回归证据。
set -euo pipefail
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
qa_dir=$(mktemp -d /tmp/interface-initial-journey-XXXXXX)
session="interface-initial-$$"
server_pid=""
export GOCACHE="${GOCACHE:-/tmp/interface-is-all-gocache}"
export XDG_CACHE_HOME="$qa_dir/cache"
cleanup() {
  playwright-cli -s="$session" close >/dev/null 2>&1 || true
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; fi
  printf '固定替身浏览器回归产物：%s\n' "$qa_dir"
}
trap cleanup EXIT
mkdir -p "$qa_dir/landing" "$qa_dir/existing"
cp "$repo_dir/examples/shop/go.mod" "$qa_dir/existing/"
cp -R "$repo_dir/examples/shop/order" "$repo_dir/examples/shop/payment" "$qa_dir/existing/"
cd "$repo_dir"
go build -buildvcs=false -o "$qa_dir/archdesign" ./cmd/archdesign
"$qa_dir/archdesign" serve -project "$qa_dir/landing" -addr 127.0.0.1:0 > "$qa_dir/server.log" 2>&1 &
server_pid=$!
for attempt in {1..100}; do
  if rg -q '^工作台已启动：' "$qa_dir/server.log"; then break; fi
  if ! kill -0 "$server_pid" 2>/dev/null; then cat "$qa_dir/server.log"; exit 1; fi
  sleep 0.1
done
url=$(sed -n 's/^工作台已启动：//p' "$qa_dir/server.log")
if [[ -z "$url" ]]; then cat "$qa_dir/server.log"; exit 1; fi
browser_args=()
if [[ -n "${WORKBENCH_BROWSER_CONFIG:-}" ]]; then browser_args+=("--config=$WORKBENCH_BROWSER_CONFIG"); fi
cd "$qa_dir"
# 功能修复的定向复跑可以复用已有有效截图，避免重复视觉检查。
if [[ "${WORKBENCH_SKIP_SCREENSHOTS:-}" == "1" ]]; then url="$url/?skip-captures"; fi
playwright-cli -s="$session" open "$url" "${browser_args[@]}"
# 固定替身注册在浏览器上下文中，刷新和新标签页都可用；程序本身不调用模型。
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/fixed-agent.js" | tee "$qa_dir/fixed-agent.log"
rg -q 'fixed agent ready' "$qa_dir/fixed-agent.log"
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/initial-analysis-journey.js" | tee "$qa_dir/journey.log"
playwright-cli --raw -s="$session" eval "window.__initialJourneyResult" | tee -a "$qa_dir/journey.log"
rg -q '^"?VERIFIED: 初次分析浏览器流程' "$qa_dir/journey.log"
if rg -q '^### Error' "$qa_dir/journey.log"; then exit 1; fi
# 同一个隔离项目已有基准后继续验证重新分析，仍由固定替身扮演 agent。
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/reanalysis-journey.js" | tee "$qa_dir/reanalysis-journey.log"
playwright-cli --raw -s="$session" eval "window.__reanalysisJourneyResult" | tee -a "$qa_dir/reanalysis-journey.log"
rg -q '^"?VERIFIED: 已有基准重新分析' "$qa_dir/reanalysis-journey.log"
if rg -q '^### Error' "$qa_dir/reanalysis-journey.log"; then exit 1; fi
