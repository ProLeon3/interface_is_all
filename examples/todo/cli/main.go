// 待办命令行只处理参数与输出，所有业务操作交给 task。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"example.test/workbench-todo/task"
)

func main() { os.Exit(Run(os.Args[1:], os.Stdout, os.Stderr)) }

// Run 对应确认设计中的 cli-command，使用可传入的输出流验证退出语义。
func Run(args []string, out, stderr io.Writer) int {
	flags := flag.NewFlagSet("todo", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "tasks.json", "JSON 数据文件路径")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	command := flags.Args()
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 2 }
	if len(command) == 0 {
		return fail(fmt.Errorf("用法：todo [-file 路径] add [-due YYYY-MM-DD] <标题> | done <ID> | list"))
	}
	switch command[0] {
	case "add":
		// -due 放在标题前；日期是否有效由 task 判断，cli 只负责转交。
		addFlags := flag.NewFlagSet("add", flag.ContinueOnError)
		addFlags.SetOutput(stderr)
		due := addFlags.String("due", "", "截止日期 YYYY-MM-DD，可省略")
		if err := addFlags.Parse(command[1:]); err != nil {
			return 2
		}
		item, err := task.Add(*file, strings.Join(addFlags.Args(), " "), *due)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, formatTask(item))
	case "done":
		if len(command) != 2 {
			return fail(fmt.Errorf("用法：done <ID>"))
		}
		id, err := strconv.ParseUint(command[1], 10, 64)
		if err != nil || id == 0 {
			return fail(fmt.Errorf("ID 必须是正整数"))
		}
		item, changed, err := task.Complete(*file, id)
		if err != nil {
			return fail(err)
		}
		label := "已经完成"
		if changed {
			label = "已完成"
		}
		fmt.Fprintf(out, "%s %d\t%s\n", label, item.ID, item.Title)
	case "list":
		if len(command) != 1 {
			return fail(fmt.Errorf("list 不接受额外参数"))
		}
		items, err := task.Pending(*file)
		if err != nil {
			return fail(err)
		}
		for _, item := range items {
			fmt.Fprintln(out, formatTask(item))
		}
	default:
		return fail(fmt.Errorf("未知命令：%s", command[0]))
	}
	return 0
}

// formatTask 输出「ID\t标题」，有截止日期时追加「\t截止 日期」。
func formatTask(item task.Task) string {
	line := fmt.Sprintf("%d\t%s", item.ID, item.Title)
	if item.Due != "" {
		line += "\t截止 " + item.Due
	}
	return line
}
