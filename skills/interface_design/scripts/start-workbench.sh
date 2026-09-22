#!/usr/bin/env bash
# interface_design 工作台启动脚本：检查默认地址是否已有工作台在监听；没有则以目标项目为 -project 在后台启动。
#
# - 只监听本机回环地址；不读取、不传递任何模型配置或凭据。
# - 已在监听且是工作台时不重复启动，只输出地址（页面里可用「选择项目」切换到目标项目）。
# - 端口被其他服务占用时报告并以退出码 2 结束，交给 skill 说明情况。
# - 输出一行 JSON：{"ok":true,"url":"http://127.0.0.1:8090","started":true|false,"pid":…,"log":"…"}
#
# 用法：
#   start-workbench.sh --project <目标项目> [--addr 127.0.0.1:8090] [--timeout 5m] [--tags <构建标签>] [--wait 15]
set -euo pipefail

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
}

project=""
addr="127.0.0.1:8090"
timeout="5m"
tags=""
wait_seconds=15
while [ $# -gt 0 ]; do
  case "$1" in
    --project) project="$2"; shift 2 ;;
    --addr) addr="$2"; shift 2 ;;
    --timeout) timeout="$2"; shift 2 ;;
    --tags) tags="$2"; shift 2 ;;
    --wait) wait_seconds="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数：$1" >&2; usage >&2; exit 2 ;;
  esac
done
if [ -z "$project" ]; then
  echo "必须提供 --project <目标项目>" >&2
  exit 2
fi
if [ ! -d "$project" ]; then
  echo "目标项目目录不存在：$project" >&2
  exit 2
fi
project=$(cd "$project" && pwd -P)
host="${addr%:*}"
port="${addr##*:}"

# 解析 archdesign：环境变量 ARCHDESIGN_BIN 优先，否则 PATH。
bin="${ARCHDESIGN_BIN:-}"
if [ -z "$bin" ]; then
  bin=$(command -v archdesign || true)
fi
if [ -z "$bin" ] || [ ! -x "$bin" ]; then
  # 与 state.py、export-request.py、SKILL.md 保持同一段安装提示。
  echo "找不到可执行的 archdesign。安装方式：运行 \`go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest\`（需要 Go 1.22+），并确保 \`\$(go env GOPATH)/bin\` 在 PATH 上；或设置环境变量 ARCHDESIGN_BIN 指向已构建的二进制。" >&2
  exit 2
fi

# 用 bash 内建的 /dev/tcp 判断端口是否在监听，不依赖 curl 或 ss。
listening() {
  (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null
}

# 通过 GET /api/session 判断监听者是否为本工作台（响应含 token 与 project）。
is_workbench() {
  local response
  response=$( (exec 3<>"/dev/tcp/$host/$port"; printf 'GET /api/session HTTP/1.0\r\nHost: %s\r\n\r\n' "$addr" >&3; timeout 3 cat <&3) 2>/dev/null || true)
  # 字段顺序不固定，分别检查两个键是否出现。
  case "$response" in
    *'"token"'*) ;;
    *) return 1 ;;
  esac
  case "$response" in
    *'"project"'*) return 0 ;;
    *) return 1 ;;
  esac
}

log_dir="${TMPDIR:-/tmp}/interface_design"
mkdir -p "$log_dir"
log_file="$log_dir/workbench-$port.log"

if listening; then
  if is_workbench; then
    printf '{"ok":true,"url":"http://%s","started":false,"log":"%s","note":"工作台已在监听；若页面显示的当前项目不是目标项目，请在页面「选择项目」中输入：%s"}\n' "$addr" "$log_file" "$project"
    exit 0
  fi
  printf '{"ok":false,"url":"http://%s","started":false,"error":"地址 %s 已被其他服务占用，不是本工作台；请让用户释放端口或改用 --addr 指定其他回环地址"}\n' "$addr" "$addr"
  exit 2
fi

# 后台启动：脱离当前会话，标准输入输出全部重定向，避免 coding agent 的命令结束时把它一起结束。
cmd=("$bin" serve -project "$project" -addr "$addr" -timeout "$timeout")
if [ -n "$tags" ]; then
  cmd+=(-tags "$tags")
fi
if command -v setsid >/dev/null 2>&1; then
  setsid nohup "${cmd[@]}" >"$log_file" 2>&1 </dev/null &
else
  nohup "${cmd[@]}" >"$log_file" 2>&1 </dev/null &
fi
pid=$!

# 等待监听就绪；超时则把日志尾部原样交给调用方。
elapsed=0
while [ "$elapsed" -lt "$wait_seconds" ]; do
  if listening; then
    printf '{"ok":true,"url":"http://%s","started":true,"pid":%s,"log":"%s","project":"%s"}\n' "$addr" "$pid" "$log_file" "$project"
    exit 0
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done
echo "工作台在 ${wait_seconds} 秒内没有开始监听 $addr；日志：$log_file" >&2
tail -n 20 "$log_file" >&2 || true
exit 2
