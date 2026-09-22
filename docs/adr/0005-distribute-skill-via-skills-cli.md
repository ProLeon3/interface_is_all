# 0005. 以 skills CLI 分发 skill，以 go install 从 GitHub 安装 archdesign

- **状态**：Accepted（已接受）
- **日期**：2026-09-22
- **相关**：[ADR-0002](0002-external-coding-agent-orchestration.md)、[ADR-0003](0003-turn-based-skill-integration.md)、[ADR-0004](0004-design-compliance-via-project-rules-file.md)；实施依据见[skill 分发安装实现依据](../../skill%20分发安装实现依据.md)，实现与验证记录见 [docs/skill-distribution.md](../skill-distribution.md)

## 背景

`interface_design` skill 与 `archdesign` 程序此前只能在本仓库内使用：skill 通过软链指向仓库源目录；三份 agent 指令文档放在仓库 `docs/agent-prompts/`，脚本与 SKILL.md 用 `<skill-dir>/../../` 相对路径引用；`archdesign` 要先克隆仓库再 `go install ./cmd/archdesign`；`go.mod` 的 module 路径是 `interfaceisall`，无法用 `go install <路径>@latest` 从远端安装。用户要求任何人在自己的机器上用两条命令装好，然后在目标项目里输入 `/interface_design @spec.md` 即可使用。

skills CLI（`npx skills add`）只复制含 `SKILL.md` 的那个目录到 `.agents/skills/<name>/`，并为 Claude Code 建 `.claude/skills/<name>` 软链；不复制仓库其他部分，不编译程序，没有安装钩子。

## 决定

我们将把本仓库整体推送到 GitHub（`github.com/ProLeon3/interface_is_all`），用两条命令分发：

```sh
go install github.com/ProLeon3/interface_is_all/cmd/archdesign@latest
npx skills add ProLeon3/interface_is_all
```

为此：

- skill 自包含：三份指令文档移入 `skills/interface_design/references/`，脚本与 SKILL.md 只引用 skill 目录内的路径；`archdesign` 仍按「`ARCHDESIGN_BIN` 优先，其次 PATH」解析。
- `go.mod` 的 module 路径改为 `github.com/ProLeon3/interface_is_all`，仓库内 import 做机械替换；Go 逻辑、前端、Schema、交换协议与 `.architecture/` 存储格式零改动。
- skill 与程序留在同一仓库同步演进，不拆到独立的 skills 仓库；不发布到 npm；本次不做预编译二进制；名称保持 `interface_design`。

## 后果

- module 路径把仓库绑定到 GitHub 上的这个位置。迁移仓库或改名时，module 路径、所有 import、安装命令、SKILL.md 与脚本中的提示文字都要一起改。
- 用户机器需要 Go 1.22 或更高版本安装 `archdesign`，以及 Node.js 的 `npx` 安装 skill；脚本依赖 bash 与 `/dev/tcp`，不支持 Windows 原生环境。
- `@latest` 解析到最新的 git 标签，推送后需要打标签（首个为 `v0.1.0`），否则会解析到主分支伪版本。
- 规则文件标记块里的能力审查指令路径改为已安装 skill 的 `references/capability-review.md` 绝对路径；项目范围安装时该路径位于目标项目内。
- 本仓库开发仍可用软链把 `skills/interface_design/` 装到用户目录，改动即时生效；CLI 安装的是副本，更新要靠 `npx skills update`。

## 考虑过的替代方案

### 独立的 skills 仓库

把 `skills/interface_design/` 拆到另一个仓库分发。skill 与程序分开演进，两边版本容易错位，且指令文档要在两处维护。未采用。

### 预编译二进制

用 GitHub Releases 发布各平台的 `archdesign`，skill 内加下载脚本，用户不必装 Go。需要发布流程与多平台构建，先以 `go install` 起步，列为可选后续。

### 名称改为 `interface-design`

完全符合 Agent Skills 规范的命名要求，但斜杠命令随之改变，需要同步 README、示例与本机软链；CLI 实测接受下划线，暂不改。
