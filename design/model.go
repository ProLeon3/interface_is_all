// Package design 表达用户确认的模块职责和接口能力，不绑定具体 Go 声明。
package design

// Design 只保存设计约束；布局、实现关联及扫描报告均单独保存。
type Design struct {
	SchemaVersion         int                   `json:"schema_version"`
	Modules               []Module              `json:"modules"`
	Interfaces            []Interface           `json:"interfaces"`
	ForbiddenDependencies []ForbiddenDependency `json:"forbidden_dependencies"`
	// 可选扩展：空协作不参与序列化，保持旧设计和历史快照的指纹不变。
	Collaborations []Collaboration `json:"collaborations,omitempty"`
}

// Collaboration 表达模块使用另一模块的具体能力，不构成调用方白名单。
// 目标模块由 InterfaceID 的归属确定，迁移接口时无须维护重复的目标字段。
type Collaboration struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	InterfaceID string `json:"interface_id"`
	Purpose     string `json:"purpose"`
}

// Module 独占以 Root 为根的项目相对目录子树。
type Module struct {
	ID             string `json:"id"`
	Root           string `json:"root"`
	Responsibility string `json:"responsibility"`
}

// Interface 是一项对外能力，可以关联多个函数、方法等入口。
type Interface struct {
	ID          string     `json:"id"`
	ModuleID    string     `json:"module_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Semantics   *Semantics `json:"semantics,omitempty"`
}

// Semantics 用自然语言补充交互含义，不要求逐字段声明 Go 类型。
type Semantics struct {
	Inputs  string `json:"inputs,omitempty"`
	Outputs string `json:"outputs,omitempty"`
	Errors  string `json:"errors,omitempty"`
}

// ForbiddenDependency 只禁止 From 到 To 的直接包引用，未禁止的方向默认允许。
type ForbiddenDependency struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}
