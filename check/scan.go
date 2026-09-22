package check

import (
	"context"
	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/source"
	"github.com/ProLeon3/interface_is_all/store"
)

// Options 沿用公共源码扫描的构建选项，保持检查接口兼容。
type Options = source.Options

// Scan 与首次分析使用同一套读取规则；设计仅决定模块归属。
func Scan(ctx context.Context, root string, d design.Design, options Options) (Facts, error) {
	return source.Scan(ctx, root, d, options)
}

// Run 始终使用已确认快照。加载不完整时返回 incomplete，保留已经定位的违规。
func Run(ctx context.Context, root string, snapshot store.Snapshot, options Options) (Report, error) {
	if err := snapshot.Validate(); err != nil {
		return Report{}, err
	}
	facts, err := Scan(ctx, root, snapshot.Design, options)
	if err != nil {
		return Report{}, err
	}
	result := DeterministicResult{Status: "passed", Violations: []Violation{}}
	rules := map[string]design.ForbiddenDependency{}
	for _, rule := range snapshot.Design.ForbiddenDependencies {
		rules[rule.From+"\x00"+rule.To] = rule
	}
	for _, dependency := range facts.Dependencies {
		if rule, ok := rules[dependency.FromModule+"\x00"+dependency.ToModule]; ok {
			result.Violations = append(result.Violations, Violation{"forbidden_dependency", rule, dependency})
		}
	}
	if len(result.Violations) > 0 {
		result.Status = "violations"
	}
	if len(facts.Diagnostics) > 0 {
		result.Status = "incomplete"
	}
	return Report{1, snapshot.Revision, facts, result, "not_run"}, nil
}
