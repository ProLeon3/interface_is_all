<!-- interface_design:begin revision=774baec80d0b1238a279d79589c430e203527acd279efe7745cc6475b641bb57 -->
## 模块与接口设计约束（由 interface_design skill 维护）

设计基准在 `.architecture/confirmed.json`，由用户在浏览器工作台确认：版本 `774baec80d0b1238a279d79589c430e203527acd279efe7745cc6475b641bb57`，确认人「工作台验收代理（示例，不代表用户业务定稿）」，时间 2026-09-22T11:17:12.664683931Z。实现时遵守：

1. 动手前读 `.architecture/confirmed.json`：按 `modules` 的职责、`interfaces` 的能力、`collaborations` 的协作和 `forbidden_dependencies` 实现，新代码放进所属模块目录。
2. 不为迁就代码改 `.architecture/` 下的基准、草稿或历史。要改设计就在对话里调用 `/interface_design` 说明，由用户在浏览器重新确认。
3. 每个任务完成后运行 `archdesign check -project <项目根目录>`，如实汇报：违规（规则 ID、两端包、位置）、`unassigned_packages`、`empty_modules`、`incomplete` 诊断。违规先改代码再复查，不改基准。
4. 接口能力与职责是否偏离，程序不做硬检查。任务涉及接口实现或新增公开能力时，运行 `archdesign review-request -project <项目根目录>` 导出请求，按 `/home/pr0le0n/projects/resume_project/interface_is_all/skills/interface_design/references/capability-review.md` 生成响应，用 `archdesign review-import` 导入，把结论（均为 `pending_confirmation`）交给用户核对。
5. 以上是给 agent 的指令，不是硬保障：程序只硬检查被禁止的直接包依赖，`check` 的 `semantic_status` 始终为 `not_run`。不要对用户说设计已被强制遵守。
<!-- interface_design:end -->
