package designer

import (
	"fmt"
	"path"
	"strings"

	"interfaceisall/design"
	"interfaceisall/source"
)

// Analysis 与设计约束分开保存，源码位置校验不代表语义已得到程序证明。
type Analysis struct {
	Evidence      []Evidence `json:"evidence"`
	Issues        []Finding  `json:"issues"`
	Suggestions   []Finding  `json:"suggestions"`
	Uncertainties []string   `json:"uncertainties"`
}

type Evidence struct {
	Kind        string            `json:"kind"`
	ID          string            `json:"id"`
	Status      string            `json:"status"`
	Explanation string            `json:"explanation"`
	Locations   []source.Location `json:"locations"`
}

type Finding struct {
	Description string            `json:"description"`
	Locations   []source.Location `json:"locations"`
}

func analysisResponseSchema(schema map[string]any) map[string]any {
	str := map[string]any{"type": "string", "minLength": 1}
	obj := func(props map[string]any, keys ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props, "required": keys}
	}
	array := func(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
	integer := map[string]any{"type": "integer", "minimum": 1}
	location := obj(map[string]any{"file": str, "line": integer, "column": integer, "end_line": integer}, "file", "line", "column", "end_line")
	locations := array(location)
	evidence := obj(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"module", "interface", "collaboration"}}, "id": str, "status": map[string]any{"type": "string", "enum": []string{"supported", "uncertain"}}, "explanation": str, "locations": locations}, "kind", "id", "status", "explanation", "locations")
	finding := obj(map[string]any{"description": str, "locations": locations}, "description", "locations")
	schema["properties"].(map[string]any)["analysis"] = obj(map[string]any{"evidence": array(evidence), "issues": array(finding), "suggestions": array(finding), "uncertainties": array(str)}, "evidence", "issues", "suggestions", "uncertainties")
	schema["required"] = []string{"summary", "design", "analysis"}
	return schema
}

// completeCollaborationEvidence 复用接口已给出的目标依据，不要求模型在每条协作重复抄录。
// 只补充缺少目标模块位置的协作；已有但错误的目标位置仍交给后续校验拒绝。
func completeCollaborationEvidence(result *Result) {
	if result.Analysis == nil {
		return
	}
	byID := map[string]*Evidence{}
	for i := range result.Analysis.Evidence {
		e := &result.Analysis.Evidence[i]
		byID[e.ID] = e
	}
	for _, link := range result.Design.Collaborations {
		usage, capability := byID[link.ID], byID[link.InterfaceID]
		if usage == nil || capability == nil || usage.Kind != "collaboration" || capability.Kind != "interface" {
			continue
		}
		owner := ""
		for _, api := range result.Design.Interfaces {
			if api.ID == link.InterfaceID {
				owner = api.ModuleID
			}
		}
		if owner == "" {
			continue
		}
		hasTarget := false
		for _, loc := range usage.Locations {
			if result.Design.ModuleAt(path.Dir(loc.File)) == owner {
				hasTarget = true
			}
		}
		if hasTarget {
			continue
		}
		for _, loc := range capability.Locations {
			if result.Design.ModuleAt(path.Dir(loc.File)) == owner {
				usage.Locations = append(usage.Locations, loc)
			}
		}
	}
}

// ValidateAnalysis 拒绝虚构目录、丢包、错误归属及越界位置；语义真实性仍由用户结合源码审阅。
func ValidateAnalysis(context source.Context, result Result) error {
	if err := context.Validate(); err != nil {
		return err
	}
	d, a := result.Design, result.Analysis
	if err := design.Validate(d); err != nil {
		return err
	}
	if strings.TrimSpace(result.Summary) == "" || a == nil || a.Evidence == nil || a.Issues == nil || a.Suggestions == nil || a.Uncertainties == nil {
		return fmt.Errorf("初次分析缺少说明、依据、问题建议或不确定项列表")
	}
	if len(d.ForbiddenDependencies) != 0 {
		return fmt.Errorf("初次分析不能自动产生禁止依赖规则")
	}
	files := map[string]source.SourceFile{}
	for _, f := range context.Facts.Files {
		files[f.Path] = f
	}
	owners, kinds := map[string]string{}, map[string]string{}
	for _, m := range d.Modules {
		found := false
		for _, f := range context.Facts.Files {
			if design.Contains(m.Root, path.Dir(f.Path)) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("模块 %s 的目录没有本次读取的源码依据", m.ID)
		}
		owners[m.ID], kinds[m.ID] = m.ID, "module"
	}
	for _, p := range context.Facts.Packages {
		if len(p.Files) > 0 && d.ModuleAt(p.Directory) == "" {
			return fmt.Errorf("分析设计遗漏了已扫描包：%s", p.ImportPath)
		}
	}
	for _, i := range d.Interfaces {
		owners[i.ID], kinds[i.ID] = i.ModuleID, "interface"
	}
	links := map[string]design.Collaboration{}
	for _, c := range d.Collaborations {
		owners[c.ID], kinds[c.ID], links[c.ID] = c.From, "collaboration", c
	}
	validateLocations := func(locations []source.Location) error {
		for _, loc := range locations {
			f, ok := files[loc.File]
			lines := strings.Split(f.Content, "\n")
			if !ok || loc.Line < 1 || loc.EndLine < loc.Line || loc.EndLine > len(lines) || loc.Column < 1 || loc.Column > len(lines[loc.Line-1])+1 {
				return fmt.Errorf("分析引用了无效源码位置：%s:%d", loc.File, loc.Line)
			}
		}
		return nil
	}
	evidenceByID := map[string]Evidence{}
	for _, e := range a.Evidence {
		evidenceByID[e.ID] = e
	}
	seen := map[string]bool{}
	for _, e := range a.Evidence {
		if kinds[e.ID] == "" || kinds[e.ID] != e.Kind || seen[e.ID] {
			return fmt.Errorf("分析依据对象不存在、重复或类型错误：%s", e.ID)
		}
		seen[e.ID] = true
		if e.Status != "supported" && e.Status != "uncertain" || strings.TrimSpace(e.Explanation) == "" || len(e.Locations) == 0 {
			return fmt.Errorf("分析依据缺少状态、解释或源码：%s", e.ID)
		}
		if err := validateLocations(e.Locations); err != nil {
			return err
		}
		ownerFound, targetFound := false, false
		for _, loc := range e.Locations {
			module := d.ModuleAt(path.Dir(loc.File))
			if module == owners[e.ID] {
				ownerFound = true
			}
			if e.Kind == "collaboration" && module == owners[links[e.ID].InterfaceID] {
				targetFound = true
			}
		}
		if !ownerFound || e.Kind == "collaboration" && !targetFound {
			return fmt.Errorf("分析依据未覆盖对象的模块归属或协作双方：%s", e.ID)
		}
		if e.Kind == "collaboration" && e.Status == "supported" && !directCapabilityUse(d, files, e, evidenceByID[links[e.ID].InterfaceID], owners[e.ID], owners[links[e.ID].InterfaceID]) {
			return fmt.Errorf("协作 %s 缺少可直接关联目标能力的代码引用；动态或间接使用应标记为不确定", e.ID)
		}
	}
	if len(seen) != len(kinds) {
		return fmt.Errorf("每个模块、接口和协作都必须提供源码依据")
	}
	for _, findings := range [][]Finding{a.Issues, a.Suggestions} {
		for _, f := range findings {
			if strings.TrimSpace(f.Description) == "" || len(f.Locations) == 0 {
				return fmt.Errorf("问题和建议必须说明内容及源码依据")
			}
			if err := validateLocations(f.Locations); err != nil {
				return err
			}
		}
	}
	for _, u := range a.Uncertainties {
		if strings.TrimSpace(u) == "" {
			return fmt.Errorf("不确定项不能是空说明")
		}
	}
	return nil
}
