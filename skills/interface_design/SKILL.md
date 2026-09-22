---
name: interface_design
description: 结合目标项目的已有代码与需求描述做模块与接口的架构设计，导出请求、生成设计提案并导入本地工作台（archdesign），让用户在浏览器审阅、修改、确认；确认后把设计遵循约束写入项目规则文件。用户输入 /interface_design @spec.md、要求设计或调整模块划分与接口、要为新需求做架构设计、要先分析已有 Go 代码的现状基准、或在浏览器操作后回到对话说「继续」时使用。回合制、可重入：每次唤醒都从 .architecture/ 重建阶段。
argument-hint: "[@spec.md 或需求文字；浏览器操作完成后可只说「继续」]"
---

# interface_design

把 `/interface_design @spec.md` 落到本地程序 `archdesign` 上：你负责读代码、按需求生成设计提案；程序负责导出请求、校验并保存提案、在浏览器展示给用户审阅、由用户接受与确认；确认后你把遵循约束写入项目规则文件。决策依据见 `<skill-dir>/../../docs/adr/0003-turn-based-skill-integration.md` 与 `0004-design-compliance-via-project-rules-file.md`。

`<skill-dir>` 指本 SKILL.md 所在目录（安装后通常是 `~/.claude/skills/interface_design` 或 `~/.agents/skills/interface_design`，软链指向本仓库 `skills/interface_design/`）。脚本在 `<skill-dir>/scripts/`，指令文档在 `<skill-dir>/../../docs/agent-prompts/`（脚本输出里也给出绝对路径）。

## 硬性边界（先读，全程有效）

1. **回合制、可重入。** 每次被唤醒都先运行状态脚本，从目标项目 `.architecture/` 重建阶段；不依赖对话记忆，不长轮询、不阻塞等待用户在浏览器的动作。每回合只做阶段表命中的那一件事，做完就把「用户下一步在浏览器做什么、做完回来说什么」讲清楚，然后结束回合。
2. **永远不运行 `archdesign confirm` 和 `archdesign proposal-accept`。** 接受提案与确认版本是用户在浏览器的动作。`proposal-discard` 只在用户明确说要放弃/重来当前提案时运行。
3. **有 `go.mod` 且没有确认基准的项目必须先走 `initial_analysis`**，现状基准在浏览器确认前不得导出 `design` 请求。
4. **导出失败照实转述并停止。** 源码分析请求导出失败（含超过 1 MiB 上限、加载不完整）时，原样转述程序返回的原因与范围，不截断源码、不分批、不改用其他请求类型。导入被拒绝时原样转述错误，不修改请求文件、不伪造或改写 `request_id`、不绕过校验。
5. **生成的响应必须遵守请求文件里的 `response_schema` 和对应指令文档**（`docs/agent-prompts/design.md` 或 `source-analysis.md`）。
6. **规则文件只写标记块**，块外内容一律不动；不提交 git。
7. **不得声称设计已被强制遵守。** 程序只能硬检查被禁止的直接包依赖；接口能力与职责偏离依赖 agent 主动做能力审查和用户核对。
8. 工作台只监听本机；你不读取、不传递任何模型配置或凭据给程序。

## 第 0 步：准备

1. **目标项目**：默认是当前工作目录（用户打开的项目根目录）；用户明确指定其他路径时用该路径。下文记为 `<project>`。
2. **需求输入**：`$ARGUMENTS` 里的 `@文件` 引用（例如 `@spec.md`）——读取整份文件内容作为 `requirement`；纯文字参数直接作为需求；用户只说「继续」「看看状态」等则视为**没有新需求**。参数为空且状态脚本给出的阶段需要需求时，向用户问一次「请给我需求文件或需求描述」，然后结束回合。
3. **修改意见**：用户本回合对当前提案说了具体改法（拆模块、改接口语义、改归属等），视为**有修改意见**。
4. **工作台**：运行
   ```sh
   bash <skill-dir>/scripts/start-workbench.sh --project <project>
   ```
   已在监听则直接得到地址；否则脚本以 `<project>` 为 `-project` 在后台启动 `archdesign serve`（只监听 `127.0.0.1:8090`）。失败（找不到 `archdesign`、端口被其他服务占用、启动超时）时把脚本输出原样告诉用户并停止本回合。地址在每回合结束时都要再告诉用户一次。若脚本提示工作台已在监听但关联的是别的项目，提醒用户在页面「选择项目」输入 `<project>`。
5. **读取状态**（本回合的唯一阶段依据）：
   ```sh
   python3 <skill-dir>/scripts/state.py --project <project> [--has-requirement] [--has-feedback]
   ```
   有新需求加 `--has-requirement`，有修改意见加 `--has-feedback`。输出 JSON 的 `phase`、`next_action`、`commands` 就是本回合要做的事；`warnings` 里的内容要转述给用户。

## 第 1 步：按 `phase` 行动（一回合只做一件事）

`phase` 的顺序就是阶段判断规则表的顺序，命中第一条即执行。

### `archdesign_unavailable`

转述 `archdesign.error` 与 `archdesign.install_hint`（在本仓库运行 `go install ./cmd/archdesign`，或设置 `ARCHDESIGN_BIN`），停止。

### `answer_requests`（存在未应答的请求）

`unanswered_requests` 是 `.architecture/requests/` 里还没有任何提案引用其 `request_id` 的请求，通常来自用户在浏览器「调整设计」导出的请求，也可能是上一回合导出后没能成功导入的请求。按顺序逐个处理：读取 `file`，按 `has_source`（true 用 `source-analysis.md`，false 用 `design.md`）生成响应并导入（见「生成响应并导入」）。`stale_requests` 是版本已变、必然被拒绝的旧请求，不要应答，只在回复里提一句它们的 `blocked_reasons`。

### `adjust_proposal`（存在待审阅提案，且用户本回合给了修改意见）

以 `pending_proposal.id` 为父提案导出调整请求，用户的意见作为 `--instruction`；用户点名了模块/接口/协作时加 `--selection-kind`/`--selection-id`：

```sh
python3 <skill-dir>/scripts/export-request.py --project <project> --kind design \
  --parent-id <pending_proposal.id> --instruction '<用户的修改意见>'
```

继续调整永远用 `--kind design`；父提案含源码时请求会自动携带同一份源码，响应仍需 `analysis`（脚本输出的 `has_source` 与 `prompt_doc` 会告诉你）。然后生成响应并导入。

### `await_review`（存在待审阅提案，用户没有新意见）

不要重复生成。告诉用户：在工作台看提案 `pending_proposal.id`（摘要在 `summary`），可以「接受并保存草稿」再「审阅并确认」，或选中对象填写意图导出调整请求，或直接在对话里说要怎么改；做完回来说「继续」。

### `await_baseline_confirmation`（源码分析已接受但未确认）

提醒用户在浏览器「审阅并确认」现状基准（填写确认人、勾选审阅声明）。不导出设计请求；用户此时给的需求先记下并明确说「确认基准后再发起设计，请确认后回来把需求再说一遍或直接说继续」。

### `initial_analysis`（有 `go.mod`、无确认基准、无有效分析状态）

```sh
python3 <skill-dir>/scripts/export-request.py --project <project> --kind initial_analysis
```

- 输出 `ok:false`：把 `stderr`、`error` 和 `source_scope`（构建范围、诊断、包清单）原样转述给用户，停止本回合。不要尝试缩小范围、分批、或改为 `design` 请求。
- 输出 `ok:true`：按 `source-analysis.md` 生成含 `analysis` 的响应并导入。源码内容以请求文件 `request.source.facts.files` 为准（`source.files` 列出路径与行数）；磁盘文件与之一致时可直接读磁盘文件定位行号，但导入前不要改动源码。每个模块、接口、协作在 `analysis.evidence` 中恰好一项；协作要同时引用调用语句与目标声明；不能静态关联的标 `uncertain`；`forbidden_dependencies` 必须为空数组；不虚构问题。

回合结束语：请用户在工作台核对源码依据，「接受并保存草稿」，再「审阅并确认」建立现状基准，然后回来说「继续」（并附上需求，若还没给）。

### `design`（无基准且非 Go 项目，或已有基准，且用户给了需求）

```sh
python3 <skill-dir>/scripts/export-request.py --project <project> --kind design --requirement-file <spec.md>
```

需求是对话文字时用 `--requirement '<文字>'`。按 `design.md` 生成响应并导入：保留未受影响对象的稳定 ID，已有基准时提案会相对基准展示差异。若状态里 `rules.stale` 为 true，回复里加一句：当前确认版本还没写入规则文件，下一次不带新需求的唤醒会写入。

回合结束语：请用户看图；要改就在浏览器选中对象填意图导出调整请求、或直接在对话里说；满意就「接受并保存草稿」再「审阅并确认」；做完回来说「继续」。

### `write_rules`（`confirmed.json` 的 `revision` 比规则文件标记块记录的新）

```sh
python3 <skill-dir>/scripts/write-rules.py --project <project>
```

脚本按规则选文件（已有 `CLAUDE.md`/`AGENTS.md` 就写已有的，两个都有则都写，都没有则新建 `AGENTS.md`），用起止标记 `<!-- interface_design:begin revision=… -->` / `<!-- interface_design:end -->` 幂等替换。然后在对话中汇报：确认版本、写入或替换了哪些文件、块内五条约定的要点，并明确说明这是给后续会话的指令，不是硬保障；不提交 git。接着可以询问用户是否有新需求。

### `report_status`

用状态 JSON 汇报：是否有基准（`confirmed.revision`）、草稿、待审阅提案、规则文件是否已含当前版本的标记块，然后问用户需求或下一步。

## 生成响应并导入

1. 读请求文件（`request_file`）：`request.requirement`、`request.instruction`、`request.selection`、`request.design`（当前设计或父提案结果）、`request.source`（源码分析才有）、`response_schema`。
2. 读对应指令文档（`prompt_doc`），按它生成**一个 JSON 对象**：`request_id` 原样复制；`summary` 中文变更说明；`design` 是完整设计（不是差量）；含源码时还要 `analysis`。说明文字用中文，ID 用简短英文稳定标识，模块目录是不重叠的项目相对目录。`<skill-dir>/examples/` 里有三份曾成功导入的响应（源码分析、需求设计、继续调整）只示范格式；内容与 `request_id` 都不能照抄。
3. 把响应写到临时文件（不要写进 `<project>`），然后导入：
   ```sh
   archdesign proposal-import -project <project> -file <响应文件>
   ```
   （`ARCHDESIGN_BIN` 设置时用该路径。）成功输出提案 JSON，其中 `id` 是提案 ID。
4. 导入失败：把标准错误原样告诉用户。结构、引用、目录、依据位置类错误可以修正响应后重试一次（这是修改你自己的输出，不是修改请求）；版本变化、源码变化、跨项目、父提案冲突类错误不要重试，说明原因并停止本回合。
5. 回合结束：告诉用户工作台地址、提案 ID、一句话摘要、下一步在浏览器做什么、做完回来说什么。工作台每 3 秒轮询文件，导入后页面会自动出现提案。

## 放弃提案（仅用户明确要求）

用户明确说放弃/重来当前提案时：

```sh
archdesign proposal-discard -project <project> -id <pending_proposal.id>
```

然后重新运行状态脚本决定下一步。用户只是想改，就走 `adjust_proposal`，不要放弃。

## 重新分析

不自动触发。用户要求重新对照最新源码，或 `archdesign check` 报告显示大量 `unassigned_packages` 时，向用户建议；用户同意后用 `--kind reanalysis` 导出（只在已有基准时可用），后续与 `initial_analysis` 相同。

## 回复模板（每回合结束必须包含）

- 本回合做了什么：请求类型、提案 ID、一句话摘要，或未做事的原因（原样转述的程序输出）。
- 工作台地址：`http://127.0.0.1:8090`（以脚本输出为准），当前项目应为 `<project>`。
- 用户下一步在浏览器做什么：核对/接受并保存草稿/审阅并确认/选中对象导出调整请求。
- 做完回来说什么：「继续」，或直接说修改意见/新需求。
- 若有 `warnings`，逐条转述。
