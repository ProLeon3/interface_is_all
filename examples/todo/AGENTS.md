<!-- interface_design:begin revision=774baec80d0b1238a279d79589c430e203527acd279efe7745cc6475b641bb57 -->
## 模块与接口设计约束（由 interface_design skill 维护）

本项目的模块与接口设计已由用户在浏览器工作台确认，基准保存在 `.architecture/confirmed.json`（确认版本 `774baec80d0b1238a279d79589c430e203527acd279efe7745cc6475b641bb57`，确认人「工作台验收代理（示例，不代表用户业务定稿）」，确认时间 2026-09-22T11:17:12.664683931Z）。后续实现请遵守以下约定：

1. 实现前先读取 `.architecture/confirmed.json`，按其中的模块职责（`modules`）、接口能力（`interfaces`）、协作关系（`collaborations`）与禁止依赖（`forbidden_dependencies`）实现；新代码放在所属模块的目录内。
2. 不得为了迁就代码改写 `.architecture/` 下的基准、草稿或历史版本。需要变更设计时回到设计流程：在对话中调用 `/interface_design` 说明变更，由用户在浏览器重新审阅并确认。
3. 每个任务完成后运行 `archdesign check -project <项目根目录>`（在项目根目录可写 `-project .`），并在对话中如实汇报结果：违规（规则 ID、两端包、位置）、`unassigned_packages`、`empty_modules` 与 `incomplete` 诊断。确定性违规先修改代码再复查，不改基准。
4. 接口能力与模块职责是否偏离设计，程序不做硬检查。任务涉及接口实现或新增公开能力时，运行 `archdesign review-request -project <项目根目录>` 导出能力审查请求，按 `/home/pr0le0n/projects/resume_project/interface_is_all/skills/interface_design/references/capability-review.md` 生成响应后用 `archdesign review-import` 导入，并把结论（均为 `pending_confirmation`）汇报给用户核对。
5. 以上是给 agent 的指令，不是硬保障：程序只能硬检查被禁止的直接包依赖，`check` 的 `semantic_status` 始终为 `not_run`。不要向用户声称设计已被强制遵守。
<!-- interface_design:end -->
