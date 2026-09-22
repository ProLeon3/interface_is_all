# 真实模型设计后的待办示例

这是独立工作台首轮技术验证的小功能。需求和两轮设计由工作台调用用户配置的真实模型生成；初稿为 `cli`、`task` 两个模块，选中 `task` 后以自然语言要求拆出 `storage`，再由验收代理通过工作台确认示例版本并导出。

本目录代码由工作台外的 coding agent 根据 [design-handoff.json](design-handoff.json) 实现。工作台没有自动生成或修复这些代码。确认记录明确写为「工作台验收代理（示例，不代表用户业务定稿）」，不能将自动化操作解释为用户已完成产品验收。

## 使用

```sh
cd examples/todo
go run -buildvcs=false ./cli -file /tmp/my-todos.json add '核对模块分工'
go run -buildvcs=false ./cli -file /tmp/my-todos.json list
go run -buildvcs=false ./cli -file /tmp/my-todos.json done 1
go run -buildvcs=false ./cli -file /tmp/my-todos.json done 1

go test -race ./...
```

`-file` 放在子命令前；默认文件为当前目录的 `tasks.json`。`add` 输出 ID 和标题，`list` 只列出未完成任务，`done` 支持重复执行。空白标题、未知或无效 ID、损坏的数据文件均返回非零退出状态。完成任务不复用 ID，重复完成不重写文件。

范围是单进程本地命令行，不支持多进程同时写同一文件，不包含网络服务或多用户功能。

## 设计与实现对应

| 设计能力 | 实现位置 | 核对结果 |
| --- | --- | --- |
| `cli-command` | [cli/main.go](cli/main.go)，`Run` | 只处理参数、输出与退出状态；通过 `task` 完成业务 |
| `task-service` | [task/task.go](task/task.go)，`Add`、`Complete`、`Pending` | 校验业务文档、标题与 ID，按 ID 稳定返回；通过 `storage` 读写 |
| `storage-json-document` | [storage/json.go](storage/json.go)，`ReadJSON`、`WriteJSON` | 只处理 JSON 文档，不引用 `task` 或 `cli`；同目录临时文件同步后原子替换 |
| `cli-uses-task-service` | `cli/main.go` 对上述三项业务函数的调用 | 由源码核对落实 |
| `task-uses-storage-json-document` | `task/task.go` 的 `read`、`save` | 业务校验成功且状态变化后才写入，由源码与幂等回归核对 |

Go 依赖检查确认未违反四条显式禁止规则，状态为 `passed`；模型语义审查仍为 `not_run`。上述对应关系由 coding agent 对照代码核查，用户仍可进一步审阅语义。业务测试覆盖进程调用式持久化、创建/完成/列表、重复完成不额外写入、无效操作不改数据、损坏文档不覆盖。

完整需求、修改意图和前后设计在 `.architecture/proposals/`；确认版本在 `.architecture/confirmed.json` 与 `.architecture/versions/`。工作台截图与检查报告见[本轮验收记录](../../docs/standalone-workbench.md#真实模型与实际实现验证)。
