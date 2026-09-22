// Package check 提取 Go 代码事实，分别生成确定性依赖结果和待确认的能力审查。
package check

import (
	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/source"
)

// 事实类型由 source 统一维护，旧检查和审查协议保持原 JSON 形状。
type Location = source.Location
type Diagnostic = source.Diagnostic
type BuildScope = source.BuildScope
type Package = source.Package
type SourceFile = source.SourceFile
type BuildFile = source.BuildFile
type Entry = source.Entry
type Dependency = source.Dependency
type Facts = source.Facts

type Violation struct {
	Code       string                     `json:"code"`
	Rule       design.ForbiddenDependency `json:"rule"`
	Dependency Dependency                 `json:"dependency"`
}

type DeterministicResult struct {
	Status     string      `json:"status"`
	Violations []Violation `json:"violations"`
}

// Report 没有混合确定性与语义结果的整体通过标志。
type Report struct {
	SchemaVersion  int                 `json:"schema_version"`
	DesignRevision string              `json:"design_revision"`
	Facts          Facts               `json:"facts"`
	Deterministic  DeterministicResult `json:"deterministic"`
	SemanticStatus string              `json:"semantic_status"`
}
