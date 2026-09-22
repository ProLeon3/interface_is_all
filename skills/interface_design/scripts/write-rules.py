#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
interface_design 规则文件写入脚本：把设计遵循约束写入目标项目的规则文件（带起止标记的块，幂等替换）。

内容依据 ADR-0004：实现前读 .architecture/confirmed.json；不得为迁就代码改基准，需要变更回到设计流程；
每个任务完成后运行 archdesign check 并在对话汇报；接口能力与职责偏离用 review-request / review-import 走能力审查。
块内记录对应的确认 revision；标记块以外的内容不改动；不提交 git。

写哪个文件：目标项目已有 CLAUDE.md 或 AGENTS.md 就写已有的（两个都有则都写），都没有就新建 AGENTS.md。

只在检测到新确认版本后由 skill 调用（state.py 的 phase 为 write_rules）。

用法示例：
  python3 write-rules.py --project /path            # revision 取自 .architecture/confirmed.json
  python3 write-rules.py --project /path --dry-run  # 只输出将要写入的内容与目标文件
"""

import argparse
import json
import os
import re
import sys

BLOCK_BEGIN = re.compile(r"<!--\s*interface_design:begin(?:\s+revision=[0-9a-f]{64})?\s*-->")
BLOCK_END = "<!-- interface_design:end -->"
RULE_FILE_CANDIDATES = ("CLAUDE.md", "AGENTS.md")


def skill_dir():
    """脚本所在的 skill 目录（通过软链的真实路径）；能力审查指令随 skill 分发在其 references/ 下。"""
    scripts_dir = os.path.dirname(os.path.realpath(__file__))
    return os.path.dirname(scripts_dir)


def load_confirmed(project):
    """读取确认快照的记录字段；不存在或损坏时返回 None 与原因。"""
    path = os.path.join(project, ".architecture", "confirmed.json")
    if not os.path.isfile(path):
        return None, "项目没有 .architecture/confirmed.json，尚无已确认基准，不写入规则文件"
    try:
        with open(path, "r", encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, ValueError) as exc:
        return None, "confirmed.json 无法读取：%s" % exc
    revision = data.get("revision")
    if not isinstance(revision, str) or not re.fullmatch(r"[0-9a-f]{64}", revision):
        return None, "confirmed.json 的 revision 无效"
    return {
        "revision": revision,
        "confirmed_at": data.get("confirmed_at", ""),
        "confirmed_by": data.get("confirmed_by", ""),
    }, None


def render_block(confirmed, review_prompt):
    """生成标记块。内容只随确认记录变化，保证同一版本重复写入结果一致。"""
    lines = [
        "<!-- interface_design:begin revision=%s -->" % confirmed["revision"],
        "## 模块与接口设计约束（由 interface_design skill 维护）",
        "",
        "本项目的模块与接口设计已由用户在浏览器工作台确认，基准保存在 `.architecture/confirmed.json`"
        "（确认版本 `%s`，确认人「%s」，确认时间 %s）。后续实现请遵守以下约定：" % (confirmed["revision"], confirmed["confirmed_by"], confirmed["confirmed_at"]),
        "",
        "1. 实现前先读取 `.architecture/confirmed.json`，按其中的模块职责（`modules`）、接口能力（`interfaces`）、协作关系（`collaborations`）与禁止依赖（`forbidden_dependencies`）实现；新代码放在所属模块的目录内。",
        "2. 不得为了迁就代码改写 `.architecture/` 下的基准、草稿或历史版本。需要变更设计时回到设计流程：在对话中调用 `/interface_design` 说明变更，由用户在浏览器重新审阅并确认。",
        "3. 每个任务完成后运行 `archdesign check -project <项目根目录>`（在项目根目录可写 `-project .`），并在对话中如实汇报结果：违规（规则 ID、两端包、位置）、`unassigned_packages`、`empty_modules` 与 `incomplete` 诊断。确定性违规先修改代码再复查，不改基准。",
        "4. 接口能力与模块职责是否偏离设计，程序不做硬检查。任务涉及接口实现或新增公开能力时，运行 `archdesign review-request -project <项目根目录>` 导出能力审查请求，按 `%s` 生成响应后用 `archdesign review-import` 导入，并把结论（均为 `pending_confirmation`）汇报给用户核对。" % review_prompt,
        "5. 以上是给 agent 的指令，不是硬保障：程序只能硬检查被禁止的直接包依赖，`check` 的 `semantic_status` 始终为 `not_run`。不要向用户声称设计已被强制遵守。",
        BLOCK_END,
    ]
    return "\n".join(lines) + "\n"


def apply_block(text, block):
    """幂等替换：已有标记块则原位替换，否则追加到文件末尾；标记块以外的内容原样保留。"""
    begin = BLOCK_BEGIN.search(text)
    end_index = text.find(BLOCK_END, begin.end()) if begin else -1
    if begin and end_index != -1:
        end_index += len(BLOCK_END)
        # 吃掉标记块原有的行尾换行，避免每次替换都多出空行。
        if text[end_index:end_index + 1] == "\n":
            end_index += 1
        new_text = text[:begin.start()] + block + text[end_index:]
        action = "unchanged" if new_text == text else "replaced"
        return new_text, action
    if text and not text.endswith("\n"):
        text += "\n"
    if text and not text.endswith("\n\n"):
        text += "\n"
    return text + block, "appended"


def target_files(project, override):
    if override:
        return [name.strip() for name in override.split(",") if name.strip()]
    existing = [name for name in RULE_FILE_CANDIDATES if os.path.isfile(os.path.join(project, name))]
    return existing or [RULE_FILE_CANDIDATES[1]]


def main():
    parser = argparse.ArgumentParser(
        description="interface_design 规则文件写入脚本：幂等写入带标记的设计遵循约束块。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--project", required=True, help="目标项目根目录")
    parser.add_argument("--files", help="覆盖目标文件（逗号分隔，相对项目根目录）；默认按已有 CLAUDE.md/AGENTS.md 规则选择")
    parser.add_argument("--dry-run", action="store_true", help="只输出将写入的内容与目标，不改文件")
    args = parser.parse_args()

    project = os.path.realpath(args.project)
    confirmed, error = load_confirmed(project)
    if error:
        json.dump({"ok": False, "error": error}, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
        return 2
    # 标记块里写已安装 skill 的绝对路径：后续实现会话从目标项目任意位置都能直接打开这份指令。
    review_prompt = os.path.join(skill_dir(), "references", "capability-review.md")
    block = render_block(confirmed, review_prompt)
    results = []
    for name in target_files(project, args.files):
        path = os.path.join(project, name)
        existed = os.path.isfile(path)
        text = ""
        if existed:
            with open(path, "r", encoding="utf-8") as handle:
                text = handle.read()
        new_text, action = apply_block(text, block)
        if not existed:
            action = "created"
        if not args.dry_run and action != "unchanged":
            # 先写同目录临时文件再替换，避免留下半个文件。
            temp = path + ".interface_design.tmp"
            with open(temp, "w", encoding="utf-8") as handle:
                handle.write(new_text)
            os.replace(temp, path)
        results.append({"file": name, "path": path, "action": action, "existed": existed})
    output = {"ok": True, "dry_run": args.dry_run, "revision": confirmed["revision"], "files": results}
    if args.dry_run:
        output["block"] = block
    json.dump(output, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
