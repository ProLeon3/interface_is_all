// archdesign 提供本地设计保存、确认和检查入口，供用户及外部编程工具调用。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"interfaceisall/design"
	"interfaceisall/internal/jsonfile"
	"interfaceisall/store"
	"interfaceisall/workbench"
)

const usage = `用法：archdesign <命令> [选项]

  schema          输出设计 JSON Schema
  serve           启动本地浏览器工作台（-addr 默认 127.0.0.1:8090）
  validate        校验设计文件（-file 必填）
  save-draft      校验并保存草稿（-file 必填）
  show            读取已确认设计；-draft 读取草稿及其指纹
  confirm         确认已审阅草稿（-draft-hash、-expected、-by 必填；首次 -expected none）
  layout-save     保存独立布局（-file 必填）
  layout-show     读取独立布局
  check           检查直接包依赖并提取公开入口
  review-request  生成供外部 AI 工具使用的接口能力审查请求
  review-import   验证并保存能力审查响应（-request、-file 必填）
  design-request  导出设计或源码分析请求（-kind design|initial_analysis|reanalysis；-file 可选意图 JSON）
  proposal-import 校验并保存外部提案（-file 响应 JSON 必填）
  proposal-show   读取指定提案或当前待审阅提案（-id 可选）
  proposal-accept 接受提案为草稿（-id 必填，不确认基准）
  proposal-discard 撤回提案（-id 必填）

公共选项：-project 项目根目录（默认 .）；-timeout 超时（默认 2m）。
扫描选项：-tags 构建标签。扫描当前平台的非测试 Go 文件。
JSON 结果写入标准输出；诊断及观察文件路径写入标准错误。
退出码：0 操作成功，1 确定性违规，2 输入/执行失败或检查不完整。
check 成功仅代表所执行的确定性检查未发现违规，semantic_status 仍为 not_run。
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, out, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, usage)
		return 0
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	project := flags.String("project", ".", "项目根目录")
	address := flags.String("addr", "127.0.0.1:8090", "工作台回环监听地址")
	file := flags.String("file", "", "输入 JSON 文件")
	requestFile := flags.String("request", "", "审查请求文件")
	kind := flags.String("kind", "", "design、initial_analysis 或 reanalysis")
	proposalID := flags.String("id", "", "提案 ID")
	draft := flags.Bool("draft", false, "读取草稿")
	draftHash := flags.String("draft-hash", "", "已审阅草稿的指纹")
	expected := flags.String("expected", "", "预期已确认版本，首次使用 none")
	actor := flags.String("by", "", "确认人")
	tags := flags.String("tags", "", "Go 构建标签")
	timeout := flags.Duration("timeout", 2*time.Minute, "检查超时")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if flags.NArg() != 0 || *timeout <= 0 {
		return fail(fmt.Errorf("存在未识别的位置参数或无效超时"))
	}
	// 工作台持续提供服务，-timeout 只约束每次检查，不能让整个服务两分钟后退出。
	if command == "serve" {
		options := workbench.Options{Tags: *tags, Timeout: *timeout}
		if err := workbench.Listen(ctx, *project, *address, options, out); err != nil {
			return fail(err)
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	write := func(value any) int {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return fail(err)
		}
		return 0
	}
	if command == "schema" {
		_, err := fmt.Fprintln(out, design.Schema)
		if err != nil {
			return fail(err)
		}
		return 0
	}
	s, err := store.Open(*project)
	if err != nil {
		return fail(err)
	}
	switch command {
	case "validate", "save-draft":
		data, err := readInput(*file)
		if err != nil {
			return fail(err)
		}
		d, err := design.Parse(data)
		if err == nil {
			err = design.ValidateProject(*project, d)
		}
		if err != nil {
			var validation *design.ValidationError
			if errors.As(err, &validation) {
				write(validation)
			}
			return fail(err)
		}
		if command == "save-draft" {
			if _, err := s.SaveDraft(d); err != nil {
				return fail(err)
			}
		}
		return write(map[string]any{"valid": true, "draft_hash": design.Fingerprint(d)})
	case "show":
		if *draft {
			d, hash, err := s.Draft()
			if err != nil {
				return fail(err)
			}
			return write(map[string]any{"draft_hash": hash, "design": d})
		}
		current, err := s.Confirmed()
		if err != nil {
			return fail(err)
		}
		return write(current)
	case "confirm":
		if *expected == "" {
			return fail(fmt.Errorf("必须显式提供 -expected 版本；首次确认使用 none"))
		}
		if *expected == "none" {
			*expected = ""
		}
		current, err := s.Confirm(*draftHash, *expected, *actor)
		if err != nil {
			return fail(err)
		}
		return write(current)
	case "layout-save":
		data, err := readInput(*file)
		if err != nil {
			return fail(err)
		}
		var layout store.Layout
		if err := jsonfile.Decode(data, &layout); err != nil {
			return fail(err)
		}
		if err := s.SaveLayout(layout); err != nil {
			return fail(err)
		}
		return write(layout)
	case "layout-show":
		layout, err := s.Layout()
		if err != nil {
			return fail(err)
		}
		return write(layout)
	case "design-request", "proposal-import", "proposal-show", "proposal-accept", "proposal-discard":
		return runProposal(ctx, s, command, *kind, *file, *proposalID, *tags, write, stderr)
	case "check", "review-request", "review-import":
		return runCheck(ctx, s, command, *project, *tags, *requestFile, *file, write, stderr)
	default:
		return fail(fmt.Errorf("未知命令 %q；运行 archdesign help 查看用法", command))
	}
}

func readInput(name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("必须提供输入文件路径")
	}
	return os.ReadFile(name)
}
