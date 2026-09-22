# interface_design skill 分发与安装：实现与验证记录

实施日期：2026-09-22。依据为[skill 分发安装实现依据](../skill%20分发安装实现依据.md)与 [ADR-0005](adr/0005-distribute-skill-via-skills-cli.md)。程序（Go 逻辑、前端、Schema、交换协议、`.architecture/` 存储格式）零改动；本次只做 module 路径与 import 的机械替换、指令文档搬移、脚本的路径解析与提示文字、SKILL.md 头部与文字、文档。

`<owner>` 取本机 `gh auth status` 的登录账号 `ProLeon3`，仓库名 `interface_is_all` 不变。安装命令：

```sh
go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest
npx skills add ProLeon3/interface_is_all
```

本文记录本地验收与推送后的远端验证，都由下文的命令记录支撑；仍未验证的项见文末 UNVERIFIED 列表。源仓库：<https://github.com/ProLeon3/interface_is_all>（公开），首个标签 `v0.1.0`。

## 改动清单

| 路径 | 改动 |
| --- | --- |
| `go.mod` | `module interfaceisall` 改为 `module github.com/ProLeon3/interface_is_all` |
| 39 个 Go 文件（97 处 import） | `"interfaceisall/…"` 改为 `"github.com/ProLeon3/interface_is_all/…"`；gofmt 随之调整了 `check/review.go`、`design/validate.go`、`designer/designer.go` 三个文件的 import 顺序（新前缀含大写字母，排序位置变化） |
| `skills/interface_design/references/design.md`、`source-analysis.md`、`capability-review.md` | `git mv` 自 `docs/agent-prompts/`，内容不变 |
| `skills/interface_design/scripts/state.py` | `skill_paths()` 改为 `<skill-dir>/references/`，输出去掉 `repo_dir`；`resolve_archdesign()` 不再接收 `repo_dir`，`install_hint` 改为统一提示；指令文档不存在时写入 `warnings` |
| `skills/interface_design/scripts/export-request.py` | `skill_paths()` 改为 `<skill-dir>/references/`；找不到 archdesign 的 `stderr` 改为统一提示 |
| `skills/interface_design/scripts/write-rules.py` | `repo_dir()` 改为 `skill_dir()`；标记块里的能力审查指令路径改为 `<skill-dir>/references/capability-review.md` |
| `skills/interface_design/scripts/start-workbench.sh` | 去掉 `repo_dir` 推算；提示改为统一提示 |
| `skills/interface_design/SKILL.md` | frontmatter 增加 `compatibility` 与 `metadata`；ADR 引用改为文字说明加 GitHub 链接；`<skill-dir>` 说明、指令文档路径与 `archdesign_unavailable` 提示改为安装后的形态 |
| `README.md` | 顶部状态段补一句；「在 coding agent 中使用 interface_design skill」一节改为两条安装命令，说明项目范围与 `-g`、`npx skills update`，保留软链开发方式；4 处指令文档链接、实现位置表 |
| `coding agent 接入实现依据.md` | 第 27、28 行默认值补注新安装方式；第 39、40、80 行链接改指 `references/` |
| `docs/initial-analysis.md`、`check/review.go` | 一处链接、一处注释改指新位置 |
| `docs/agent-skill.md` | 开头加一段说明路径与安装方式已迁移，其余原文保留 |
| `docs/adr/0005-distribute-skill-via-skills-cli.md` | 新建 |
| `examples/todo/AGENTS.md` | 用改后的 `write-rules.py --project examples/todo` 重写标记块（`replaced`），只有第 4 条的指令路径变化；文件只含标记块，块外无内容 |
| `skill 分发安装实现依据.md` | 状态行改为已实施并链接本文 |
| `docs/skill-distribution.md` | 本文 |

## 实现选择

- **统一安装提示。** 三个脚本与 SKILL.md 用同一段文字：「安装方式：运行 `go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest`（需要 Go 1.22+），并确保 `$(go env GOPATH)/bin` 在 PATH 上；或设置环境变量 ARCHDESIGN_BIN 指向已构建的二进制。」`state.py` 与 `export-request.py` 各放一个 `INSTALL_HINT` 常量，bash 脚本内联同一段。
- **skill 自身位置解析不变。** 仍用 `os.path.realpath(__file__)` 与 `readlink -f` 取脚本真实路径，再上溯一级得到 `<skill-dir>`；通过 `.claude/skills/interface_design` 软链调用时落到 `.agents/skills/interface_design` 真实目录（验证记录第 4 项）。`archdesign` 仍按 `ARCHDESIGN_BIN` 优先、其次 PATH 解析，不新增查找位置。
- **指令文档缺失不再静默。** `state.py` 检查三份 `references/*.md` 是否存在，缺失时在 `warnings` 里说明「skill 安装不完整」；`prompt_docs` 仍输出路径。
- **规则块里的指令路径仍写绝对路径。** `write-rules.py` 写 `<skill-dir 真实路径>/references/capability-review.md`。项目范围安装时该路径位于目标项目内，全局安装时位于用户目录。
- **SKILL.md frontmatter。** `compatibility`（179 字符）写 Go 1.22+ 与安装命令、python3、bash、工作台只监听 `127.0.0.1:8090`、不支持 Windows 原生环境；`metadata` 为 `version: "0.1.0"`、`author: ProLeon3`；保留 `argument-hint`。用 PyYAML 解析通过，`description` 227 字符，正文 142 行。
- **ADR 引用。** SKILL.md 不再用 `<skill-dir>/../../docs/adr/…`，改为「源仓库 `docs/adr/` 下的 ADR-0003 与 ADR-0004」加 GitHub 链接。

## 依据文档没有写明、由实施者判断的事项

1. `<owner>`：用户消息里的占位符未填写；本机 `gh auth status` 登录账号为 `ProLeon3`，与 `~/.agents/.skill-lock.json` 里已有的 `ProLeon3/skills` 来源一致，据此取 `ProLeon3`。仓库名不同时需要同步改 module 路径、所有 import、脚本与 SKILL.md 里的提示文字、README 与本文。
2. 本机旧软链 `~/.agents/skills/interface_design` 与 `~/.claude/skills/interface_design`：用户消息里的选项未填写，按依据文档「边界」默认不删；用户随后决定验收第 5 项亲自做，不由 agent 执行。
3. `docs/agent-skill.md` 只在开头加说明，历史路径与验证命令原文不改（依据文档默认值）。
4. 规则块路径仍写绝对路径（依据文档默认值），没有改成项目相对路径。
5. Windows 限制写在 SKILL.md 的 `compatibility` 与 README 的安装说明里。
6. 补了 ADR-0005：module 路径改为 GitHub 路径会长期约束仓库位置，值得记录。
7. `coding agent 接入实现依据.md` 第 27 行「skill 位置与名称」的默认值（软链安装）也随之过时，依据文档只列了第 28 行；两行都补了一句注记，原文保留。
8. 验收第 4 项发现 `skills/interface_design/examples/initial-analysis-response.json` 的依据行号对应提交 ebc2832 时的 `examples/todo` 源码，提交 360eff6 给 todo 加了截止日期后行号已变，直接导入被程序拒绝。验收时只在副本里修正依据位置（相当于 agent 修正自己的输出），仓库内示例未改。同日稍后应用户要求单独重新生成了该示例：以 `examples/todo` 当前源码的副本导出 `initial_analysis` 请求，按源码分析指令的字数限制重写 design 与 analysis（沿用已确认设计的对象 ID，`forbidden_dependencies` 为空），`archdesign proposal-import` 一次通过，`state.py` 进入 `await_review`；示例的 `request_id` 仍绑定生成时的临时项目路径，只示范格式。
9. 验收时 `127.0.0.1:8090` 被用户正在运行的工作台占用。`start-workbench.sh` 用默认地址验证了「已在监听则不重复启动」分支；用 `--addr 127.0.0.1:8096` 验证了后台启动、再次调用不重复启动与找不到 archdesign 的错误文字，验证后停止了该进程。`state.py` 相应传 `--addr`。
10. 验收前删除了 `skills/interface_design/scripts/__pycache__/`（git 已忽略），避免被 CLI 一起复制。

## 验证记录

环境：Go 1.26.6，python3 3.12.3，Node v22.23.2 / npx 10.9.8，skills CLI 1.7.0（`npx -y skills@latest`）。所有验收都在仓库之外的临时目录进行。原始输出当时写在 coding agent 的会话临时目录里，会话结束后该目录已被清理，没有保留下来；下文各表是依据当时输出整理的记录。下文提案 ID 与 `revision` 只取前 12 位。

### 改动清单每步之后的构建检查

| 步骤 | `go build ./...` | `go vet ./...` | `go test ./...` |
| --- | --- | --- | --- |
| 1 module 路径 | 通过 | 通过 | `check`、`cmd/archdesign`、`design`、`designer`、`store`、`workbench` ok |
| 2 指令文档移入 skill | 通过 | 通过 | 同上 |
| 3 脚本 | 通过 | 通过 | 同上；另 `python3 -m py_compile` 三个脚本、`bash -n start-workbench.sh` 通过 |
| 4 SKILL.md、5 文档 | 通过 | 通过 | 同上（最终一次见 `09-final-go.txt`） |

步骤 1 之后 `go install ./cmd/archdesign` 生成 `$(go env GOPATH)/bin/archdesign`，`archdesign help` 输出用法。

### 验收第 1 项：构建与残留

`go build`、`go vet`、`go test` 通过；`grep -rn "interfaceisall" --include='*.go' .` 无输出。

### 验收第 2 项：旧路径残留

`grep -rn "docs/agent-prompts" --exclude-dir=.git .` 剩余命中：`skill 分发安装实现依据.md`（依据文档自身对旧路径的描述，6 处）、`docs/agent-skill.md` 第 7 行（迁移说明）与第 89 行（原始验证记录）、`docs/adr/0005-distribute-skill-via-skills-cli.md` 第 9 行（背景）与本文（改动清单、判断事项）。都是历史性描述，没有仍指向旧位置的链接或脚本路径；`docs/verification/` 里没有命中。

### 验收第 3 项：skills CLI 安装到临时目录 `T`

`cd T && npx -y skills@latest add <本仓库绝对路径> --skill interface_design --agent claude-code codex -y`，退出码 0。CLI 输出 `./.agents/skills/interface_design  universal: Codex  symlinked: Claude Code`。结果：

```text
T/.agents/skills/interface_design/{SKILL.md, scripts/{state.py,export-request.py,write-rules.py,start-workbench.sh}, references/{design.md,source-analysis.md,capability-review.md}, examples/{spec.md, initial-analysis-response.json, design-response.json, adjust-response.json}}
T/.claude/skills/interface_design -> ../../.agents/skills/interface_design
T/skills-lock.json  （source 为本仓库的相对路径，sourceType local，computedHash）
```

没有创建 `.codex/` 目录；脚本保留可执行位。

### 验收第 4 项：`examples/todo` 副本旅程（通过 `T/.claude/skills/interface_design/scripts/` 软链调用）

副本删除了 `.architecture/` 与 `AGENTS.md`。「接受」与「确认」在真实流程中由用户在浏览器完成，下表中的 `proposal-accept` 与 `confirm` 只是验收替身。

| 步骤 | 命令 | 观察结果 |
| --- | --- | --- |
| 1 | `state.py --project <副本> --addr 127.0.0.1:8096` | `phase=initial_analysis`；`skill.skill_dir` 为 `T/.agents/skills/interface_design`（软链已解析到真实目录）；`prompt_docs` 三个路径都在 `T/.agents/skills/interface_design/references/` 下且存在；`warnings=[]`；输出不再含 `repo_dir` |
| 2 | `PATH=/usr/bin:/bin`、未设 `ARCHDESIGN_BIN` 的 `state.py` | `phase=archdesign_unavailable`，`error=PATH 上没有 archdesign，环境变量 ARCHDESIGN_BIN 也未设置`，`install_hint` 含 `go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest` |
| 3 | `export-request.py --kind initial_analysis` | `ok:true`，请求 `419124920757…`，`has_source:true`，`prompt_doc=T/.agents/skills/interface_design/references/source-analysis.md`，源码 3 文件、18002 字节 |
| 4 | 示例响应改写 `request_id` 后 `proposal-import` | **被拒绝**：`协作 cli-uses-task-service 缺少可直接关联目标能力的代码引用；动态或间接使用应标记为不确定`。原因是示例的依据行号对应旧版 todo 源码（判断事项 8） |
| 4b | 修正依据位置后的响应再 `proposal-import` | 通过，提案 `5041bc7349f5…`；`state.py` → `await_review`，`pending_proposal.kind=initial_analysis` |
| 5 | 替身 `proposal-accept` → `state.py` | `await_baseline_confirmation`，`initial_analysis.status=accepted` |
| 5 | `show -draft` 取 `draft_hash` 后替身 `confirm -expected none` | `revision=6364d3ad8c28…`，`initial_analysis_id=5041bc7349f5…`；`state.py` → `write_rules`，`rules.stale=true`，`targets=[AGENTS.md]`，`commands` 指向软链解析后的 `T/.agents/skills/interface_design/scripts/write-rules.py` |
| 6 | `write-rules.py` 两次 | 第一次 `AGENTS.md created`；第二次 `unchanged`。块内能力审查指令路径为 `T/.agents/skills/interface_design/references/capability-review.md`，文件存在；起止标记各一个。之后 `state.py` → `report_status`，`warnings=[]` |
| 7 | `start-workbench.sh --project <副本>`（默认 8090，被用户的工作台占用） | `ok:true`，`started:false`，附「工作台已在监听；…请在页面「选择项目」中输入」提示 |
| 7 | `--addr 127.0.0.1:8096` | `ok:true`，`started:true`，`pid`、`log`、`project` 齐全；再次调用 `started:false`；验证后已停止该进程，端口释放 |
| 7 | `PATH=/usr/bin:/bin` 的 `start-workbench.sh` | 退出码 2，错误文字与统一提示一致 |
| 8 | `archdesign check -project <副本>` | 退出码 0，0 违规，`semantic_status=not_run` |

### 验收第 5 项：本机全局安装

由用户自行验证，不由 agent 执行。步骤：删除 `~/.agents/skills/interface_design` 与 `~/.claude/skills/interface_design` 两条旧开发软链；运行 `npx skills add ProLeon3/interface_is_all -g -y`；在 Claude Code 新会话里确认能列出 `interface_design`，`/interface_design` 能启动工作台并输出状态。结果待回填本文。

### 验收第 6 项：推送与远端验证（2026-09-22 完成）

- 提交 `cf279f9` 后用 `gh repo create ProLeon3/interface_is_all --public --source=. --remote=origin --push` 创建公开仓库。本机 git 走 HTTPS 时出现两次 `gnutls_handshake() failed: The TLS connection was non-properly terminated`，第 3 次推送成功；标签 `git tag -a v0.1.0` 一次推送成功。远端 `refs/heads/main` 与 `refs/tags/v0.1.0` 都指向 `cf279f9`。
- 临时目录 `GOBIN=<临时目录>/bin go install github.com/ProLeon3/interface_is_all/cmd/archdesign@v0.1.0`：退出码 0，`go: downloading github.com/ProLeon3/interface_is_all v0.1.0`，`archdesign help` 输出用法；`go version -m` 显示 `mod github.com/ProLeon3/interface_is_all v0.1.0 h1:x0pnhe3+dn0ElEkGmB6FfJtY5Ap77AvJ8/NBSt1AlLA=`。module 路径含大写字母 `ProLeon3` 没有造成问题。
- 另一个临时目录 `go install …@latest`：退出码 0，`go version -m` 同样解析到 `v0.1.0`（同一 `h1` 哈希）。
- 新临时目录 `npx -y skills@latest add ProLeon3/interface_is_all -y`：CLI 从 `https://github.com/ProLeon3/interface_is_all.git` 克隆，找到 1 个 skill；未指定 `--agent` 时自动检测到 13 个 agent，结果仍只有 `./.agents/skills/interface_design/`（universal）与 `./.claude/skills/interface_design -> ../../.agents/skills/interface_design` 软链，加 `skills-lock.json`（`source: ProLeon3/interface_is_all`，`sourceType: github`，`skillPath: skills/interface_design/SKILL.md`，`computedHash` 与本地路径安装时相同）。`diff -r` 安装目录与仓库 `skills/interface_design/` 逐字一致。

## UNVERIFIED

- **本机全局安装后的发现**：`npx skills add … -g` 后 Claude Code 新会话能否列出 `interface_design` 并用 `/interface_design` 启动工作台（验收第 5 项），由用户自行验证，结果待回填。项目范围安装产生的软链布局与 `docs/agent-skill.md` 已验证的全局软链布局一致。
- **Codex 发现**：CLI 标记 `universal: Codex`，本次未启动 Codex 验证；`~/.agents/skills` 是否被扫描沿用 `docs/agent-skill.md` 的 UNVERIFIED。
- **`npx skills update`**：未跑更新流程；`skills-lock.json` 的 `computedHash` 是否随内容变化触发更新未验证。
- **另外两份示例响应的上下文已旧**：`examples/design-response.json` 与 `adjust-response.json` 是在 360eff6 之前的 todo 基准上生成的（截止日期需求当时还没实现），说明文字也长于现行字数限制；它们只示范格式，未重新生成。`initial-analysis-response.json` 已按当前源码重新生成并导入验证（判断事项 8）。
- **真实 agent 生成质量、浏览器确认后的回合衔接、规则文件遵循率、`@spec.md` 展开方式、后台工作台在各沙箱的存活**：沿用 `docs/agent-skill.md` 的 UNVERIFIED，本次未新增验证。
