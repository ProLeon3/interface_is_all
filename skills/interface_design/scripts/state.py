#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
interface_design 状态脚本：只读地从目标项目的 .architecture/ 目录重建当前阶段，输出 JSON。

skill 每次被唤醒都先运行本脚本，用它的 phase 与 next_action 决定本回合动作，
而不是让 agent 手工翻文件或依赖对话记忆（见 ADR-0003）。

本脚本只读取文件、执行 archdesign 的只读命令（help / show）和访问工作台的
GET /api/session；不写任何文件，也从不运行 confirm、proposal-accept、proposal-discard。

阶段判断顺序严格对应《coding agent 接入实现依据》的阶段判断规则表，命中第一条即输出：

  archdesign_unavailable        archdesign 不可用（工作台能否启动由 start-workbench.sh 判断）
  answer_requests               存在未应答且仍可导入的请求
  adjust_proposal               存在待审阅提案，且本回合用户给出修改意见（--has-feedback）
  await_review                  存在待审阅提案，用户没有新意见
  await_baseline_confirmation   源码分析已接受但未确认
  initial_analysis              有 go.mod、无确认基准、无有效分析状态
  design                        无基准且非 Go 项目，或已有基准，且用户给出需求（--has-requirement）
  write_rules                   confirmed.json 的 revision 比规则文件标记块记录的新
  report_status                 以上都不命中

用法示例：
  python3 state.py --project /path/to/project
  python3 state.py --project /path/to/project --has-requirement
  python3 state.py --project /path/to/project --has-feedback
"""

import argparse
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import urllib.request

# 规则文件标记块的起止标记；revision 记录在起始标记内，便于状态脚本比对。
BLOCK_BEGIN = re.compile(r"<!--\s*interface_design:begin(?:\s+revision=([0-9a-f]{64}))?\s*-->")
BLOCK_END = "<!-- interface_design:end -->"
# 目标项目规则文件候选；已有的都写，都没有则新建 AGENTS.md（写入由 write-rules.py 负责）。
RULE_FILE_CANDIDATES = ("CLAUDE.md", "AGENTS.md")
DEFAULT_ADDR = "127.0.0.1:8090"
SUMMARY_LIMIT = 300


def skill_paths():
    """解析脚本所在的 skill 目录与本仓库根目录（通过软链的真实路径）。"""
    scripts_dir = os.path.dirname(os.path.realpath(__file__))
    skill_dir = os.path.dirname(scripts_dir)
    repo_dir = os.path.realpath(os.path.join(skill_dir, "..", ".."))
    prompts = os.path.join(repo_dir, "docs", "agent-prompts")
    return {
        "skill_dir": skill_dir,
        "scripts_dir": scripts_dir,
        "repo_dir": repo_dir,
        "prompt_docs": {
            "design": os.path.join(prompts, "design.md"),
            "source_analysis": os.path.join(prompts, "source-analysis.md"),
            "capability_review": os.path.join(prompts, "capability-review.md"),
        },
    }


def resolve_archdesign(repo_dir):
    """优先环境变量 ARCHDESIGN_BIN，否则 PATH 上的 archdesign；再用 help 子命令确认可执行。"""
    hint = (
        "安装方式：在本仓库 %s 运行 `go install ./cmd/archdesign`（需要 Go 1.22+，"
        "并确保 `$(go env GOPATH)/bin` 在 PATH 上）；或设置环境变量 ARCHDESIGN_BIN 指向已构建的二进制。" % repo_dir
    )
    env = os.environ.get("ARCHDESIGN_BIN", "").strip()
    if env:
        info = {"path": env, "source": "ARCHDESIGN_BIN", "available": False, "install_hint": hint}
        if not (os.path.isfile(env) and os.access(env, os.X_OK)):
            info["error"] = "ARCHDESIGN_BIN 指向的文件不存在或不可执行"
            return info
    else:
        found = shutil.which("archdesign")
        info = {"path": found, "source": "PATH", "available": False, "install_hint": hint}
        if not found:
            info["error"] = "PATH 上没有 archdesign，环境变量 ARCHDESIGN_BIN 也未设置"
            return info
    try:
        result = subprocess.run([info["path"], "help"], capture_output=True, text=True, timeout=15)
    except (OSError, subprocess.SubprocessError) as exc:
        info["error"] = "无法执行 archdesign help：%s" % exc
        return info
    if result.returncode != 0 or "archdesign" not in result.stdout:
        info["error"] = "archdesign help 返回异常：%s" % (result.stderr.strip() or result.stdout.strip())
        return info
    info["available"] = True
    return info


def run_archdesign(binary, args, timeout=120):
    """执行 archdesign 只读命令，返回 (退出码, 标准输出, 标准错误)。"""
    try:
        result = subprocess.run([binary] + args, capture_output=True, text=True, timeout=timeout)
    except (OSError, subprocess.SubprocessError) as exc:
        return 2, "", str(exc)
    return result.returncode, result.stdout, result.stderr


def workbench_status(addr):
    """检查默认地址是否已有工作台在监听；通过 GET /api/session 区分工作台与其他服务。"""
    host, _, port = addr.rpartition(":")
    info = {"addr": addr, "url": "http://%s" % addr, "listening": False, "is_workbench": False, "initial_project": None}
    try:
        with socket.create_connection((host, int(port)), timeout=1.5):
            info["listening"] = True
    except (OSError, ValueError) as exc:
        info["error"] = "端口未监听：%s" % exc
        return info
    try:
        request = urllib.request.Request("http://%s/api/session" % addr, headers={"Accept": "application/json"})
        with urllib.request.urlopen(request, timeout=3) as response:
            data = json.loads(response.read().decode("utf-8"))
        if isinstance(data, dict) and "token" in data and isinstance(data.get("project"), dict):
            info["is_workbench"] = True
            info["initial_project"] = data["project"].get("path")
        else:
            info["error"] = "端口有监听，但 /api/session 的响应不是工作台格式"
    except Exception as exc:  # noqa: BLE001 - 任何异常都只说明它不是可用的工作台
        info["error"] = "端口有监听，但不是工作台或无法访问 /api/session：%s" % exc
    return info


def read_json(path, warnings, label):
    """读取 JSON 文件；不存在返回 None，损坏则记录警告并返回 None。"""
    if not os.path.isfile(path):
        return None
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError) as exc:
        warnings.append("%s 无法读取或不是有效 JSON：%s" % (label, exc))
        return None


def truncate(text, limit=SUMMARY_LIMIT):
    if text is None:
        return None
    text = str(text)
    return text if len(text) <= limit else text[:limit] + "…"


def load_proposals(arch_dir, warnings):
    """读取 proposals/ 中所有提案的摘要信息；提案本体不可变，request_id 用于判断请求是否已应答。"""
    proposals = {}
    directory = os.path.join(arch_dir, "proposals")
    if not os.path.isdir(directory):
        return proposals
    for name in sorted(os.listdir(directory)):
        if not name.endswith(".json"):
            continue
        data = read_json(os.path.join(directory, name), warnings, "提案 %s" % name)
        if not isinstance(data, dict):
            continue
        request = data.get("request") or {}
        proposals[name[:-5]] = {
            "id": data.get("id", name[:-5]),
            "request_id": data.get("request_id"),
            "parent_id": data.get("parent_id") or "",
            "has_source": request.get("source") is not None,
            "created_at": data.get("created_at"),
            "expected_draft_hash": data.get("expected_draft_hash", ""),
            "expected_revision": data.get("expected_revision", ""),
            "summary": truncate(data.get("summary")),
            "file": os.path.join(directory, name),
        }
    return proposals


def load_requests(arch_dir, warnings):
    """读取 requests/ 中所有请求的摘要信息（不含源码正文）。"""
    requests = []
    directory = os.path.join(arch_dir, "requests")
    if not os.path.isdir(directory):
        return requests
    for name in sorted(os.listdir(directory)):
        if not name.endswith(".json"):
            continue
        data = read_json(os.path.join(directory, name), warnings, "请求 %s" % name)
        if not isinstance(data, dict):
            continue
        request = data.get("request") or {}
        requests.append({
            "request_id": data.get("request_id", name[:-5]),
            "kind": data.get("kind"),
            "parent_id": data.get("parent_id") or "",
            "has_source": request.get("source") is not None,
            "expected_draft_hash": data.get("expected_draft_hash", ""),
            "expected_revision": data.get("expected_revision", ""),
            "requirement": truncate(request.get("requirement")),
            "instruction": truncate(request.get("instruction")),
            "selection": request.get("selection"),
            "file": os.path.join(directory, name),
        })
    return requests


def initial_analysis_status(arch_dir, proposals, confirmed, warnings):
    """按 store.InitialAnalysis 的规则推导源码分析状态：none/pending/accepted/withdrawn/confirmed/corrupt。"""
    data = read_json(os.path.join(arch_dir, "initial-analysis.json"), warnings, "initial-analysis.json")
    if data is None:
        return {"status": "none"}
    proposal_id = data.get("proposal_id") or ""
    info = {
        "proposal_id": proposal_id,
        "accepted": bool(data.get("accepted")),
        "accepting": bool(data.get("accepting")),
        "withdrawn": bool(data.get("withdrawn")),
        "requires_acceptance": bool(data.get("requires_acceptance")),
    }
    if proposal_id not in proposals:
        warnings.append("initial-analysis.json 引用的提案 %s 不存在，源码分析记录损坏" % proposal_id)
        info["status"] = "corrupt"
        return info
    confirmed_ids = set()
    if confirmed:
        confirmed_ids = {confirmed.get("initial_analysis_id"), confirmed.get("source_analysis_id")}
    if proposal_id in confirmed_ids:
        info["status"] = "confirmed"
    elif info["withdrawn"]:
        info["status"] = "withdrawn"
    elif info["accepted"] or info["accepting"]:
        info["status"] = "accepted"
    else:
        info["status"] = "pending"
    info["request_id"] = proposals[proposal_id]["request_id"]
    return info


def pending_design_proposal(arch_dir, proposals, warnings):
    """按 store.PendingDesignProposal 的规则读取待审阅的普通设计提案 ID。"""
    data = read_json(os.path.join(arch_dir, "design-proposal.json"), warnings, "design-proposal.json")
    if data is None:
        return ""
    proposal_id = data.get("proposal_id") or ""
    if proposal_id and proposal_id not in proposals:
        warnings.append("design-proposal.json 引用的提案 %s 不存在" % proposal_id)
        return ""
    if data.get("accepted") or data.get("withdrawn"):
        return ""
    return proposal_id


def request_block_reasons(request, draft_hash, revision, initial, pending_design_id):
    """镜像 store.checkProposalParent 与版本校验：返回导入必然被拒绝的原因列表，空列表表示仍可应答。"""
    reasons = []
    if request["expected_draft_hash"] != (draft_hash or ""):
        reasons.append("导出后草稿已变化")
    if request["expected_revision"] != (revision or ""):
        reasons.append("导出后确认版本已变化")
    source_based = request["has_source"]
    parent = request["parent_id"]
    active_initial = initial.get("status") in ("pending", "accepted")
    if active_initial:
        if not source_based:
            reasons.append("源码分析尚未确认，普通设计请求会被拒绝")
        elif initial["status"] == "accepted":
            reasons.append("源码分析已接受，只能在浏览器确认或撤回，不能再调整")
        elif initial.get("proposal_id") != parent:
            reasons.append("父提案不是当前待审阅的源码分析提案")
    else:
        if source_based and parent:
            reasons.append("源码分析调整请求的父提案已不再待审阅")
        elif (pending_design_id or "") != (parent or ""):
            reasons.append("父提案与当前待审阅的设计提案不一致")
    return reasons


def rules_status(project, revision):
    """检查目标项目规则文件中的标记块及其记录的 revision。"""
    files = []
    for name in RULE_FILE_CANDIDATES:
        path = os.path.join(project, name)
        entry = {"file": name, "path": path, "exists": os.path.isfile(path), "has_block": False, "block_revision": None}
        if entry["exists"]:
            try:
                with open(path, "r", encoding="utf-8") as handle:
                    text = handle.read()
                match = BLOCK_BEGIN.search(text)
                if match and BLOCK_END in text:
                    entry["has_block"] = True
                    entry["block_revision"] = match.group(1)
            except (OSError, UnicodeDecodeError) as exc:
                entry["error"] = str(exc)
        files.append(entry)
    targets = [f["file"] for f in files if f["exists"]] or [RULE_FILE_CANDIDATES[1]]
    stale = False
    if revision:
        for entry in files:
            if entry["file"] in targets and (not entry["has_block"] or entry["block_revision"] != revision):
                stale = True
    return {"files": files, "targets": targets, "stale": stale}


def build_state(args):
    warnings = []
    paths = skill_paths()
    project = os.path.realpath(args.project)
    arch_dir = os.path.join(project, ".architecture")
    state = {
        "project": project,
        "architecture_dir": arch_dir,
        "architecture_exists": os.path.isdir(arch_dir),
        "has_go_mod": os.path.isfile(os.path.join(project, "go.mod")),
        "inputs": {"has_requirement": args.has_requirement, "has_feedback": args.has_feedback},
        "skill": paths,
        "prompt_docs": paths["prompt_docs"],
        "warnings": warnings,
    }
    if not os.path.isdir(project):
        state["warnings"].append("目标项目目录不存在：%s" % project)
    state["archdesign"] = resolve_archdesign(paths["repo_dir"])
    state["workbench"] = workbench_status(args.addr)
    binary = state["archdesign"]["path"] if state["archdesign"]["available"] else None

    # 已确认基准：文件直读取摘要，再用 archdesign show 复核完整性（历史快照与指纹）。
    confirmed_raw = read_json(os.path.join(arch_dir, "confirmed.json"), warnings, "confirmed.json")
    confirmed = None
    if isinstance(confirmed_raw, dict):
        confirmed = {
            "revision": confirmed_raw.get("revision", ""),
            "parent_revision": confirmed_raw.get("parent_revision", ""),
            "confirmed_at": confirmed_raw.get("confirmed_at"),
            "confirmed_by": confirmed_raw.get("confirmed_by"),
            "initial_analysis_id": confirmed_raw.get("initial_analysis_id", ""),
            "source_analysis_id": confirmed_raw.get("source_analysis_id", ""),
            "valid": None,
        }
        if binary:
            code, _, err = run_archdesign(binary, ["show", "-project", project])
            confirmed["valid"] = code == 0
            if code != 0:
                warnings.append("archdesign show 拒绝当前基准：%s" % err.strip())
    state["confirmed"] = confirmed
    revision = confirmed["revision"] if confirmed and confirmed.get("valid") is not False else ""

    # 草稿指纹只能由程序计算；archdesign 不可用时标记 unknown。
    draft_path = os.path.join(arch_dir, "draft.json")
    draft = {"exists": os.path.isfile(draft_path), "draft_hash": None}
    if draft["exists"] and binary:
        code, out, err = run_archdesign(binary, ["show", "-draft", "-project", project])
        if code == 0:
            try:
                draft["draft_hash"] = json.loads(out).get("draft_hash")
            except ValueError as exc:
                warnings.append("archdesign show -draft 输出无法解析：%s" % exc)
        else:
            warnings.append("archdesign show -draft 失败：%s" % err.strip())
    elif draft["exists"]:
        draft["draft_hash"] = "unknown"
    state["draft"] = draft
    draft_hash = draft["draft_hash"] if draft["draft_hash"] not in (None, "unknown") else ""

    proposals = load_proposals(arch_dir, warnings)
    requests = load_requests(arch_dir, warnings)
    initial = initial_analysis_status(arch_dir, proposals, confirmed, warnings)
    pending_design_id = pending_design_proposal(arch_dir, proposals, warnings)
    state["initial_analysis"] = initial

    # 待审阅提案：源码分析提案记录在 initial-analysis.json，普通设计提案记录在 design-proposal.json。
    pending = None
    if initial.get("status") == "pending":
        info = proposals[initial["proposal_id"]]
        pending = dict(info, kind="reanalysis" if info["expected_revision"] else "initial_analysis")
    elif pending_design_id:
        pending = dict(proposals[pending_design_id], kind="design")
    state["pending_proposal"] = pending

    # 未应答请求：requests/ 中没有任何提案引用其 request_id；再区分仍可导入与必然被拒绝的。
    answered = {p["request_id"] for p in proposals.values() if p.get("request_id")}
    unanswered, stale = [], []
    for request in requests:
        if request["request_id"] in answered:
            continue
        reasons = request_block_reasons(request, draft_hash, revision, initial, pending_design_id)
        if draft["draft_hash"] == "unknown":
            reasons.append("archdesign 不可用，无法核对草稿指纹")
        if reasons:
            stale.append(dict(request, blocked_reasons=reasons))
        else:
            unanswered.append(request)
    state["unanswered_requests"] = unanswered
    state["stale_requests"] = stale
    state["proposal_count"] = len(proposals)
    state["rules"] = rules_status(project, revision)

    if initial.get("status") == "withdrawn" and revision:
        warnings.append("上一次重新分析已撤回：在接受新的重新分析提案前，浏览器不允许确认新版本；需要时向用户建议重新分析")
    if not revision and state["rules"]["files"] and any(f["has_block"] for f in state["rules"]["files"]):
        warnings.append("规则文件已有标记块，但项目没有有效的确认基准")
    if state["workbench"]["listening"] and not state["workbench"]["is_workbench"]:
        warnings.append("默认地址被其他服务占用：%s" % state["workbench"].get("error"))

    decide_phase(state, revision)
    return state


def decide_phase(state, revision):
    """严格按阶段判断规则表的顺序命中第一条。"""
    scripts = state["skill"]["scripts_dir"]
    project = state["project"]
    export = "python3 %s --project %s" % (os.path.join(scripts, "export-request.py"), project)
    docs = state["prompt_docs"]
    has_go_mod = state["has_go_mod"]
    initial_status = state["initial_analysis"].get("status")
    pending = state["pending_proposal"]
    remind = "回合结束时明确告诉用户：打开 %s，在工作台审阅；做完后回到对话说「继续」。" % state["workbench"]["url"]

    if not state["archdesign"]["available"]:
        state["phase"] = "archdesign_unavailable"
        state["next_action"] = "说明 archdesign 不可用的原因与安装方式，然后停止本回合。" + " " + state["archdesign"]["install_hint"]
        state["commands"] = []
        return
    if state["unanswered_requests"]:
        state["phase"] = "answer_requests"
        state["next_action"] = (
            "逐个读取未应答请求文件，按其 response_schema 与对应指令文档（含 source 用 source_analysis，否则用 design）生成响应并 "
            "proposal-import；导入被拒绝时原样转述错误并停止。" + remind
        )
        state["commands"] = [
            "%s proposal-import -project %s -file <响应文件>" % (state["archdesign"]["path"], project)
        ]
        return
    if pending and state["inputs"]["has_feedback"]:
        state["phase"] = "adjust_proposal"
        state["next_action"] = (
            "以待审阅提案 %s 为父提案导出调整请求（kind 固定为 design，父提案含源码时响应仍需 analysis），"
            "把用户意见作为 instruction，生成响应并导入。" % pending["id"] + remind
        )
        state["commands"] = [
            "%s --kind design --parent-id %s --instruction '<用户本回合的修改意见>'" % (export, pending["id"]),
        ]
        return
    if pending:
        state["phase"] = "await_review"
        state["next_action"] = (
            "不要重复生成。提醒用户在浏览器审阅提案 %s：接受并保存草稿后审阅并确认，或选中对象导出调整请求，"
            "或直接在对话里说修改意见。" % pending["id"] + remind
        )
        state["commands"] = []
        return
    if initial_status == "accepted":
        state["phase"] = "await_baseline_confirmation"
        state["next_action"] = (
            "源码分析已接受为草稿但尚未确认：提醒用户在浏览器「审阅并确认」现状基准；确认前不导出设计请求。" + remind
        )
        state["commands"] = []
        return
    if has_go_mod and not revision and initial_status in ("none", "withdrawn"):
        state["phase"] = "initial_analysis"
        state["next_action"] = (
            "导出 initial_analysis 请求；导出失败时原样转述程序输出并停止（不截断、不分批、不改用其他请求类型）。"
            "成功后依据 %s 生成含 analysis 的响应并导入。" % docs["source_analysis"] + remind
        )
        state["commands"] = ["%s --kind initial_analysis" % export]
        return
    if ((not revision and not has_go_mod) or revision) and state["inputs"]["has_requirement"]:
        state["phase"] = "design"
        state["next_action"] = (
            # 先判断需求是否已被当前设计覆盖，避免对未变化的需求反复生成只改措辞的提案。
            "先对照 archdesign show -draft 判断需求是否已被当前设计覆盖：已覆盖则不导出请求，告知用户后不带 --has-requirement 重新运行本脚本；"
            "否则以用户需求为 requirement 导出 design 请求，依据 %s 生成响应并导入，无关对象的文字逐字保留。" % docs["design"]
            + ("（规则文件标记块落后于当前确认版本，本回合先处理需求；提醒用户下一次不带新需求的唤醒会写入规则块。）" if state["rules"]["stale"] else "")
            + remind
        )
        state["commands"] = ["%s --kind design --requirement-file <需求文件>" % export]
        return
    if revision and state["rules"]["stale"]:
        state["phase"] = "write_rules"
        state["next_action"] = (
            "confirmed.json 的 revision（%s）比规则文件标记块记录的新：运行 write-rules.py 写入或替换标记块，"
            "然后在对话中汇报确认版本、写入的文件与约束内容；不提交 git。" % revision
        )
        state["commands"] = ["python3 %s --project %s" % (os.path.join(scripts, "write-rules.py"), project)]
        return
    state["phase"] = "report_status"
    state["next_action"] = "汇报当前状态（基准版本、草稿、待审阅提案、规则文件），询问用户需求或下一步。"
    state["commands"] = []


def main():
    parser = argparse.ArgumentParser(
        description="interface_design 状态脚本：从目标项目 .architecture/ 重建阶段并输出 JSON（只读）。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--project", required=True, help="目标项目根目录")
    parser.add_argument("--addr", default=DEFAULT_ADDR, help="工作台监听地址（默认 %s）" % DEFAULT_ADDR)
    parser.add_argument("--has-requirement", action="store_true", help="本回合用户给出了需求（对应规则表第 7 行）")
    parser.add_argument("--has-feedback", action="store_true", help="本回合用户对待审阅提案给出了修改意见（对应规则表第 3 行）")
    parser.add_argument("--compact", action="store_true", help="单行输出 JSON")
    args = parser.parse_args()
    state = build_state(args)
    if args.compact:
        json.dump(state, sys.stdout, ensure_ascii=False)
    else:
        json.dump(state, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
