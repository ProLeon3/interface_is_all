#!/usr/bin/env bash
# 真实浏览器配合页面内固定替身扮演 coding agent，验证导出请求、导入提案到确认交接的程序链路；产物明确标记为回归证据。
set -euo pipefail
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
qa_dir=$(mktemp -d /tmp/interface-ai-journey-XXXXXX)
session="interface-ai-$$"
server_pid=""
export GOCACHE="${GOCACHE:-/tmp/interface-is-all-gocache}"
export XDG_CACHE_HOME="$qa_dir/cache"
cleanup() {
  playwright-cli -s="$session" close >/dev/null 2>&1 || true
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; fi
  printf '固定替身浏览器回归产物：%s\n' "$qa_dir"
}
trap cleanup EXIT
mkdir -p "$qa_dir/project"
cd "$repo_dir"
go build -buildvcs=false -o "$qa_dir/archdesign" ./cmd/archdesign
"$qa_dir/archdesign" serve -project "$qa_dir/project" -addr 127.0.0.1:0 > "$qa_dir/server.log" 2>&1 &
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
playwright-cli -s="$session" open "$url" "${browser_args[@]}"
# 固定替身注册在浏览器上下文中，刷新和新标签页都可用；程序本身不调用模型。
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/fixed-agent.js" | tee "$qa_dir/fixed-agent.log"
rg -q 'fixed agent ready' "$qa_dir/fixed-agent.log"
playwright-cli -s="$session" run-code --filename="$repo_dir/scripts/ai-workbench-journey.js" | tee "$qa_dir/journey.log"
playwright-cli --raw -s="$session" eval "window.__aiJourneyResult" | tee -a "$qa_dir/journey.log"
rg -q '^"?VERIFIED: AI 固定响应回归' "$qa_dir/journey.log"
if rg -q '^### Error' "$qa_dir/journey.log"; then exit 1; fi

# 待审阅提案触发真实离开保护；将刷新与确认拆成独立浏览器操作，避免 CLI 提前返回。
playwright-cli -s="$session" run-code 'async page => { await page.evaluate(() => {setTimeout(() => location.reload(),0);}); }' > "$qa_dir/reload.log"
playwright-cli -s="$session" dialog-accept >> "$qa_dir/reload.log"
playwright-cli -s="$session" run-code 'async page => { await page.locator("#proposal-review").waitFor(); await page.locator("#conflict-banner").waitFor(); if (!await page.locator("#proposal-accept").isDisabled()) throw new Error("刷新后旧提案可覆盖新版本"); await page.evaluate(() => {window.__recoveryVerified=true;}); }' >> "$qa_dir/reload.log"
playwright-cli --raw -s="$session" eval 'window.__recoveryVerified' | tee "$qa_dir/recovery.log"
rg -q '^true$' "$qa_dir/recovery.log"
