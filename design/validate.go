package design

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"interfaceisall/internal/jsonfile"
)

// Schema 内嵌唯一的结构定义，命令行和保存接口使用同一份规则。
//
//go:embed design.schema.json
var Schema string

var compiledSchema = jsonschema.MustCompileString("design.schema.json", Schema)

// Issue 的 Path 使用 JSON Pointer，便于工作台定位设计字段。
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError 包含一次验证发现的全部关系错误。
type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Issues))
	for i, issue := range e.Issues {
		parts[i] = issue.Path + ": " + issue.Message
	}
	return "设计验证失败：" + strings.Join(parts, "；")
}

// Parse 先执行 JSON Schema 校验，再检查跨对象约束。
func Parse(data []byte) (Design, error) {
	var raw any
	if err := jsonfile.Decode(data, &raw); err != nil {
		return Design{}, &ValidationError{[]Issue{{"json", "", err.Error()}}}
	}
	if err := compiledSchema.Validate(raw); err != nil {
		issues := []Issue{}
		var visit func(*jsonschema.ValidationError)
		visit = func(e *jsonschema.ValidationError) {
			if len(e.Causes) == 0 {
				issues = append(issues, Issue{"schema", e.InstanceLocation, "结构不符合 JSON Schema：" + e.Message})
			}
			for _, cause := range e.Causes {
				visit(cause)
			}
		}
		if e, ok := err.(*jsonschema.ValidationError); ok {
			visit(e)
		} else {
			issues = append(issues, Issue{"schema", "", err.Error()})
		}
		return Design{}, &ValidationError{issues}
	}
	var d Design
	if err := json.Unmarshal(data, &d); err != nil {
		return Design{}, err
	}
	if issues := relationships(d); len(issues) > 0 {
		return Design{}, &ValidationError{issues}
	}
	return d, nil
}

// Validate 也校验程序构造的值，保证保存、扫描与 JSON 导入的约束一致。
func Validate(d Design) error {
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = Parse(data)
	return err
}

// Contains 按路径段匹配，避免把 payment-old 误归入 payment。
func Contains(root, relative string) bool {
	return root == "." || relative == root || strings.HasPrefix(relative, root+"/")
}

// ModuleAt 返回包目录所属模块；空字符串表示尚未纳入设计。
func (d Design) ModuleAt(relative string) string {
	for _, m := range d.Modules {
		if Contains(m.Root, relative) {
			return m.ID
		}
	}
	return ""
}

func relationships(d Design) []Issue {
	issues := []Issue{}
	add := func(code, pointer, message string) { issues = append(issues, Issue{code, pointer, message}) }
	ids := map[string]string{}
	register := func(id, pointer string) {
		if previous, ok := ids[id]; ok {
			add("duplicate_id", pointer, "ID 与 "+previous+" 重复："+id)
		}
		ids[id] = pointer
	}
	modules := map[string]bool{}
	for i, m := range d.Modules {
		pointer := fmt.Sprintf("/modules/%d", i)
		register(m.ID, pointer+"/id")
		modules[m.ID] = true
		// 允许尚未创建的目录，但路径必须已规范化且始终位于项目中。
		if m.Root != path.Clean(m.Root) || path.IsAbs(m.Root) || m.Root == ".." || strings.HasPrefix(m.Root, "../") || strings.ContainsAny(m.Root, "\\:\x00") {
			add("invalid_root", pointer+"/root", "目录必须是规范的项目相对路径，使用 / 分隔，不能包含 .. 越界")
		}
		for j := 0; j < i; j++ {
			if Contains(m.Root, d.Modules[j].Root) || Contains(d.Modules[j].Root, m.Root) {
				add("overlapping_roots", pointer+"/root", "模块目录与 "+d.Modules[j].ID+" 重叠")
			}
		}
	}
	interfaces := map[string]Interface{}
	for i, api := range d.Interfaces {
		pointer := fmt.Sprintf("/interfaces/%d", i)
		register(api.ID, pointer+"/id")
		interfaces[api.ID] = api
		if !modules[api.ModuleID] {
			add("unknown_module", pointer+"/module_id", "接口所属模块不存在："+api.ModuleID)
		}
	}
	pairs := map[string]bool{}
	for i, rule := range d.ForbiddenDependencies {
		pointer := fmt.Sprintf("/forbidden_dependencies/%d", i)
		register(rule.ID, pointer+"/id")
		for _, ref := range []struct{ field, id string }{{"from", rule.From}, {"to", rule.To}} {
			if !modules[ref.id] {
				add("unknown_module", pointer+"/"+ref.field, "依赖规则引用的模块不存在："+ref.id)
			}
		}
		if rule.From == rule.To {
			add("self_dependency", pointer, "模块内部依赖不属于跨模块禁止规则")
		}
		pair := rule.From + "\x00" + rule.To
		if pairs[pair] {
			add("duplicate_rule", pointer, "同一依赖方向已存在禁止规则")
		}
		pairs[pair] = true
	}
	uses := map[string]bool{}
	for i, link := range d.Collaborations {
		pointer := fmt.Sprintf("/collaborations/%d", i)
		register(link.ID, pointer+"/id")
		if !modules[link.From] {
			add("unknown_module", pointer+"/from", "协作的调用模块不存在："+link.From)
		}
		api, exists := interfaces[link.InterfaceID]
		if !exists {
			add("unknown_interface", pointer+"/interface_id", "协作引用的接口不存在："+link.InterfaceID)
		} else if api.ModuleID == link.From {
			add("self_collaboration", pointer, "接口协作仅表达跨模块能力使用")
		}
		pair := link.From + "\x00" + link.InterfaceID
		if uses[pair] {
			add("duplicate_collaboration", pointer, "同一模块使用同一接口的协作已经存在")
		}
		uses[pair] = true
	}
	return issues
}
