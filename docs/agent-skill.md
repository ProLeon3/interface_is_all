# interface_design skill：实现与验收记录

实现日期：2026-09-22。依据为[coding agent 接入实现依据](../coding%20agent%20接入实现依据.md)、[ADR-0003](adr/0003-turn-based-skill-integration.md) 与 [ADR-0004](adr/0004-design-compliance-via-project-rules-file.md)。程序（Go 代码、前端、Schema）零改动；本次只新增 skill 文件、辅助脚本、本文与 README 的使用说明。

本文区分「程序链路已验证」与「真实 agent 生成质量」：前者由本文的命令记录支撑；后者只能在真实使用中观察，见文末 UNVERIFIED 列表。

> 2026-09-22 补充：本文写成后同日实施了 skill 分发方案。三份指令文档已从 `docs/agent-prompts/` 移到 `skills/interface_design/references/`，`archdesign` 的安装命令改为 `go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest`，skill 改用 `npx skills add ProLeon3/interface_is_all` 安装。下文交付物表、「安装与发现」与验证记录里的旧路径、软链与旧命令是当时的原始记录，保留原文不改；新方案的实现选择与验证记录见 [docs/skill-distribution.md](skill-distribution.md)。

## 交付物

| 路径 | 作用 |
| --- | --- |
| `skills/interface_design/SKILL.md` | skill 正文：硬性边界、准备步骤、按 `phase` 的动作与话术、响应生成与导入、规则文件写入、放弃提案、回复模板 |
| `skills/interface_design/scripts/state.py` | 只读状态脚本：从目标项目 `.architecture/` 重建阶段，输出 JSON（`phase`、`next_action`、`commands` 等） |
| `skills/interface_design/scripts/export-request.py` | 组装意图 JSON 并调用 `archdesign design-request`；成功输出请求摘要与应遵循的指令文档路径，失败原样保留程序输出 |
| `skills/interface_design/scripts/write-rules.py` | 把设计遵循约束写入目标项目规则文件的标记块，幂等替换 |
| `skills/interface_design/scripts/start-workbench.sh` | 检查 `127.0.0.1:8090` 是否已有工作台；没有则以目标项目为 `-project` 后台启动 |
| `skills/interface_design/examples/` | 本次验收中由本文作者（扮演 agent）手写并成功导入的响应样例与需求样例；只示范格式，`request_id` 绑定当时的项目路径，不能照抄 |
| `~/.agents/skills/interface_design` | 软链 → 本仓库 `skills/interface_design`（绝对路径） |
| `~/.claude/skills/interface_design` | 相对软链 → `../../.agents/skills/interface_design`，与本机其他 skill 的惯例一致 |

## 实现选择

### 回合制与阶段重建

skill 不依赖对话记忆。每次唤醒先运行 `start-workbench.sh`，再运行 `state.py --project <project> [--has-requirement] [--has-feedback]`。两个开关是 agent 从本回合用户输入判断后传入的（依据表中「用户给出需求」「用户本回合给出修改意见」两个条件无法从文件得知），其余全部从文件与程序只读命令得出。`state.py` 的 `phase` 与阶段判断规则表逐行对应，顺序相同，命中第一条即输出：

| 规则表行 | `phase` | 判断来源 |
| --- | --- | --- |
| `archdesign` 不可用或工作台无法启动 | `archdesign_unavailable` | `ARCHDESIGN_BIN` 或 PATH 上的 `archdesign` 能否执行 `help`；工作台能否启动由 `start-workbench.sh` 单独报告 |
| 存在未应答的请求 | `answer_requests` | `requests/` 中没有任何 `proposals/*.json` 的 `request_id` 引用的请求，且按 `store.checkProposalParent` 与版本校验仍可导入 |
| 存在待审阅提案，且用户本回合给出修改意见 | `adjust_proposal` | `initial-analysis.json`（未接受、未撤回、未确认）或 `design-proposal.json`（未接受、未撤回）加 `--has-feedback` |
| 存在待审阅提案，用户没有新意见 | `await_review` | 同上，无 `--has-feedback` |
| 源码分析已接受但未确认 | `await_baseline_confirmation` | `initial-analysis.json` 的 `accepted`/`accepting` 为真，且 `confirmed.json` 的 `initial_analysis_id`/`source_analysis_id` 不等于其提案 ID |
| 有 `go.mod`、无确认基准、无分析状态 | `initial_analysis` | `go.mod` 存在、`confirmed.json` 不存在、分析状态为 `none` 或 `withdrawn` |
| 无确认基准且不是 Go 项目，或已有基准，且用户给出需求 | `design` | 前述条件加 `--has-requirement` |
| `confirmed.json` 的 `revision` 比规则文件标记块记录的新 | `write_rules` | 目标规则文件（已有的 `CLAUDE.md`/`AGENTS.md`，都没有则 `AGENTS.md`）缺少标记块或块内 `revision` 不等于当前 `revision` |
| 以上都不命中 | `report_status` | — |

### 与程序约束一致的细节

- 草稿指纹不在 Python 里重算，改用 `archdesign show -draft` 取 `draft_hash`；基准用 `archdesign show` 复核完整性。`archdesign` 不可用时指纹标为 `unknown`，阶段直接是 `archdesign_unavailable`。
- 「未应答请求」额外区分「仍可导入」与「必然被拒绝」：导出后草稿或基准已变、父提案与当前待审阅提案不一致、源码分析未确认时的普通设计请求等情况列入 `stale_requests`，不再应答，只向用户提一句。否则 skill 会在每个回合反复应答一个永远导不进去的旧请求。
- 继续调整提案一律用 `design` 请求加 `parent_id`（`PrepareDesignRequest` 要求）；父提案含源码时请求自动携带同一份源码，`export-request.py` 输出的 `has_source` 与 `prompt_doc` 告诉 agent 响应仍需 `analysis`。
- 源码分析导出失败时 `export-request.py` 原样输出程序标准错误，并把程序返回的 `{error, source}` 中的构建范围、诊断与包清单摘录到 `source_scope`，完整输出落盘到 `full_output_file`；不截断源码、不分批、不改类型。
- 规则文件标记块：`<!-- interface_design:begin revision=<64 位十六进制> -->` … `<!-- interface_design:end -->`。块内容只随确认记录（`revision`、确认人、时间）变化；已有块原位替换，无块追加到文件末尾，块外字节不动；写入用同目录临时文件 `os.replace`。块内五条约定按 ADR-0004：实现前读 `confirmed.json`；不为迁就代码改基准，变更回到设计流程；每个任务完成后 `archdesign check` 并汇报；能力与职责偏离走 `review-request`/`review-import`；明示这是指令不是硬保障。
- 工作台由脚本用 `setsid nohup` 启动，只监听回环地址，不传任何模型配置；日志在 `${TMPDIR:-/tmp}/interface_design/workbench-<port>.log`。端口被非工作台服务占用时按第一行规则报告并停止。

### 依据文档没有写明、由实现者判断的事项

1. `stale_requests` 的区分（见上）。依据只说「存在未应答的请求」；不区分会造成死循环。
2. 已撤回（`withdrawn`）的源码分析视为「无分析状态」：无基准时回到 `initial_analysis`；已有基准时程序允许导出设计请求但不允许确认，`state.py` 以 `warnings` 提示 agent 向用户建议重新分析（重新分析仍不自动触发）。
3. `design` 与 `write_rules` 同时满足时按表的顺序先做 `design`，`next_action` 里附一句提醒「规则块将在下一次不带新需求的唤醒时写入」。没有改成同一回合两件事，以保持「命中第一条即执行」。
4. 新出现的规则文件（例如用户后来新建了 `CLAUDE.md`）没有标记块时也算「落后」，下一次唤醒会补写；两个文件都存在时两个都写。
5. 响应格式样例放在 `skills/interface_design/examples/`，SKILL.md 注明只示范格式。
6. `state.py` 增加 `--has-requirement`/`--has-feedback` 两个开关，让规则表第 3、4、7 行在脚本输出里成为可区分的 `phase`，而不是让 agent 自己在 SKILL.md 里再分支。
7. `design` 阶段先判断需求是否已被当前设计覆盖（2026-09-22 补充）。起因：同一份只描述现状的 README 连跑两次，因为两次运行时 `design.md` 的字数要求不同，每次都生成只改措辞的提案。已覆盖时不出提案，改为不带 `--has-requirement` 重跑状态脚本；这是 agent 的语义判断，放在 SKILL.md，`state.py` 只在 `next_action` 里提示。同时 `design.md` 要求无关对象的文字逐字保留，字数限制只约束新写或修改的文字，以免部分变化的需求也夹带无关改写。

## 安装与发现

- `go install -buildvcs=false ./cmd/archdesign` → `/home/pr0le0n/go/bin/archdesign`；本机 `$(go env GOPATH)/bin` 已在 PATH 上。不在 PATH 上时把 `$(go env GOPATH)/bin` 加进 PATH，或设置 `ARCHDESIGN_BIN`。
- `ls -la ~/.claude/skills ~/.agents/skills` 核实的惯例：`~/.agents/skills/<name>` 是真实目录或绝对软链，`~/.claude/skills/<name>` 是相对软链 `../../.agents/skills/<name>`。已按此创建两条软链（见交付物表）。
- Claude Code 发现：已验证。创建软链后本会话的可用 skill 列表立即出现 `interface_design` 及其 description。
- Codex 发现：**UNVERIFIED**。本机 `codex-cli 0.155.1`。查到的线索：二进制内嵌文本写明 skill 安装到 `$CODEX_HOME/skills/<skill-name>`（默认 `~/.codex/skills`），另有一处把 `.agents/skills` 列在「external-agent-migration」模块的字符串表里；`~/.agents/skills/` 下有空文件 `.codex`，`~/.codex/config.toml` 把 `/home/pr0le0n/.agents/skills` 列为受信任项目。这些都不能证明 Codex 会扫描 `~/.agents/skills`。未查到官方文档，未做实测。若 Codex 找不到，可另建 `~/.codex/skills/interface_design -> ~/.agents/skills/interface_design`（本次未建）。

## 验证记录

环境：Go 1.26.6，python3 3.12.3，`GOCACHE=/tmp/interface-is-all-gocache GOMODCACHE=/tmp/interface-is-all-gomodcache`。原始输出保存在 `/tmp/interface-design-verify/`（临时目录，不入库）。下文提案 ID 与 `revision` 只取前 12 位。

### 静态检查

| 命令 | 结果 |
| --- | --- |
| `bash -n scripts/start-workbench.sh` | 通过 |
| `python3 -m py_compile state.py export-request.py write-rules.py` | 通过 |
| 三个 Python 脚本与 bash 脚本的 `--help` | 均输出用法 |
| `go build -buildvcs=false -o /tmp/archdesign ./cmd/archdesign` | 通过 |
| `go test ./...` | 通过（`check`、`cmd/archdesign`、`design`、`designer`、`store`、`workbench` ok） |
| `go vet ./...` | 通过，无输出 |

### 两阶段旅程（`examples/todo` 副本，去掉 `.architecture`，位于 `/tmp/interface-design-todo-ZxsikP`）

「接受」与「确认」在真实流程中由用户在浏览器完成，skill 不运行 `proposal-accept` 与 `confirm`；下表中它们只是命令行测试替身。

| 步骤 | 命令 | 观察结果 |
| --- | --- | --- |
| 1 | `start-workbench.sh --project <T>` | 未监听 → 后台启动，`started:true`，`http://127.0.0.1:8090`，`/api/session` 返回的项目为 `<T>`；再次调用返回 `started:false`（已在监听，不重复启动） |
| 2 | `state.py --project <T>` | `phase=initial_analysis`，`has_go_mod=true`，无 `confirmed` |
| 3 | `export-request.py --kind initial_analysis` | `ok:true`，请求 `d088337a…`，`has_source:true`，`prompt_doc=docs/agent-prompts/source-analysis.md`，源码 3 文件、15175 字节 |
| 4 | `state.py` | `phase=answer_requests`，`unanswered_requests=[d088337a…]` |
| 5 | 按源码分析指令手写响应（`examples/initial-analysis-response.json`），`archdesign proposal-import` | 一次通过，提案 `ee5dd9a2…`，`request_id` 对应 |
| 6 | `state.py` / `--has-feedback` / `--has-requirement` | `await_review` / `adjust_proposal`（命令含 `--parent-id ee5dd9a2…`）/ `await_review`（待审阅提案优先于需求） |
| 7 | 替身 `proposal-accept -id ee5dd9a2…` 后 `state.py --has-requirement` | `phase=await_baseline_confirmation`，`initial_analysis.status=accepted`；即使给了需求也不导出设计请求 |
| 8 | 此时强行 `export-request.py --kind design` | 程序拒绝：`设计版本已变化，请重新审阅后确认`（与 skill 规则一致的程序侧保护） |
| 9 | 替身 `confirm -expected none` | `revision=942f262f…`，`initial_analysis_id=ee5dd9a2…` |
| 10 | `state.py` / `--has-requirement` | `write_rules`（基准已有、规则块缺失）/ `design`（`next_action` 附规则块落后提醒） |
| 11 | `export-request.py --kind design --requirement-file spec.md` | 请求 `2d48031a…`，`expected_revision=942f262f…`，`before_modules=[cli, task, storage]` |
| 12 | 手写设计响应（`examples/design-response.json`）并导入 | 提案 `2fa4cabe…`；`state.py` → `await_review`，`--has-feedback` → `adjust_proposal` |
| 13 | `export-request.py --kind design --parent-id 2fa4cabe… --selection-kind interface --selection-id task-service --instruction "…"` | 请求 `1b54e577…`，`parent_id=2fa4cabe…`；`state.py` → `answer_requests`（导出未导入的请求被识别为未应答） |
| 14 | 手写调整响应（`examples/adjust-response.json`）并导入 | 提案 `fef8e92b…`，`parent_id=2fa4cabe…`；`state.py` → `await_review`，`unanswered=0` |
| 15 | 替身接受 `fef8e92b…` 并 `confirm -expected 942f262f…` | `revision=eef88c2b…` |
| 16 | `state.py` → `write-rules.py` → `state.py` | `write_rules`（目标 `AGENTS.md`）→ `AGENTS.md created` → `report_status`，块内 `revision=eef88c2b…` |
| 17 | 在 `AGENTS.md` 块前后加入用户内容，新建无块的 `CLAUDE.md`；导出第二个设计请求 `a173942b…` | — |
| 18 | `save-draft` 写入改动后的草稿，`state.py` | `unanswered=0`，`stale_requests=[a173942b…: 导出后草稿已变化]`；恢复原草稿后 `state.py` → `answer_requests` |
| 19 | 手写第二份设计响应并导入 `c7eec414…`；替身接受并 `confirm -expected eef88c2b…` | `revision=ad666351…` |
| 20 | `state.py` | `write_rules`，目标 `[CLAUDE.md, AGENTS.md]`，`AGENTS.md` 块记录 `eef88c2b…`，`CLAUDE.md` 无块 |
| 21 | `write-rules.py` 两次 | 第一次 `CLAUDE.md appended`、`AGENTS.md replaced`；第二次两者 `unchanged`；去掉标记块后两文件内容与写入前逐字一致，每个文件恰好一个块 |
| 22 | `archdesign check -project .` | `status=passed`，0 违规，`unassigned_packages=[]`，`semantic_status=not_run`，`design_revision=ad666351…` |

### 其他项目类型

| 场景 | 命令 | 结果 |
| --- | --- | --- |
| 空目录（新项目） | `state.py --project <空目录>` / `--has-requirement` | `report_status` / `design`（不经过源码分析） |
| 已有基准的项目（`examples/todo`） | `state.py --project examples/todo` / `--has-requirement` | `write_rules`（示例目录没有规则文件，属预期）/ `design` |

### 失败与拒绝分支

| 分支 | 触发方式 | 观察结果 |
| --- | --- | --- |
| `archdesign` 不可用 | `ARCHDESIGN_BIN=/nonexistent/archdesign` | `phase=archdesign_unavailable`，`error=ARCHDESIGN_BIN 指向的文件不存在或不可执行`，附 `install_hint` |
| 同上 | `PATH=/usr/bin:/bin` | `error=PATH 上没有 archdesign，环境变量 ARCHDESIGN_BIN 也未设置` |
| 工作台地址被其他服务占用 | `python3 -m http.server 8091` 后 `start-workbench.sh --addr 127.0.0.1:8091` | `ok:false`，`地址 127.0.0.1:8091 已被其他服务占用，不是本工作台…`，退出码 2；`state.py --addr …` 的 `workbench.is_workbench=false` 并给出 warning |
| 源码超过 1 MiB | 生成 2.9 MB 的合法 Go 文件后 `export-request.py --kind initial_analysis` | `ok:false`，`stderr=完整源码上下文为 7931763 字节，超过首版 1 MiB 上限；未截断或提交给模型`，`source_scope` 含构建范围与包清单，完整输出在 `full_output_file` |
| 有 `go.mod` 无源码 | 空模块目录 | `stderr=当前构建范围内没有可分析的 Go 源码` |
| 导入被拒绝：结构无效 | 删除响应的 `summary` | `提案响应结构无效：jsonschema: '' does not validate … missing properties: 'summary'` |
| 导入被拒绝：伪造 `request_id` | `request_id` 改为 64 个 `0` | `无法读取本项目的设计请求：open …/requests/000…0.json: no such file or directory` |
| 导入被拒绝：重复导入已有提案的请求 | 再次导入同一响应 | `设计版本已变化，请重新审阅后确认` |

以上拒绝信息均由程序产生，skill 只原样转述；测试中没有修改请求文件或 `request_id`。

## UNVERIFIED

- **真实 agent 生成质量**：本次三份源码分析/设计响应由本文作者扮演 agent 手写并一次导入成功，只证明程序链路与 SKILL.md 的指引可行，不证明真实模型在真实项目上能稳定产出通过校验、语义正确的响应。
- **Codex 对 `~/.agents/skills` 的发现方式**：见「安装与发现」。
- **真实用户在浏览器确认后的回合衔接**：本次用命令行 `proposal-accept`/`confirm` 代替浏览器操作，验证的是「确认发生后下一次唤醒能从 `confirmed.json` 识别新版本并写规则块」；用户是否记得回到对话说「继续」、浏览器导出的调整请求与对话意见并存时的体验，需要真实使用观察。
- **规则文件约束的遵循率**：按 ADR-0004 只是指令，没有硬保障；本次没有跑后续实现会话。
- **`$ARGUMENTS` 中 `@spec.md` 在 Claude Code 与 Codex 里的展开方式**：SKILL.md 按「读取被引用文件的整份内容」处理，未在两个 harness 里实测。
- **`setsid nohup` 启动的工作台在各 harness 的沙箱中能否存活到下一回合**：本机（WSL2，本会话）可以；其他沙箱策略未测。
