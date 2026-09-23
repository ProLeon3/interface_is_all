# 模块与接口设计工作台

> 2026-09-21 已确认并实施新方向：由外部 coding agent 主导流程并调用本程序，所有 LLM 推理由 agent 承担；程序保留源码事实采集、确定性检查、图形编辑、提案审阅、存储与用户确认。决定见 [ADR-0002](docs/adr/0002-external-coding-agent-orchestration.md)。内置模型客户端与外部审查执行器已移除，设计生成与源码分析改为导出请求文件、导入 agent 提案。2026-09-22 已确定 skill 接入形态：回合制、程序零改动、确认只在浏览器、遵循约束只写目标项目规则文件，见 [ADR-0003](docs/adr/0003-turn-based-skill-integration.md)、[ADR-0004](docs/adr/0004-design-compliance-via-project-rules-file.md) 与 [coding agent 接入实现依据](coding%20agent%20接入实现依据.md)；同日已实现 `interface_design` skill，安装与使用见[在 coding agent 中使用 interface_design skill](#在-coding-agent-中使用-interface_design-skill)，实现与验收记录见 [docs/agent-skill.md](docs/agent-skill.md)。2026-09-22 又决定以 `npx skills add ProLeon3/interface_is_all` 分发 skill、以 `go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest` 安装程序，见 [ADR-0005](docs/adr/0005-distribute-skill-via-skills-cli.md)、[skill 分发安装实现依据](skill%20分发安装实现依据.md) 与 [docs/skill-distribution.md](docs/skill-distribution.md)。

本项目提供本地浏览器工作台、Go 核心库和命令行工具：保存模块职责、接口能力及禁止依赖规则，由用户审阅确认后作为检查基准，并为外部 AI 工具提供能力审查协议。当前存储与检查边界见[设计存储与验证](设计存储与验证.md)。

已按[独立图形工作台实现依据](独立图形工作台实现依据.md)实现需求输入、接口协作图、围绕所选对象提出修改、图形变更审阅、草稿接受、版本确认和手动交接；设计方案由外部 coding agent 依据导出的请求生成，再导入工作台审阅。沿用项目隔离、版本冲突检测、未保存编辑保护、独立布局与原有检查能力。技术选择与验证边界见[独立工作台实现与验收](docs/standalone-workbench.md)。

已按[已有项目初次分析实现依据](已有项目初次分析实现依据.md)实现已有 Go 项目从代码开始的首次接入：导出源码分析请求、导入 agent 生成的附带依据的现状提案、审阅、编辑草稿、明确确认，再使用原有检查。实现选择和验证边界见[初次分析实现与验收](docs/initial-analysis.md)。

已有确认基准的项目也可以重新分析最新源码，与旧基准和已保存草稿对照，接受后再明确确认新版本。使用方式与存储保护见[已有基准项目重新分析](docs/reanalysis.md)。

coding agent 通过 `interface_design` skill 接入：在目标项目里输入 `/interface_design @spec.md`，skill 按回合导出请求、生成提案并导入工作台，用户在浏览器审阅确认，确认后 skill 把遵循约束写入项目规则文件；见[在 coding agent 中使用 interface_design skill](#在-coding-agent-中使用-interface_design-skill)。底层仍是可手动使用的文件交换协议，见[与 coding agent 交换请求和提案](#与-coding-agent-交换请求和提案)。接入范围、阶段判断规则与验收依据见 [coding agent 接入实现依据](coding%20agent%20接入实现依据.md)，实现与验收记录见 [docs/agent-skill.md](docs/agent-skill.md)；按 ADR-0004，自动检查与自动修复不再是产品承诺，改为写入目标项目规则文件的指令。依赖检查通过不表示接口协作、业务功能或模型语义已经通过验收。

## 启动浏览器工作台

需要 Go 1.22 或更高版本。页面随二进制分发，运行时无需 Node、前端构建或 CDN。

```sh
go build -buildvcs=false -o /tmp/archdesign ./cmd/archdesign

# 启动后在页面选择项目；新项目可以先设计，运行代码检查时需要 go.mod。
/tmp/archdesign serve

# 也可以用示例项目体验，按 Ctrl+C 停止服务。
/tmp/archdesign serve -project examples/shop -addr 127.0.0.1:8090
```

在浏览器打开终端显示的地址，默认是 `http://127.0.0.1:8090`。点击「当前项目」旁的「选择项目」，输入路径或逐级浏览目录即可切换，无需重启服务。`-project` 指定初始项目，省略时使用启动目录。每个标签页独立选择项目，刷新后恢复该标签页的选择；切换前若有未保存编辑，可以取消后保存、下载草稿备份，或明确同意丢弃。`-timeout` 约束单次扫描、请求导出或提案导入，`-tags` 指定构建标签，不会让工作台按检查超时退出。

主导航分为「架构设计」和「代码检查」两个工作区：前者起草并确认架构约束，后者依据已确认版本检查代码。新项目先确认设计，交给外部编程工具实现后再检查；已有代码可先分析、审阅确认，再直接检查。拥有确认基准后可反复往返两个工作区，未确认的新草稿不会替换检查基准。「使用指南」是辅助说明，无需把它当作必须完成的接入步骤。

1. 空项目先选择起草方式。已有 Go 代码默认进入代码分析；从需求开始时选择「从需求设计」，输入需求并点击「导出设计请求」，把下载的 JSON 交给 coding agent。agent 按请求内的 `response_schema` 返回结果后，在「设计工具」中「导入 agent 提案」。提案进入待审阅状态，不会自动保存草稿或确认设计。
2. 已有设计默认先显示协作图。点选模块、具体接口或协作线，展开「调整设计」，在「本次修改意图」描述调整并导出请求；所选对象随请求一起导出。导入 agent 提案后，图上显示新增、修改、迁移和删除；可以查看修改前、导出「提案调整请求」继续调整，或在「提案工具」中放弃提案。已有未保存编辑时先保存草稿。
3. 满意就点击「接受并审阅确认」：提案先写入草稿，随即打开审阅窗口，查看包含协作关系的完整设计，填写确认人并勾选审阅声明。想先手改再定稿，就点击「接受并保存草稿」，编辑后再单独「审阅并确认」。接受提案不等于确认整个版本。
4. 确认后点击「交接已确认设计」，下载含确认快照和实现说明的 JSON，手动交给 coding agent。处理其他任务时可从「设计工具」找到交接入口；其中的「导出当前设计」按当前视图导出草稿、提案或确认设计，文件名明确区分。
5. 实线表达模块使用具体接口；「显示禁止依赖」单独叠加包引用限制。未画出的关系不自动违规。拖动手柄或使用方向键只改变独立布局。
6. 在画布旁「新建模块」，通过对象详情手动修改，或从「设计工具」导入 JSON。在「代码检查」运行依赖检查，查看规则、源码位置与构建范围；展开「能力审查」可导出请求并导入外部响应，语义结论保持待确认。

页面按当前阶段突出保存、审阅确认或交接操作，详细修改依据和浏览器复审见[工作台操作层级调整](docs/ui-simplification.md)。

### 已有 Go 项目初次接入

选择项目根目录含 `go.mod`、尚无确认设计的项目，在「分析已有代码」方式下点击「导出源码分析请求」。已有草稿时先展开「调整设计」，再选择代码分析方式。无需先手写设计；请求文件包含本次读取的 Go 源码、公开声明候选、包依赖、构建清单和响应 Schema。把请求交给 coding agent，按[源码分析指令](skills/interface_design/references/source-analysis.md)生成响应，再在「设计工具」中「导入 agent 提案」。打开目录本身不扫描，也不导出。

在图上核对模块、能力和协作；详情中的「初次分析依据」可以查看分析时的原始源码。图下分别列出读取范围、排除文件、问题、建议与不确定项。包导入只证明依赖，未观察到某条依赖不会自动变成禁止规则。间接调用或动态关系不能静态关联时标为不确定。

点击「接受并保存草稿」后，可以沿用表单编辑模块、接口归属和协作。来源证据会保留，后续编辑不会改写原分析。最后单独「审阅并确认」，建立第一个基准；不需要手改时可直接点击「接受并审阅确认」一步进入审阅窗口。然后到「代码检查」运行检查。待审阅分析保存在项目中，刷新或另一窗口重新打开都可恢复。

首次分析沿用单 Go module 的扫描范围。源码加载不完整、没有可读 Go 文件或完整上下文超过 **1 MiB** 时停止导出，页面展示范围和原因；不静默截断源码或保存部分分析草稿。agent 所用模型的上下文上限可能更小，agent 未返回有效结果时原草稿仍保留。

导入、接受及确认本次分析时会重新核对源码指纹和构建范围，草稿与基准同时受版本检查保护。源码过期时可明确撤回并重新分析；已接受过的来源撤回后仍保留保护标记，必须接受新提案后才能确认。

### 已有基准项目重新分析

在「设计草稿」视图展开「调整设计」，选择「重新分析代码」方式，再点击「导出重新分析请求」，把请求交给 coding agent 后导入提案。原草稿和旧基准继续保留，导入的提案分别展示相对已保存草稿、相对已确认基准的变更。现状分析不自动保留旧禁止规则，差异会明确列出；接受后可编辑草稿恢复需要的约束。

「接受并保存草稿」只更新草稿；最后「审阅并确认」才发布新基准，「接受并审阅确认」把这两步连起来做。确认前的检查和设计交接仍使用旧基准，历史快照和分析证据保留。即使新分析没有改变设计文字，也可以确认新的源码依据。刷新或重开工作台能恢复当前分析。

### 与 coding agent 交换请求和提案

程序不内置模型客户端，也不读取模型配置；推理、提示词和模型选择由外部 coding agent 自行管理。工作台和命令行只负责导出请求、校验响应并保存为待审阅提案。agent 可参考 [skills/interface_design/references](skills/interface_design/references/) 中的设计、源码分析与能力审查指令；三份指令统一为「输出契约、文字怎么写（含字数上限）、规则、边界」四段，说明文字会原样显示在模块节点、详情面板和确认窗口里，所以要求精简、只写需求或代码支持的内容、不复述行号与范围套话。

| `kind` | 用途 | 请求内容 |
| --- | --- | --- |
| `design` | 从需求生成设计，或围绕所选对象调整草稿及待审阅提案 | 需求、修改意图、所选对象、当前设计；继续调整时以父提案结果为上下文 |
| `initial_analysis` | 尚无确认基准的 Go 项目从代码还原现状 | 上述内容加实际读取范围内的源码、公开声明候选、包依赖与构建清单 |
| `reanalysis` | 已有基准的项目对照最新源码重新分析 | 同上；导入的提案额外保存生成时的完整基准 |

请求由程序采集并保存到 `.architecture/requests/<request_id>.json`。`request_id` 是内容指纹，绑定项目路径、草稿指纹、基准版本和源码指纹；请求文件不含会话令牌。响应是一个 JSON 对象，包含 `request_id`、`summary`（中文变更说明）和 `design`（符合设计 Schema 的完整设计），源码分析还必须包含 `analysis`；完整结构以请求内的 `response_schema` 为准。

```sh
# 导出请求；-file 可提供含 requirement、instruction、selection、parent_id 的意图 JSON。
/tmp/archdesign design-request -project /你的项目目录 -kind initial_analysis > request.json

# agent 读取 request.json 并写出 response.json；导入时校验并保存为待审阅提案。
/tmp/archdesign proposal-import -project /你的项目目录 -file response.json
/tmp/archdesign proposal-show -project /你的项目目录
/tmp/archdesign proposal-accept -project /你的项目目录 -id '<提案 ID>'
/tmp/archdesign proposal-discard -project /你的项目目录 -id '<提案 ID>'
```

导入时程序只加载本项目签发的请求，重新校验响应结构、设计关系、模块目录、分析依据位置和源码指纹。草稿、基准或源码在导出后发生变化时拒绝导入，跨项目复制请求文件同样被拒绝；同一请求只产生一个待审阅提案，并行响应不会覆盖它。导入和接受都不确认设计，工作台会同步展示由命令行导入的提案。

```sh
# 构建并启动；服务只监听本机，不读取任何模型配置。
bash scripts/start-workbench.sh -project /你的项目目录 -timeout 5m
```

## 在 coding agent 中使用 interface_design skill

`skills/interface_design/` 是一份回合制、可重入的 skill（Claude Code 与 Codex 共用的 SKILL.md 格式），把上一节的请求导出、提案导入与规则文件写入串成 `/interface_design @spec.md` 一条命令的流程。决策见 [ADR-0003](docs/adr/0003-turn-based-skill-integration.md)、[ADR-0004](docs/adr/0004-design-compliance-via-project-rules-file.md) 与 [ADR-0005](docs/adr/0005-distribute-skill-via-skills-cli.md)，实现选择、逐条阶段规则的验证记录与 UNVERIFIED 项见 [docs/agent-skill.md](docs/agent-skill.md)；分发与安装方式的实现选择、验证记录与 UNVERIFIED 项见 [docs/skill-distribution.md](docs/skill-distribution.md)。

安装只需两条命令。需要 Go 1.22 或更高版本、python3、bash，以及 Node.js 自带的 `npx`；脚本依赖 bash 的 `/dev/tcp`，不支持 Windows 原生环境。

```sh
# 1. 安装 archdesign 到 $(go env GOPATH)/bin，并确保该目录在 PATH 上；也可以用环境变量 ARCHDESIGN_BIN 指向已构建的二进制。
go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest

# 2. 在目标项目根目录安装 skill（项目范围）：复制到 ./.agents/skills/interface_design/，并建软链 ./.claude/skills/interface_design。
npx skills add ProLeon3/interface_is_all

# 或全局安装（所有项目可用）：位置改为 ~/.agents/skills/ 与 ~/.claude/skills/。
npx skills add ProLeon3/interface_is_all -g
```

`npx skills add` 只复制 `skills/interface_design/` 这一个目录（`SKILL.md`、`scripts/`、`references/`、`examples/`），不编译程序，也没有安装钩子；安装位置会写 `skills-lock.json` 记录来源与内容哈希，之后用 `npx skills update` 更新。Claude Code 通过 `.claude/skills/interface_design` 软链发现该 skill；Codex 直接读 `.agents/skills`，是否会扫描 `~/.agents/skills` 未查证，见 [docs/agent-skill.md](docs/agent-skill.md) 的 UNVERIFIED 列表。源仓库为 <https://github.com/ProLeon3/interface_is_all>，`v0.1.0` 起可安装；本地开发时也可以把本仓库的绝对路径作为 `npx skills add` 的来源。

在本仓库开发 skill 时可以不经 CLI，直接把源目录软链到用户目录，改动即时生效：

```sh
go install ./cmd/archdesign
ln -s "$(pwd)/skills/interface_design" ~/.agents/skills/interface_design
ln -s ../../.agents/skills/interface_design ~/.claude/skills/interface_design
```

使用：在目标项目目录打开 coding agent，输入 `/interface_design @spec.md`（`spec.md` 的内容作为需求；没有文件时 skill 会问一次）。skill 每次被唤醒都做同样的事：

1. 找到 `archdesign`，检查 `127.0.0.1:8090` 是否已有工作台；没有则以当前项目为 `-project` 在后台启动 `archdesign serve`，把地址告诉用户。只监听本机，不传任何模型配置。
2. 运行 `scripts/state.py` 从 `.architecture/` 重建阶段，不依赖对话记忆。
3. 按阶段判断规则只做一件事：有 `go.mod` 且无确认基准，先导出源码分析请求并按[源码分析指令](skills/interface_design/references/source-analysis.md)生成现状提案；有基准或不是 Go 项目，以需求导出设计请求并按[设计指令](skills/interface_design/references/design.md)生成提案；浏览器导出而未应答的调整请求先应答；已有待审阅提案时按用户的修改意见以它为父提案继续调整，没有意见就提醒去浏览器。导出失败与导入被拒绝都原样转述程序输出并停止。
4. 回合结束时告诉用户：工作台地址、提案 ID、下一步在浏览器做什么、做完回来说「继续」。

接受提案与确认版本只在浏览器完成，skill 不运行 `confirm` 与 `proposal-accept`；`proposal-discard` 只在用户明确要求放弃当前提案时运行。用户确认后，下一次唤醒发现 `confirmed.json` 的 `revision` 比规则文件标记块记录的新，skill 把设计遵循约束写入目标项目的规则文件：已有 `CLAUDE.md` 或 `AGENTS.md` 就写已有的（两个都有则都写），都没有就新建 `AGENTS.md`；使用 `<!-- interface_design:begin revision=… -->` 与 `<!-- interface_design:end -->` 之间的块幂等替换，块外内容不动，不提交 git。这段约束是给后续会话的指令，程序只能硬检查禁止的直接包依赖，不能据此声称设计被强制遵守。

辅助脚本都带 `--help`：`scripts/state.py` 输出阶段与下一步；`scripts/export-request.py` 组装意图并导出请求；`scripts/lint-response.py` 在导入前对照请求文件自检响应（字数上限、依据条目、位置有效性、无关对象是否被改写等，程序导入时不检查这些）；`scripts/write-rules.py` 写规则文件标记块；`scripts/start-workbench.sh` 启动或复用工作台。

## 命令行快速运行

需要 Go 1.22 或更高版本，以及目标 Go 项目所需的工具链与依赖。首次构建需要下载锁定在 `go.mod`、`go.sum` 中的 JSON Schema 依赖。

```sh
# 在本仓库根目录构建。
go build -buildvcs=false -o /tmp/archdesign ./cmd/archdesign

# 校验示例设计并保存为草稿。
/tmp/archdesign validate -project examples/shop -file examples/design.json
/tmp/archdesign save-draft -project examples/shop -file examples/design.json

# 展示需要用户审阅的完整草稿和 draft_hash。
/tmp/archdesign show -project examples/shop -draft
```

审阅后，将输出中的 `draft_hash` 填入确认命令：

```sh
# 首次确认使用 none。再次确认必须传入 show 返回的当前 revision。
/tmp/archdesign confirm -project examples/shop \
  -draft-hash '<已审阅的草稿指纹>' -expected none -by '确认人'

/tmp/archdesign show -project examples/shop
/tmp/archdesign check -project examples/shop
```

示例把退款入口放在 `payment/api` 子包，订单通过它申请退款；设计只禁止支付到订单的依赖。示例退款代码用于演示检查流程，没有连接支付系统，不能作为业务验收样例。

除 `serve` 输出工作台地址外，命令将 JSON 写入标准输出，将错误和已保存的观察文件路径写入标准错误。通用选项包括 `-project`、`-timeout`，扫描还支持 `-tags`。运行 `/tmp/archdesign help` 查看所有命令。输入文件路径相对于调用命令时的工作目录，设计中的模块目录相对于 `-project`。

| 退出码 | 含义 |
| --- | --- |
| `0` | 操作成功；检查命令仅表示已执行的确定性检查未发现违规 |
| `1` | 发现明确禁止的直接包依赖；仍输出并保存完整结果 |
| `2` | 设计无效、版本冲突、输入或执行失败，或扫描不完整 |

`check` 的 `semantic_status` 为 `not_run`，不能据此认为接口能力已通过审查。`review-request` 即使生成请求成功，也会用退出码 `1` 报告同时发现的确定性违规。

## 设计文件与校验

完整示例见 [examples/design.json](examples/design.json)，结构定义见 [design/design.schema.json](design/design.schema.json)。设计文件必填以下字段：

| 字段 | 含义 |
| --- | --- |
| `schema_version` | 当前为整数 `1` |
| `modules` | 模块数组；每项含 `id`、`root`、`responsibility` |
| `interfaces` | 能力数组；每项含 `id`、`module_id`、`name`、`description` |
| `forbidden_dependencies` | 禁止方向数组；每项含 `id`、`from`、`to`、`reason` |

上述三个数组可以为空，必须显式提供。可选 `collaborations` 数组保存 `{id, from, interface_id, purpose}`，分别表示协作 ID、调用模块、目标接口及目的。接口归属决定目标模块，接口迁移后协作自动指向新模块。空协作省略序列化，以保持旧设计和历史快照指纹不变；旧版程序无法读取新增的非空字段，应升级后使用。接口可选 `semantics` 对象，用 `inputs`、`outputs`、`errors` 补充自然语言含义。用户无需逐字段定义 Go 参数或结构体。

所有对象的 ID 共用一个命名空间，使用小写字母开头的字母、数字、点、下划线和短横线，最长 128 字符。ID 由设计编辑方生成，修改名称或说明时保留原 ID。依赖规则也有 ID，便于报告准确引用设计依据。

结构校验由实际 JSON Schema 校验器执行，之后由程序检查：

- 重复 ID、未知模块或接口引用、重复禁止方向及模块自依赖规则。协作还拒绝同模块接口使用和重复的“调用模块＋接口”关系。
- 模块根目录重复或存在包含关系；`payment` 与 `payment-old` 可以并存。
- 绝对路径、非规范路径、越界路径及反斜杠分隔。单一模块可使用 `.` 独占整个项目。
- 已有模块目录及其父目录中的符号链接、根路径为普通文件等归属歧义。尚未创建的模块目录可以保存。

JSON 解析还拒绝重复键、未知字段和尾随内容。关系错误提供稳定错误码与 JSON Pointer，例如 `/interfaces/0/module_id`，工作台保存和导入失败时显示这些字段位置。

## 存储与确认

所有数据位于目标项目的 `.architecture` 目录：

```text
.architecture/
  draft.json                     # 设计草稿，符合设计 Schema
  confirmed.json                 # 当前已确认快照，包含 design 和确认记录
  versions/<revision>.json       # 各次确认的完整历史快照
  layout.json                    # 独立画布节点坐标
  requests/<request_id>.json      # 导出给 coding agent 的设计或源码分析请求，导入响应时据此核对
  proposals/<id>.json             # 不可变 agent 提案，源码分析保存源码及依据，重新分析另存原基准
  initial-analysis.json           # 首次或后续源码分析的待审阅、接受中、已接受或撤回状态
  observations/check-*.json       # 代码事实与确定性检查报告
  observations/review-request-*.json
  observations/review-*.json      # 待确认的语义审查结果
  write.lock                     # 写入草稿、确认版本或发布观察结果期间存在
```

草稿和已确认设计分开保存。`save-draft` 不改变检查基准；`confirm` 同时比较已审阅的 `draft_hash` 和 `-expected` 当前版本，任一发生变化都拒绝确认。设计和来源证据均未变化时，再次确认不会重复创建版本；新的源码分析证据仍可单独确认。

`confirmed.json` 包含 `revision`、`parent_revision`、`design_hash`、`confirmed_at`、`confirmed_by`、`design`。确认操作先保存历史，再原子替换当前版本。读取时验证内容指纹及历史一致性；检查结果不能成为设计约束。确认人字段是审计记录，浏览器通过独立审阅窗口让用户确认；命令行调用方仍负责获得用户确认。本版没有浏览器账号或身份认证层。

多个本地进程通过排他锁协调草稿写入、确认和观察结果发布。报告保存时在同一把锁内复核设计版本，拒绝已过期的结果。异常退出留下锁时，先确认记录的进程已结束，再由维护者移除 `write.lock`；程序不会自动抢占不明状态的锁。JSON 文件先写入同目录临时文件，同步后替换，避免读取到半个 JSON。

布局示例：

```json
{"nodes":{"payment":{"x":120,"y":80},"order":{"x":420,"y":80}}}
```

使用 `layout-save -file <布局文件>`、`layout-show` 保存和读取布局。布局更新不创建设计版本。若提交到版本控制，建议保留草稿、确认快照和历史，按需忽略观察文件与本地布局；本仓库的演示目录默认忽略运行生成的 `.architecture`。

## 确定性检查与覆盖范围

扫描通过 `go list -e -deps -json` 获取当前构建选择、包目录和导入关系，再用 Go 标准库 AST 定位导入声明与公开候选。包归属依据实际目录，不根据 import 字符串前缀判断。该机制依据 [Go 命令的包列表说明](https://pkg.go.dev/cmd/go#hdr-List_packages_or_modules)。

检查只将“某模块中的包直接导入另一模块中的包，且该方向已被明确禁止”判为违规。报告包含原始禁止规则、原因、两端包和导入行列号。未禁止方向、未维护允许列表、公开入口位于子包等情况都不会单独产生违规。

当前范围为项目根目录有 `go.mod` 的单个 Go module：

- 记录实际 `GOOS`、`GOARCH`、CGO 状态、工具链版本和显式 `-tags`；不扫描 `_test.go` 与其他构建条件下的文件，报告列出包内排除文件。
- 固定 `GOWORK=off`，清空隐式 `GOFLAGS`；依赖按只读模块模式加载，已有 vendor 时使用 vendor。需要依赖可用；加载失败会记录诊断。
- 沿用 Go 包遍历对隐藏目录、下划线目录、`testdata`、vendor 的排除。发现嵌套 Go module 会标记 `incomplete`，需要单独检查。暂不展开 SWIG 生成入口。
- 未纳入设计的包和当前没有包的模块分别列入 `unassigned_packages`、`empty_modules`，不自动成为违规。
- 提取导出函数、方法、类型、变量和常量，并保留扫描到的项目 Go 源文件，包括私有辅助逻辑。导出声明是候选证据；类型别名、嵌入方法、未导出类型的可达性等不做完整类型或调用分析。

包加载错误、语法错误、读文件失败等使确定性状态成为 `incomplete`；仍保留已经定位的违规，并拒绝生成完整语义审查请求。扫描不执行目标程序、测试或代码生成，不验证类型检查或业务功能正确性，也不推断接口注入、回调的运行时目标。

## 接口能力审查

能力审查可通过文件交换接入现有 AI 工具：

```sh
/tmp/archdesign review-request -project examples/shop > /tmp/review-request.json

# 外部 AI 工具读取请求，在同一次审查中按 response_schema 返回响应文件。
/tmp/archdesign review-import -project examples/shop \
  -request /tmp/review-request.json -file /tmp/review-response.json
```

coding agent 可以直接调用这两个命令完成审查闭环，审查指令见 [skills/interface_design/references/capability-review.md](skills/interface_design/references/capability-review.md)。程序不启动任何外部审查程序，也不读取模型凭据。

请求包含已确认设计、源码快照、构建范围、模块及依赖清单、公开候选、中文审查指令和响应 Schema。`request_id` 绑定这些内容；`go.mod`、已有的 `go.sum` 和 `vendor/modules.txt` 变化也会使旧审查过期。指令要求逐接口关联实现，检查未设计的新增能力与职责偏离，明确不得仅凭名称和参数变化判定违规。实现参考使用的 [JSON Schema 校验库](https://pkg.go.dev/github.com/santhosh-tekuri/jsonschema/v5@v5.3.1) 同样校验模型响应结构。

响应包含 `schema_version`、`request_id`、`interfaces`、`concerns`。每个接口恰好返回一条记录，包含：

| 字段 | 含义 |
| --- | --- |
| `interface_id` | 对应的设计接口 ID |
| `entry_ids` | 同模块的一个或多个实现候选 ID；尚未找到实现时为空 |
| `assessment` | `aligned`、`not_found`、`potential_deviation` 或 `uncertain` |
| `reason` | 对照设计与代码的判断理由 |
| `evidence` | `{file, line, end_line}` 数组，引用本次源码中的有效位置 |

`concerns` 使用 `additional_capability` 或 `responsibility_drift`，并提供 `module_id`、`entry_ids`、`reason`、`evidence`。完整结构以请求内 `response_schema` 和 [check/review.schema.json](check/review.schema.json) 为准。

导入时验证接口覆盖、重复或未知引用、跨模块关联、证据行号和必要的实现关联，再按请求的构建条件重新扫描。代码、构建范围或已确认设计变化时拒绝旧响应。缺失能力的报告附上扫描过的模块文件列表；没有代码时保留空搜索范围和解释。

所有语义关联和问题都标记 `pending_confirmation`，原始设计依据由程序从确认快照附加。`aligned` 仍是模型判断，`not_found` 表示“在本次范围尚未找到”，不是确定性能力缺失。`review-import` 输出并保存的是语义报告；确定性检查报告的 `semantic_status` 始终保留 `not_run`，两者分别展示。

当前校验能够保证结构、引用与证据位置可核对，不能证明模型理由正确或没有漏报。仓库测试使用固定响应验证接入流程，没有使用真实模型验证判断准确率。

## 实现位置与验证

| 目录 | 职责 |
| --- | --- |
| `design` | 设计模型、Schema、关系和模块目录校验 |
| `source` | 源码、公开声明、包依赖与构建范围的事实采集及指纹 |
| `store` | 草稿、确认版本、历史、AI 提案、布局与观察文件 |
| `designer` | 外部提案的请求与响应协议、响应 Schema 及分析依据校验 |
| `check` | 代码事实提取、确定性检查、能力审查协议与证据验证 |
| `cmd/archdesign` | 命令行流程、JSON 输出及退出码 |
| `workbench` | 本地 HTTP 接口、嵌入式浏览器页面、同源和会话边界 |
| `scripts/workbench-journey.js` | 使用真实浏览器验证编辑、确认与检查旅程 |
| `scripts/fixed-agent.js` | 浏览器回归中在页面内扮演 coding agent 的固定响应替身，不代表真实模型效果 |
| `skills/interface_design` | coding agent 的回合制 skill：SKILL.md、状态/导出/响应自检/规则写入/工作台启动脚本、`references/` 下的三份 agent 指令与按指令写成并导入验证过的响应样例；验收见 [docs/agent-skill.md](docs/agent-skill.md)，分发见 [docs/skill-distribution.md](docs/skill-distribution.md) |

```sh
go test ./...
go test -race ./...
go vet ./...
# 以下需要 playwright-cli 和可用的 Chromium；固定响应替身扮演 coding agent。
bash scripts/verify-workbench.sh
bash scripts/verify-ai-workbench.sh
bash scripts/verify-initial-analysis.sh
```

测试覆盖草稿与基准隔离、陈旧版本拒绝、并发确认、历史完整性、设计结构和关系校验、符号链接边界、子包入口、允许与禁止的直接依赖、改名与参数重组、构建标签、扫描失败、缺失能力和新增能力响应、无效证据、过期审查、请求与项目绑定、跨项目导入拒绝、导出后源码或设计变化的拒绝，以及命令行完整流程。它们验证本地程序行为，不代表真实模型效果或外部工具自动触发已经验证。浏览器流程的契约与执行命令见[核心用户旅程](docs/core-user-journeys.md)。

在限制缓存写入的环境可指定可写缓存：`GOCACHE=/tmp/interface-is-all-gocache GOMODCACHE=/tmp/interface-is-all-gomodcache go test ./...`。初次下载完成后可加 `GOPROXY=off` 运行本地回归。
