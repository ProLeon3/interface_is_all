package source

// Location 使用项目相对路径和从 1 开始的行列号。
type Location struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	EndLine int    `json:"end_line"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

// BuildScope 记录实际采用的构建条件；测试代码不参与首版检查。
type BuildScope struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	CGOEnabled string `json:"cgo_enabled"`
	GoVersion  string `json:"go_version"`
	Tags       string `json:"tags"`
	Tests      bool   `json:"tests"`
}

type Package struct {
	ImportPath string   `json:"import_path"`
	Directory  string   `json:"directory"`
	ModuleID   string   `json:"module_id"`
	Files      []string `json:"files"`
	Excluded   []string `json:"excluded"`
}

// SourceFile 保留完整源码供能力审查查看私有辅助逻辑，不把名称匹配当成实现证明。
type SourceFile struct {
	Path        string `json:"path"`
	PackagePath string `json:"package_path"`
	ModuleID    string `json:"module_id"`
	Content     string `json:"content"`
}

// BuildFile 绑定模块与依赖清单，依赖配置变化同样会使旧能力审查过期。
type BuildFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Entry 是公开声明候选；包括子包和未导出接收者上的导出方法。
// 是否形成对外能力仍由语义审查结合工厂、类型及调用上下文判断。
type Entry struct {
	ID          string   `json:"id"`
	ModuleID    string   `json:"module_id"`
	PackagePath string   `json:"package_path"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Receiver    string   `json:"receiver,omitempty"`
	Code        string   `json:"code"`
	Location    Location `json:"location"`
}

type Dependency struct {
	FromModule  string   `json:"from_module"`
	ToModule    string   `json:"to_module"`
	FromPackage string   `json:"from_package"`
	ToPackage   string   `json:"to_package"`
	Location    Location `json:"location"`
}

// Facts 是观察结果。未归属包与空模块用于说明覆盖范围，不自动成为违规。
type Facts struct {
	Scope              BuildScope   `json:"scope"`
	BuildFiles         []BuildFile  `json:"build_files"`
	Packages           []Package    `json:"packages"`
	Files              []SourceFile `json:"files"`
	Entries            []Entry      `json:"entries"`
	Dependencies       []Dependency `json:"dependencies"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
	UnassignedPackages []string     `json:"unassigned_packages"`
	EmptyModules       []string     `json:"empty_modules"`
}
