#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
interface_design 响应自检脚本：在 proposal-import 之前，对照请求文件检查 agent 生成的响应。

程序导入时校验结构、引用、目录和源码位置；本脚本补上指令文档要求但程序不检查的部分：
字数上限、无关对象是否被改写、依据条目数量、行号复述、范围套话、Go 标识符当能力名等。

- errors：指令文档的硬性要求未满足（字数超限、依据缺失或重复、位置越界、禁止规则非空……），
  应修正响应后再导入。
- warnings：可能偏离「精简、准确、可信」（改写了未点名的对象、explanation 复述行号、空话……），
  由 agent 自行判断是否修正。

脚本只读文件、不改任何文件、不调用 archdesign。退出码：0 无 error，1 有 error，2 输入无法读取。

用法：
  python3 lint-response.py --request <请求文件> --response <响应文件>
"""

import argparse
import json
import re
import sys

# 字数上限与 references/design.md、source-analysis.md 的表一致；改这里要同步改文档。
LIMITS = {
    "summary": 100,
    "responsibility": 60,
    "name": 12,
    "description": 40,
    "semantics": 40,
    "purpose": 40,
    "reason": 40,
    "explanation": 80,
    "finding": 80,
    "uncertainty": 60,
}
ID_PATTERN = re.compile(r"^[a-z][a-z0-9._-]{0,127}$")
# 纯 ASCII 标识符（如 Cancel、RequestRefund）不该直接当能力名称。
IDENTIFIER_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_.]*$")
# explanation 里复述行号：「18–79 行」「第 36 行」「36-42 行」「L18」。
LINE_MENTION = re.compile(r"(第\s*\d+\s*行|\d+\s*[–\-~]\s*\d+\s*行|\d+\s*行|\bL\d+\b)")
# 请求已声明的范围限制，uncertainties 不该再复述。
SCOPE_BOILERPLATE = re.compile(r"(_test\.go|测试(文件|代码)?不在|其他构建条件|嵌套\s*module|vendor|testdata)")
# 不增加信息的措辞。
FILLER = re.compile(r"(等[，。；、）\s]|等$|各种|相关(的)?(功能|逻辑|业务)|进行处理|负责处理)")
GROUPS = (("modules", "模块"), ("interfaces", "接口"), ("collaborations", "协作"), ("forbidden_dependencies", "禁止规则"))


def load(path):
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


class Report:
    def __init__(self):
        self.errors = []
        self.warnings = []

    def error(self, message):
        self.errors.append(message)

    def warn(self, message):
        self.warnings.append(message)


# 字数口径：一个汉字算 1，一个英文或数字词（标识符、路径、参数名）算 1，标点和空格不算。
TOKEN = re.compile(r"[\u3400-\u9fff\uf900-\ufaff]|[A-Za-z0-9_][A-Za-z0-9_./\-]*")


def text_length(text):
    return len(TOKEN.findall(text or ""))


def length_check(report, path, text, limit, changed=True):
    """超限只对本次新写或修改的文字报 error；保留原文的对象不受字数约束。"""
    if not isinstance(text, str):
        return
    n = text_length(text)
    if n > limit and changed:
        report.error("%s 有 %d 字，超过上限 %d 字" % (path, n, limit))
    if FILLER.search(text) and changed:
        report.warn("%s 含不增加信息的措辞（等/各种/相关功能/进行处理）：%s" % (path, text.strip()[:40]))


def by_id(items):
    return {item.get("id"): item for item in items or [] if isinstance(item, dict)}


def module_of(design, file_path):
    """按设计的模块目录判断文件归属，与程序的 ModuleAt 相同：最长匹配的根目录。"""
    directory = file_path.rsplit("/", 1)[0] if "/" in file_path else "."
    best, best_len = "", -1
    for module in design.get("modules") or []:
        root = (module.get("root") or "").strip("/")
        if root in ("", "."):
            if best_len < 0:
                best, best_len = module.get("id"), 0
            continue
        if directory == root or directory.startswith(root + "/"):
            if len(root) > best_len:
                best, best_len = module.get("id"), len(root)
    return best


def check_design_text(report, design, before, rewrite_all):
    """设计文字：字数、空话、ID 规范、Go 标识符当名称、semantics 复述 description。"""
    before_groups = {key: by_id(before.get(key)) for key, _ in GROUPS}
    rewritten = []
    for key, label in GROUPS:
        for index, item in enumerate(design.get(key) or []):
            path = "design.%s[%d]" % (key, index)
            item_id = item.get("id", "")
            if not ID_PATTERN.match(item_id or ""):
                report.error("%s.id 不符合命名规则（小写字母开头，只含小写字母、数字、点、下划线、短横线）：%r" % (path, item_id))
            old = before_groups[key].get(item_id)
            changed = rewrite_all or old is None or old != item
            if old is not None and old != item and not rewrite_all:
                rewritten.append("%s %s" % (label, item_id))
            if key == "modules":
                length_check(report, path + ".responsibility", item.get("responsibility"), LIMITS["responsibility"], changed)
            elif key == "interfaces":
                name = item.get("name") or ""
                length_check(report, path + ".name", name, LIMITS["name"], changed)
                if changed and IDENTIFIER_NAME.match(name.strip()):
                    report.warn("%s.name 是 Go 标识符「%s」，能力名称应是中文名词短语，标识符可放进 description 的括号里" % (path, name))
                length_check(report, path + ".description", item.get("description"), LIMITS["description"], changed)
                semantics = item.get("semantics") or {}
                for field in ("inputs", "outputs", "errors"):
                    value = semantics.get(field)
                    if value is None:
                        continue
                    length_check(report, "%s.semantics.%s" % (path, field), value, LIMITS["semantics"], changed)
                    if changed and value.strip() in ("无", "无。", "-", "—"):
                        report.warn("%s.semantics.%s 写了「%s」；没有必要就省略整个 semantics" % (path, field, value.strip()))
                    if changed and value.strip() == (item.get("description") or "").strip():
                        report.warn("%s.semantics.%s 复述了 description" % (path, field))
            elif key == "collaborations":
                length_check(report, path + ".purpose", item.get("purpose"), LIMITS["purpose"], changed)
            else:
                length_check(report, path + ".reason", item.get("reason"), LIMITS["reason"], changed)
    if rewritten:
        report.warn("以下对象在请求的 design 中已存在且文字被改写，请确认都与本次需求或修改意图相关，否则逐字保留原文：" + "、".join(rewritten))


def check_analysis(report, response, request_body):
    """分析部分：依据一一对应、位置有效且落在归属模块、supported 协作有调用方位置、文字上限。"""
    analysis = response.get("analysis")
    design = response.get("design") or {}
    if not isinstance(analysis, dict):
        report.error("含源码的请求必须返回 analysis 对象")
        return
    for field in ("evidence", "issues", "suggestions", "uncertainties"):
        if not isinstance(analysis.get(field), list):
            report.error("analysis.%s 必须是数组（没有内容就给空数组）" % field)
    if design.get("forbidden_dependencies"):
        report.error("源码分析的 forbidden_dependencies 必须为空数组，禁止规则由用户决定")
    files = {}
    for file in ((request_body.get("source") or {}).get("facts") or {}).get("files") or []:
        files[file.get("path")] = (file.get("content") or "").split("\n")
    # 已扫描包必须被模块目录覆盖，与程序的检查一致，提前给出可读的原因。
    for package in ((request_body.get("source") or {}).get("facts") or {}).get("packages") or []:
        if package.get("files") and not module_of(design, (package.get("directory") or ".").rstrip("/") + "/x.go"):
            report.error("已扫描包 %s（目录 %s）没有被任何模块目录覆盖" % (package.get("import_path"), package.get("directory")))

    def check_locations(path, locations, owner=None, extra_owner=None):
        if not isinstance(locations, list) or not locations:
            report.error("%s.locations 不能为空" % path)
            return False, False
        owner_hit, extra_hit = False, False
        for i, loc in enumerate(locations):
            file_path = loc.get("file")
            if file_path not in files:
                report.error("%s.locations[%d] 引用了本次未读取的文件：%s" % (path, i, file_path))
                continue
            lines = files[file_path]
            line, end = loc.get("line"), loc.get("end_line")
            if not isinstance(line, int) or not isinstance(end, int) or line < 1 or end < line or end > len(lines):
                report.error("%s.locations[%d] 行号越界：%s:%s–%s（文件共 %d 行）" % (path, i, file_path, line, end, len(lines)))
                continue
            column = loc.get("column", 1)
            if not isinstance(column, int) or column < 1 or column > len(lines[line - 1]) + 1:
                report.error("%s.locations[%d] column 无效：%s:%d 列 %s" % (path, i, file_path, line, column))
            module = module_of(design, file_path)
            if owner and module == owner:
                owner_hit = True
            if extra_owner and module == extra_owner:
                extra_hit = True
        return owner_hit, extra_hit

    owners, kinds = {}, {}
    for module in design.get("modules") or []:
        owners[module["id"]], kinds[module["id"]] = module["id"], "module"
    interface_owner = {}
    for api in design.get("interfaces") or []:
        owners[api["id"]], kinds[api["id"]] = api.get("module_id"), "interface"
        interface_owner[api["id"]] = api.get("module_id")
    targets = {}
    for link in design.get("collaborations") or []:
        owners[link["id"]], kinds[link["id"]] = link.get("from"), "collaboration"
        targets[link["id"]] = interface_owner.get(link.get("interface_id"))
    seen = {}
    for index, item in enumerate(analysis.get("evidence") or []):
        path = "analysis.evidence[%d]" % index
        item_id, kind = item.get("id"), item.get("kind")
        if item_id not in kinds:
            report.error("%s 引用了设计里不存在的对象：%s" % (path, item_id))
            continue
        if kinds[item_id] != kind:
            report.error("%s 的 kind 应为 %s，实际 %s：%s" % (path, kinds[item_id], kind, item_id))
        if item_id in seen:
            report.error("%s 重复给出了 %s 的依据（每个对象恰好一项）" % (path, item_id))
        seen[item_id] = True
        if item.get("status") not in ("supported", "uncertain"):
            report.error("%s.status 必须是 supported 或 uncertain" % path)
        explanation = item.get("explanation") or ""
        length_check(report, path + ".explanation", explanation, LIMITS["explanation"])
        if LINE_MENTION.search(explanation):
            report.warn("%s.explanation 复述了行号，locations 已经带了位置：%s" % (path, explanation[:40]))
        owner_hit, target_hit = check_locations(path, item.get("locations"), owners[item_id], targets.get(item_id))
        if item.get("locations") and not owner_hit:
            report.error("%s 的位置没有落在对象所属模块 %s 的目录内：%s" % (path, owners[item_id], item_id))
        if kind == "collaboration" and item.get("status") == "supported" and item.get("locations") and not owner_hit:
            report.error("%s 标了 supported，但没有调用方模块 %s 里的使用语句位置" % (path, owners[item_id]))
    missing = [i for i in kinds if i not in seen]
    if missing:
        report.error("以下对象缺少 analysis.evidence：" + "、".join(missing))
    for field in ("issues", "suggestions"):
        for index, finding in enumerate(analysis.get(field) or []):
            path = "analysis.%s[%d]" % (field, index)
            description = finding.get("description") or ""
            if not description.strip():
                report.error("%s.description 不能为空" % path)
            length_check(report, path + ".description", description, LIMITS["finding"])
            if LINE_MENTION.search(description):
                report.warn("%s.description 复述了行号" % path)
            check_locations(path, finding.get("locations"))
    for index, text in enumerate(analysis.get("uncertainties") or []):
        path = "analysis.uncertainties[%d]" % index
        if not isinstance(text, str) or not text.strip():
            report.error("%s 不能是空说明" % path)
            continue
        length_check(report, path, text, LIMITS["uncertainty"])
        if SCOPE_BOILERPLATE.search(text):
            report.warn("%s 复述了请求已声明的范围限制，页面已展示，只写本项目特有的不确定项：%s" % (path, text[:40]))


def main():
    parser = argparse.ArgumentParser(
        description="interface_design 响应自检脚本：对照请求文件检查响应是否满足指令文档的要求。",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("--request", required=True, help="程序导出的请求文件（.architecture/requests/<id>.json）")
    parser.add_argument("--response", required=True, help="agent 生成的响应文件")
    args = parser.parse_args()
    try:
        request, response = load(args.request), load(args.response)
    except (OSError, ValueError) as exc:
        json.dump({"ok": False, "errors": ["无法读取输入：%s" % exc], "warnings": []}, sys.stdout, ensure_ascii=False, indent=2)
        sys.stdout.write("\n")
        return 2
    report = Report()
    body = request.get("request") or {}
    has_source = body.get("source") is not None
    kind = request.get("kind")

    if response.get("request_id") != request.get("request_id"):
        report.error("request_id 与请求文件不一致，导入会被拒绝；不要改写 request_id，改为复制请求文件里的值")
    expected_keys = {"request_id", "summary", "design"} | ({"analysis"} if has_source else set())
    actual_keys = set(response.keys())
    for key in sorted(expected_keys - actual_keys):
        report.error("响应缺少顶层字段：%s" % key)
    for key in sorted(actual_keys - expected_keys):
        report.error("响应多出了 response_schema 不允许的顶层字段：%s" % key)

    summary = response.get("summary") or ""
    if not summary.strip():
        report.error("summary 不能为空")
    length_check(report, "summary", summary, LIMITS["summary"])

    design = response.get("design") if isinstance(response.get("design"), dict) else {}
    if not design:
        report.error("design 必须是完整设计对象")
    else:
        if design.get("schema_version") != 1:
            report.error("design.schema_version 必须为 1")
        for key in ("modules", "interfaces", "forbidden_dependencies"):
            if not isinstance(design.get(key), list):
                report.error("design.%s 必须提供（可为空数组）" % key)
        # 源码分析的设计文字全部按代码重写，一律受字数约束；需求设计只约束新写或修改的对象。
        check_design_text(report, design, body.get("design") or {}, rewrite_all=has_source)
        if has_source:
            check_analysis(report, response, body)
        elif design == body.get("design"):
            report.warn("design 与请求里的当前设计完全相同；只有需求已被完整覆盖时才这样返回，并在 summary 说明理由")

    stats = {
        "kind": kind,
        "has_source": has_source,
        "modules": len(design.get("modules") or []),
        "interfaces": len(design.get("interfaces") or []),
        "collaborations": len(design.get("collaborations") or []),
        "forbidden_dependencies": len(design.get("forbidden_dependencies") or []),
        "summary_length": text_length(summary),
    }
    output = {"ok": not report.errors, "errors": report.errors, "warnings": report.warnings, "stats": stats}
    json.dump(output, sys.stdout, ensure_ascii=False, indent=2)
    sys.stdout.write("\n")
    return 1 if report.errors else 0


if __name__ == "__main__":
    sys.exit(main())
