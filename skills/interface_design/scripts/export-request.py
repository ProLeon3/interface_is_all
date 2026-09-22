#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
interface_design 导出请求脚本：把需求、修改意图、选中对象和父提案组装成意图 JSON，
调用 `archdesign design-request` 导出请求，并输出便于 agent 使用的摘要 JSON。

- 成功时输出 ok=true，含 request_id、请求文件路径（.architecture/requests/<id>.json）、
  应遵循的指令文档路径、源码范围摘要（不含源码正文，正文以请求文件为准）。
- 失败时输出 ok=false，原样保留程序的标准错误；源码分析导出失败时另外保留程序返回的
  范围与原因（scope、diagnostics、packages、error），完整输出写入 full_output_file。
  agent 必须原样转述，不截断、不分批、不改用其他请求类型。

本脚本只导出请求，不生成提案、不接受、不确认。

用法示例：
  python3 export-request.py --project /path --kind initial_analysis
  python3 export-request.py --project /path --kind design --requirement-file spec.md
  python3 export-request.py --project /path --kind design --parent-id <提案ID> --instruction "把存储拆成独立模块"
"""

import argparse
import json
import os
import subprocess
import sys
import tempfile

SUMMARY_LIMIT = 400
# 找不到 archdesign 时的统一安装提示；与 state.py、start-workbench.sh、SKILL.md 保持同一段文字。
INSTALL_HINT = (
    "安装方式：运行 `go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest`（需要 Go 1.22+），"
    "并确保 `$(go env GOPATH)/bin` 在 PATH 上；或设置环境变量 ARCHDESIGN_BIN 指向已构建的二进制。"
)


def skill_paths():
    """解析 skill 目录（通过软链的真实路径），指令文档随 skill 一起分发在 <skill-dir>/references/。"""
    scripts_dir = os.path.dirname(os.path.realpath(__file__))
    skill_dir = os.path.dirname(scripts_dir)
    prompts = os.path.join(skill_dir, "references")
    return {
        "design": os.path.join(prompts, "design.md"),
        "source_analysis": os.path.join(prompts, "source-analysis.md"),
    }


def resolve_archdesign():
    """与 state.py 相同的解析规则：ARCHDESIGN_BIN 优先，否则 PATH 上的 archdesign。"""
    env = os.environ.get("ARCHDESIGN_BIN", "").strip()
    if env:
        return env
    for directory in os.environ.get("PATH", "").split(os.pathsep):
        candidate = os.path.join(directory, "archdesign")
        if os.path.isfile(candidate) and os.access(candidate, os.X_OK):
            return candidate
    return None


def read_text(path):
    with open(path, "r", encoding="utf-8") as handle:
        return handle.read()


def truncate(text, limit=SUMMARY_LIMIT):
    if text is None:
        return None
    return text if len(text) <= limit else text[:limit] + "…"


def build_intent(args):
    """只放入非空字段；archdesign 的意图 JSON 拒绝未知字段。"""
    intent = {"kind": args.kind}
    requirement = args.requirement
    if args.requirement_file:
        requirement = read_text(args.requirement_file)
    if requirement:
        intent["requirement"] = requirement
    instruction = args.instruction
    if args.instruction_file:
        instruction = read_text(args.instruction_file)
    if instruction:
        intent["instruction"] = instruction
    if args.parent_id:
        intent["parent_id"] = args.parent_id
    if args.selection_kind:
        intent["selection"] = {"kind": args.selection_kind, "id": args.selection_id or ""}
    if args.expected_draft_hash is not None:
        intent["expected_draft_hash"] = args.expected_draft_hash
    if args.expected_revision is not None:
        intent["expected_revision"] = args.expected_revision
    return intent


def source_summary(source):
    """源码范围摘要：文件清单与行数、包清单、构建范围、诊断；不复制源码正文。"""
    if not isinstance(source, dict):
        return None
    facts = source.get("facts") or {}
    files = facts.get("files") or []
    return {
        "fingerprint": source.get("fingerprint"),
        "scope": facts.get("scope"),
        "diagnostics": facts.get("diagnostics") or [],
        "packages": [
            {
                "import_path": p.get("import_path"),
                "directory": p.get("directory"),
                "files": p.get("files") or [],
                "excluded": p.get("excluded") or [],
            }
            for p in (facts.get("packages") or [])
        ],
        "file_count": len(files),
        "files": [
            {"path": f.get("path"), "package_path": f.get("package_path"), "lines": len((f.get("content") or "").split("\n"))}
            for f in files
        ],
        "bytes": len(json.dumps(source, ensure_ascii=False).encode("utf-8")),
    }


def failure_output(args, code, stdout, stderr, work_dir):
    """导出失败：原样保留 stderr；若程序还输出了 {error, source}，保留范围与原因，完整输出落盘。"""
    result = {"ok": False, "exit_code": code, "stderr": stderr.rstrip("\n")}
    if stdout.strip():
        full_path = os.path.join(work_dir, "design-request-failure.json")
        with open(full_path, "w", encoding="utf-8") as handle:
            handle.write(stdout)
        result["full_output_file"] = full_path
        try:
            payload = json.loads(stdout)
        except ValueError:
            result["stdout"] = stdout.rstrip("\n")
            return result
        if isinstance(payload, dict):
            result["error"] = payload.get("error")
            summary = source_summary(payload.get("source"))
            if summary:
                # 失败范围只保留清单，不保留正文；完整输出在 full_output_file 中。
                summary.pop("files", None)
                result["source_scope"] = summary
    return result


def main():
    parser = argparse.ArgumentParser(
        description="interface_design 导出请求脚本：组装意图 JSON 并调用 archdesign design-request。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--project", required=True, help="目标项目根目录")
    parser.add_argument("--kind", default="design", choices=["design", "initial_analysis", "reanalysis"], help="请求类型（默认 design；继续调整提案必须用 design）")
    parser.add_argument("--requirement", help="总体需求文字")
    parser.add_argument("--requirement-file", help="需求文件（例如用户给的 spec.md），整份内容作为 requirement")
    parser.add_argument("--instruction", help="本次修改意图（用户的修改意见）")
    parser.add_argument("--instruction-file", help="修改意图文件")
    parser.add_argument("--parent-id", help="以该待审阅提案为父提案继续调整")
    parser.add_argument("--selection-kind", choices=["design", "module", "interface", "collaboration"], help="用户选中对象的类型")
    parser.add_argument("--selection-id", help="用户选中对象的 ID")
    parser.add_argument("--expected-draft-hash", help="可选：导出时预期的草稿指纹，不一致则程序拒绝导出")
    parser.add_argument("--expected-revision", help="可选：导出时预期的确认版本，不一致则程序拒绝导出")
    parser.add_argument("--tags", help="Go 构建标签（仅源码分析）")
    parser.add_argument("--timeout", default="2m", help="archdesign 超时（默认 2m）")
    parser.add_argument("--work-dir", help="临时文件目录（默认系统临时目录下 interface_design/）")
    args = parser.parse_args()

    binary = resolve_archdesign()
    if not binary:
        json.dump({"ok": False, "exit_code": 2, "stderr": "找不到 archdesign。" + INSTALL_HINT}, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
        return 2
    work_dir = args.work_dir or os.path.join(tempfile.gettempdir(), "interface_design")
    os.makedirs(work_dir, exist_ok=True)
    intent = build_intent(args)
    intent_file = os.path.join(work_dir, "intent-%s.json" % os.getpid())
    with open(intent_file, "w", encoding="utf-8") as handle:
        json.dump(intent, handle, ensure_ascii=False, indent=2)

    command = [binary, "design-request", "-project", args.project, "-kind", args.kind, "-file", intent_file, "-timeout", args.timeout]
    if args.tags:
        command += ["-tags", args.tags]
    try:
        completed = subprocess.run(command, capture_output=True, text=True)
    finally:
        try:
            os.remove(intent_file)
        except OSError:
            pass

    if completed.returncode != 0:
        json.dump(failure_output(args, completed.returncode, completed.stdout, completed.stderr, work_dir), sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
        return completed.returncode

    request = json.loads(completed.stdout)
    body = request.get("request") or {}
    has_source = body.get("source") is not None
    docs = skill_paths()
    schema = request.get("response_schema") or {}
    result = {
        "ok": True,
        "request_id": request.get("request_id"),
        "kind": request.get("kind"),
        "has_source": has_source,
        "request_file": os.path.join(os.path.realpath(args.project), ".architecture", "requests", "%s.json" % request.get("request_id")),
        "parent_id": request.get("parent_id") or "",
        "expected_draft_hash": request.get("expected_draft_hash"),
        "expected_revision": request.get("expected_revision"),
        "requirement": truncate(body.get("requirement")),
        "instruction": truncate(body.get("instruction")),
        "selection": body.get("selection"),
        "before_modules": [m.get("id") for m in (body.get("design") or {}).get("modules", [])],
        "prompt_doc": docs["source_analysis"] if has_source else docs["design"],
        "response_required_fields": schema.get("required"),
        "response_schema_location": "请求文件中的 response_schema 字段",
    }
    if has_source:
        result["source"] = source_summary(body.get("source"))
    json.dump(result, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
