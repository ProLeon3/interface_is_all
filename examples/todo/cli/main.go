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
		return fail(fmt.Errorf("用法：todo [-file 路径] add <标题> | done <ID> | list"))
	}
	switch command[0] {
	case "add":
		item, err := task.Add(*file, strings.Join(command[1:], " "))
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(out, "%d\t%s\n", item.ID, item.Title)
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
			fmt.Fprintf(out, "%d\t%s\n", item.ID, item.Title)
		}
	default:
		return fail(fmt.Errorf("未知命令：%s", command[0]))
	}
	return 0
}
