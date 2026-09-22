#!/usr/bin/env bash
# 真实浏览器与本机固定响应 HTTP 服务验证程序链路，产物明确标记为回归证据。
set -euo pipefail
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
qa_dir=$(mktemp -d /tmp/interface-ai-journey-XXXXXX)
session="interface-ai-$$"
fixture_pid=""
server_pid=""
export GOCACHE="${GOCACHE:-/tmp/interface-is-all-gocache}"
export XDG_CACHE_HOME="$qa_dir/cache"
cleanup() {
  playwright-cli -s="$session" close >/dev/null 2>&1 || true
  if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; fi
  if [[ -n "$fixture_pid" ]]; then kill "$fixture_pid" 2>/dev/null || true; fi
  printf '固定响应浏览器回归产物：%s\n' "$qa_dir"
}
trap cleanup EXIT
mkdir -p "$qa_dir/project"
python3 "$repo_dir/scripts/ai-fixture.py" > "$qa_dir/fixture.log" 2>&1 &
fixture_pid=$!
for attempt in {1..100}; do
  if rg -q '^http://' "$qa_dir/fixture.log"; then break; fi
  sleep 0.1
done
export ARCHDESIGN_AI_BASE_URL
ARCHDESIGN_AI_BASE_URL=$(head -n 1 "$qa_dir/fixture.log")
export ARCHDESIGN_AI_MODEL="fixed-regression-fixture"
export ARCHDESIGN_AI_API_KEY=""
export ARCHDESIGN_AI_API="chat_completions"
export ARCHDESIGN_AI_REASONING_EFFORT=""
export ARCHDESIGN_AI_RESPONSE_FORMAT="json_schema"
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
