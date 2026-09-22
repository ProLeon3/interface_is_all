# skill 分发安装实现依据

确认日期：2026-09-22  
状态：已实施（2026-09-22；`<owner>` 取 GitHub 登录账号 `ProLeon3`，仓库名 `interface_is_all` 不变）。实现选择、验证记录与 UNVERIFIED 列表见 [docs/skill-distribution.md](docs/skill-distribution.md)；「改动清单」第 6 步已于同日完成：仓库 https://github.com/ProLeon3/interface_is_all（公开），标签 `v0.1.0`，远端 `go install` 与 `npx skills add` 均已验证；「验收依据」第 5 项本机全局安装由用户自行验证。本文其余内容保留实施前的原文。

目标：任何人在自己的机器上用两条命令装好本项目，然后在目标项目里输入 `/interface_design @spec.md` 即可使用：

```sh
go install github.com/<owner>/interface_is_all/cmd/archdesign@latest
npx skills add <owner>/interface_is_all
```

`<owner>` 是本仓库将要推送到的 GitHub 账号或组织，实施前向用户确认（本机 `~/.agents/.skill-lock.json` 里出现过 `ProLeon3/skills`，但没有核实它就是本仓库要用的账号）。仓库名若不是 `interface_is_all`，module 路径和安装命令随仓库名一起改。

## 实测依据

以下事实于 2026-09-22 在本机用 skills CLI 1.7.0（`npx -y skills@latest`）核实，实施者可以直接依赖。

1. **发现规则。** CLI 在仓库根目录、`skills/`、`.claude/skills/`、`.agents/skills/` 等标准位置向下最多三层查找 `SKILL.md`。本仓库的 `skills/interface_design/SKILL.md` 已能被发现：`npx skills add . --list` 列出 `interface_design` 及其 description。名称里的下划线没有被拒绝。
2. **安装内容与位置。** CLI 只复制含 `SKILL.md` 的那个目录（本仓库是 `SKILL.md`、`scripts/`、`examples/`），不复制仓库其他部分，不编译任何程序，也没有安装钩子。项目范围（默认）放到 `./.agents/skills/interface_design/`，全局（`-g`）放到 `~/.agents/skills/interface_design/`。Claude Code 通过相对软链 `.claude/skills/interface_design -> ../../.agents/skills/interface_design` 使用；Codex 直接读 `.agents/skills`（CLI 称之为 universal）。安装后在目标位置写 `skills-lock.json`，记录来源与内容哈希，`npx skills update` 据此更新。
3. **来源格式。** 接受 `owner/repo`、GitHub 与 GitLab 的 URL、指向子目录的 tree URL、`git@…` 地址与本地路径。本仓库目前没有 git remote，所以 `owner/repo` 形式要等推送到 GitHub 后才可用；本地路径形式可以随时用来验证。
4. **规范约束。** Agent Skills 规范（agentskills.io）要求 `name` 只含小写字母、数字与连字符且与目录名一致；`description` 不超过 1024 字符；可选字段 `compatibility`（不超过 500 字符，写环境要求）、`metadata`（字符串到字符串的映射）、`license`。约定 `scripts/`、`references/`、`assets/` 三个子目录，文件引用用相对 skill 根目录的路径。
5. **安装后失效的两处依赖。** 三个脚本与 SKILL.md 都用 `<skill-dir>/../../docs/agent-prompts/` 定位指令文档，安装后这个路径落在用户家目录或目标项目里，文件不存在。`archdesign` 的安装提示是「在本仓库运行 `go install ./cmd/archdesign`」，安装后没有「本仓库」；`go.mod` 的 module 路径是 `interfaceisall`，也不能用 `go install <路径>@latest` 从远端安装。

## 已确认的决策

| 决策点 | 结论 | 理由或直接影响 |
| --- | --- | --- |
| skill 自包含 | 三份指令文档移入 `skills/interface_design/references/`，脚本与 SKILL.md 只引用 skill 目录内的路径 | CLI 只复制 skill 目录；`references/` 是规范约定的位置 |
| archdesign 安装方式 | `go.mod` 的 module 路径改为 `github.com/<owner>/interface_is_all`，安装命令统一为 `go install github.com/<owner>/interface_is_all/cmd/archdesign@latest` | 不需要克隆仓库；脚本的提示、README 与 `compatibility` 字段写同一条命令。用户机器需要 Go 1.22 或更高 |
| 发布位置 | 本仓库整体推送到 GitHub，用户用 `npx skills add <owner>/interface_is_all` 安装 | Go 程序与 skill 同步演进，不把 skill 拆到别的 skills 仓库 |
| 名称 | 保持 `interface_design` | 改成连字符更合规，但会改掉斜杠命令；CLI 实测接受下划线。见「可选后续」 |
| 程序主体 | Go 逻辑、前端、Schema 零改动；只做 module 路径与 import 路径的机械替换 | 本次是分发方式的改动，不是功能改动 |
| 预编译二进制 | 本次不做 | 先用 `go install`；下载预编译二进制作为可选后续 |

## 采用的默认值

以下默认值未单独询问，可随时推翻。

| 事项 | 默认值 |
| --- | --- |
| 指令文档原位置 `docs/agent-prompts/` | 用 `git mv` 移走，不留软链；仓库内所有链接改指新位置 |
| SKILL.md 新增字段 | `compatibility`：一句话写清依赖（Go 1.22+ 用于安装 archdesign、python3、bash；工作台只监听 `127.0.0.1:8090`）。`metadata`：`version`、`author`，值都是字符串。保留 `argument-hint`（Claude Code 私有字段，CLI 忽略） |
| 找不到 archdesign 时的提示 | 三个脚本与 SKILL.md 统一为：运行 `go install github.com/<owner>/interface_is_all/cmd/archdesign@latest` 并确保 `$(go env GOPATH)/bin` 在 PATH 上，或设置 `ARCHDESIGN_BIN` |
| 规则文件标记块里的能力审查指令路径 | 仍写绝对路径，指向已安装 skill 的 `references/capability-review.md` |
| 实现与验收记录 | 新建 `docs/skill-distribution.md`，README 与本文链接它 |
| 版本标签 | 推送后打 `v0.1.0` 标签，让 `@latest` 解析到稳定版本而不是主分支伪版本 |
| 本机开发方式 | 继续用软链开发；在本机用 `npx skills add -g` 验证前先删除 `~/.agents/skills/interface_design` 与 `~/.claude/skills/interface_design` 两条旧软链（需用户同意） |

## 目标形态

安装后的 skill 目录（项目范围为例）：

```
<目标项目>/
├── .agents/skills/interface_design/
│   ├── SKILL.md
│   ├── scripts/        state.py  export-request.py  write-rules.py  start-workbench.sh
│   ├── references/     design.md  source-analysis.md  capability-review.md
│   └── examples/       spec.md  initial-analysis-response.json  design-response.json  adjust-response.json
├── .claude/skills/interface_design -> ../../.agents/skills/interface_design
└── skills-lock.json
```

仓库内对应的源位置是 `skills/interface_design/`，结构与安装后一致。`archdesign` 由 `go install` 放到 `$(go env GOPATH)/bin`，脚本仍按「`ARCHDESIGN_BIN` 优先，其次 PATH」解析，不新增查找位置。

## 改动清单

按下列顺序做，每步之后都能构建和运行。行号以 2026-09-22 的源码为准，实施时以实际内容为准。

### 1. module 路径

- `go.mod`：`module interfaceisall` 改为 `module github.com/<owner>/interface_is_all`。
- 仓库内 39 个 Go 文件、97 处 `"interfaceisall/…"` import 改为新前缀（`grep -rl '"interfaceisall/' --include='*.go' .` 可列出）。`examples/todo` 与 `examples/shop` 是独立 module，不受影响。
- 之后 `go build ./...`、`go vet ./...`、`go test ./...` 必须通过；`go install ./cmd/archdesign` 后 `archdesign help` 正常。

### 2. 指令文档移入 skill

- `git mv docs/agent-prompts/design.md docs/agent-prompts/source-analysis.md docs/agent-prompts/capability-review.md skills/interface_design/references/`。三份文档内部没有相对链接，内容不用改。
- 更新仓库内指向旧位置的链接与文字：`README.md` 第 44、62、111、242 行；`coding agent 接入实现依据.md` 第 39、40、80 行；`docs/initial-analysis.md` 第 27 行；`check/review.go` 第 17 行的注释。`docs/agent-skill.md` 第 59、87 行是当时的验证记录，保留原文，在该文开头加一句说明路径已迁移。

### 3. 脚本

三个 Python 脚本与 bash 脚本的改动都只涉及路径解析与提示文字，逻辑不变。

- `scripts/state.py`：`skill_paths()`（第 49 到 64 行）改为 `prompts = os.path.join(skill_dir, "references")`，去掉 `repo_dir`；输出里用 `skill_dir` 取代 `repo_dir`。`resolve_archdesign()` 的 `install_hint`（第 67 到 71 行）改为默认值表里的统一提示，不再接收 `repo_dir` 参数。指令文档不存在时在 `warnings` 里说明，而不是静默输出一个不存在的路径。
- `scripts/export-request.py`：`skill_paths()`（第 31 到 39 行）同样改为 `<skill-dir>/references/`；第 164 行找不到 archdesign 的提示改为统一提示。
- `scripts/write-rules.py`：`repo_dir()`（第 30 到 32 行）改为返回 skill 目录，第 117 行改为 `<skill-dir>/references/capability-review.md`。
- `scripts/start-workbench.sh`：第 50 到 52 行去掉 `repo_dir` 推算，提示改为统一提示。
- 所有脚本继续用 `os.path.realpath` 或 `readlink -f` 解析自身位置，保证通过 `.claude/skills` 软链调用时仍落到 `.agents/skills` 下的真实目录。

### 4. SKILL.md

- frontmatter 增加 `compatibility` 与 `metadata`（见默认值表）。
- 第 9 行引用 ADR 的相对路径改为仓库内路径的文字说明或 GitHub 链接，不再用 `<skill-dir>/../../`。
- 第 11 行改为：脚本在 `<skill-dir>/scripts/`，指令文档在 `<skill-dir>/references/`，示例在 `<skill-dir>/examples/`；安装后 `<skill-dir>` 通常是 `.agents/skills/interface_design`（项目范围）或 `~/.agents/skills/interface_design`（全局），`.claude/skills` 下是指向它的软链。
- 第 19 行的 `docs/agent-prompts/design.md` 与 `source-analysis.md` 改为 `references/` 路径。
- 第 46 行 `archdesign_unavailable` 的提示改为统一提示。
- 正文保持在 500 行以内，不复制指令文档内容。

### 5. 文档

- `README.md`「在 coding agent 中使用 interface_design skill」一节（第 90 行起）：安装改为本文开头的两条命令；保留「本仓库开发时用软链」作为备选；说明 `npx skills add` 的项目范围与 `-g` 全局范围、`npx skills update` 更新；链接 `docs/skill-distribution.md`。第 20、124 行的 `go build` 示例可保留，用于本仓库开发。
- `README.md` 顶部状态段补一句：2026-09-22 决定以 `npx skills add` 分发，见本文。
- `coding agent 接入实现依据.md` 第 28 行「程序二进制」默认值改为新安装命令。
- 新建 `docs/skill-distribution.md`：实现选择、验证记录、UNVERIFIED 列表，格式参照 `docs/agent-skill.md`。
- `examples/todo/AGENTS.md` 第 9 行的标记块含本机绝对路径 `/home/pr0le0n/.../docs/agent-prompts/capability-review.md`。改完脚本后运行 `python3 skills/interface_design/scripts/write-rules.py --project examples/todo` 重写该块，让示例反映新路径；块外内容不得变化。

### 6. 推送与远端验证

这一步需要用户提供 GitHub 仓库并授权推送，实施者先完成前五步并在本地验收，再向用户要仓库地址。

- 用户创建仓库并添加 remote 后推送 `main`，打标签 `v0.1.0`。
- 在仓库之外的临时目录验证 `go install github.com/<owner>/interface_is_all/cmd/archdesign@v0.1.0`。仓库为私有时需要设置 `GOPRIVATE`，本文默认仓库公开。
- 在临时目录验证 `npx skills add <owner>/interface_is_all -y`。

## 验收依据

本地验收在推送前完成，全部在仓库之外的临时目录进行；不要在本仓库内运行 `npx skills add`，否则会在仓库里生成 `.agents/`、`.claude/skills/` 与 `skills-lock.json`。

1. `go build ./...`、`go vet ./...`、`go test ./...` 通过；`grep -rn "interfaceisall" --include='*.go' .` 没有输出。
2. `grep -rn "docs/agent-prompts" --exclude-dir=.git .` 只剩 `docs/agent-skill.md` 与 `docs/verification/` 里的历史记录。
3. 在临时目录 `T` 运行 `npx -y skills@latest add <本仓库绝对路径> --skill interface_design --agent claude-code codex -y`，得到 `T/.agents/skills/interface_design/` 含 `SKILL.md`、`scripts/`、`references/`、`examples/`，以及 `T/.claude/skills/interface_design` 相对软链和 `T/skills-lock.json`。
4. 用 `examples/todo` 的副本（删除其 `.architecture/` 与 `AGENTS.md`）作为目标项目，通过 `T/.claude/skills/interface_design/scripts/` 这条软链路径调用脚本：
   - `state.py --project <副本>`：`phase=initial_analysis`，`prompt_docs` 里三个路径都在 `T/.agents/skills/interface_design/references/` 下且存在，`warnings` 为空。
   - `PATH=/usr/bin:/bin` 且未设 `ARCHDESIGN_BIN` 时：`phase=archdesign_unavailable`，`install_hint` 含 `go install github.com/<owner>/interface_is_all/cmd/archdesign@latest`。
   - `export-request.py --project <副本> --kind initial_analysis`：`ok:true`，`prompt_doc` 指向安装目录下的 `references/source-analysis.md`。
   - 用 `examples/initial-analysis-response.json` 改写 `request_id` 后 `archdesign proposal-import`，再用命令行替身 `proposal-accept` 与 `confirm`（只在验收里允许），然后 `write-rules.py --project <副本>`：生成的 `AGENTS.md` 标记块里的能力审查指令路径在安装目录下且文件存在；再运行一次输出 `unchanged`。
   - `start-workbench.sh --project <副本>` 返回 `ok:true`；找不到 archdesign 时的错误文字与统一提示一致。
5. 在本机全局安装验证（需用户同意删除旧软链后）：`npx skills add <本仓库绝对路径> -g -y` 后 Claude Code 新会话能列出 `interface_design`，`/interface_design` 能启动工作台并输出状态。
6. 推送后的远端验证按「改动清单」第 6 步执行并记录；未能执行的项在 `docs/skill-distribution.md` 的 UNVERIFIED 列表里如实写明。

## 边界

- 不改变程序功能、交换协议、Schema、前端与 `.architecture/` 存储格式；`docs/adr/` 已有决策不受影响。
- 不发布到 npm，不改 skills CLI 的行为，不在 SKILL.md 里加安装钩子之类 CLI 不支持的东西。
- 不删除用户本机的开发软链，除非用户明确同意；不代用户创建 GitHub 仓库、不推送，等用户给出仓库并授权。
- 提交 git 只在用户要求时进行；提交信息与代码注释按仓库惯例用中文。

## 实施者需自行判断的事项

1. `docs/agent-skill.md` 里的历史路径与验证命令是否只加说明、不改原文。默认只加说明。
2. 规则文件标记块里的指令路径：skill 安装在目标项目内（项目范围）时，写项目相对路径更稳；本文默认仍写绝对路径，改成相对路径需同时更新 `docs/agent-skill.md` 描述的块格式。
3. Windows 支持不在本次范围（脚本依赖 bash 与 `/dev/tcp`），若要说明限制，写在 `compatibility` 与 README 即可。
4. 是否把这次分发方式写成 ADR-0005。module 路径改为 GitHub 路径会长期约束仓库位置，实施者认为值得记录时按 `docs/adr/` 惯例补一份，不强制。

## 可选后续

- **预编译二进制。** 用 GitHub Releases 发布各平台的 `archdesign`，在 skill 里加 `scripts/install-archdesign.sh` 按系统与架构下载到 `<skill-dir>/bin/`，三个脚本在 `ARCHDESIGN_BIN` 与 PATH 之后多查这个位置。做了这一步用户就不需要装 Go。
- **名称改为 `interface-design`。** 完全符合规范，但斜杠命令随之改变，需要同步 README、示例与本机软链。
- **登记到 skills.sh。** 仓库公开后可用 `npx skills find` 检索；是否需要额外登记未查证。
