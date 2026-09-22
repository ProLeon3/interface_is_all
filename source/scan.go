// Package source 采集 Go 源码事实，不要求设计基准，也不保存设计。
package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"interfaceisall/design"
	"interfaceisall/internal/projectpath"
)

type Options struct {
	Tags string
}

// goPackage 只读取 go list 的事实，不将依赖路径前缀等同于模块目录。
type goPackage struct {
	Dir, ImportPath, Name                              string
	GoFiles, CgoFiles, IgnoredGoFiles                  []string
	TestGoFiles, XTestGoFiles, SwigFiles, SwigCXXFiles []string
	ImportMap                                          map[string]string
	Incomplete                                         bool
	Error                                              *struct{ Err string }
	DepsErrors                                         []struct{ Err string }
}

// Scan 使用当前 Go 工具链的构建选择和 AST；不执行目标程序、测试或 go generate。
func Scan(ctx context.Context, root string, d design.Design, options Options) (Facts, error) {
	root, err := projectpath.Root(root)
	if err != nil {
		return Facts{}, err
	}
	if err := design.ValidateProject(root, d); err != nil {
		return Facts{}, err
	}
	gomod, err := projectpath.Resolve(root, "go.mod")
	if err != nil {
		return Facts{}, err
	}
	if _, err := os.Stat(gomod); err != nil {
		return Facts{}, fmt.Errorf("首版扫描要求项目根目录存在 go.mod：%w", err)
	}
	facts := Facts{
		Packages: []Package{}, Files: []SourceFile{}, Entries: []Entry{}, Dependencies: []Dependency{},
		Diagnostics: []Diagnostic{}, UnassignedPackages: []string{}, EmptyModules: []string{},
	}
	facts.BuildFiles, err = readBuildFiles(root)
	if err != nil {
		return Facts{}, err
	}
	envData, err := goCommand(ctx, root, "env", "-json", "GOOS", "GOARCH", "CGO_ENABLED", "GOVERSION")
	if err != nil {
		return Facts{}, err
	}
	var buildEnv map[string]string
	if err := json.Unmarshal(envData, &buildEnv); err != nil {
		return Facts{}, err
	}
	facts.Scope = BuildScope{buildEnv["GOOS"], buildEnv["GOARCH"], buildEnv["CGO_ENABLED"], buildEnv["GOVERSION"], options.Tags, false}
	mode := "-mod=readonly"
	if _, err := os.Stat(filepath.Join(root, "vendor/modules.txt")); err == nil {
		mode = "-mod=vendor"
	}
	// 静态事实不需要可执行文件的 VCS 标记；导出副本或受限的 .git 不应阻断架构扫描。
	output, err := goCommand(ctx, root, "list", "-e", "-deps", "-json", "-buildvcs=false", mode, "-tags="+options.Tags, "./...")
	if err != nil {
		return Facts{}, err
	}
	packages := map[string]goPackage{}
	decoder := json.NewDecoder(bytes.NewReader(output))
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return Facts{}, fmt.Errorf("解析 Go 包信息失败：%w", err)
		}
		packages[pkg.ImportPath] = pkg
		if pkg.Error != nil {
			facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"package_load", pkg.ImportPath, pkg.Error.Err})
		} else if pkg.Incomplete {
			facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"package_load", pkg.ImportPath, "包或其依赖加载不完整"})
		}
	}
	paths := make([]string, 0, len(packages))
	for importPath := range packages {
		paths = append(paths, importPath)
	}
	sort.Strings(paths)
	seenModules := map[string]bool{}
	fset := token.NewFileSet()
	for _, importPath := range paths {
		pkg := packages[importPath]
		if pkg.Dir == "" {
			continue
		}
		relative, inside := projectpath.Relative(root, pkg.Dir)
		if !inside || relative == "vendor" || strings.HasPrefix(relative, "vendor/") {
			continue
		}
		moduleID := d.ModuleAt(relative)
		if moduleID == "" {
			facts.UnassignedPackages = append(facts.UnassignedPackages, importPath)
		} else {
			seenModules[moduleID] = true
		}
		files := append(append([]string{}, pkg.GoFiles...), pkg.CgoFiles...)
		sort.Strings(files)
		excluded := append(append(append([]string{}, pkg.IgnoredGoFiles...), pkg.TestGoFiles...), pkg.XTestGoFiles...)
		sort.Strings(excluded)
		facts.Packages = append(facts.Packages, Package{importPath, relative, moduleID, files, excluded})
		if len(pkg.SwigFiles)+len(pkg.SwigCXXFiles) > 0 {
			facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"unsupported_swig", relative, "尚不支持 SWIG 生成入口，扫描不完整"})
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return Facts{}, err
			}
			filePath := filepath.ToSlash(filepath.Join(relative, file))
			absolute, err := projectpath.Resolve(root, filePath)
			if err != nil {
				facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"source_read", filePath, err.Error()})
				continue
			}
			content, err := os.ReadFile(absolute)
			if err != nil {
				facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"source_read", filePath, err.Error()})
				continue
			}
			facts.Files = append(facts.Files, SourceFile{filePath, importPath, moduleID, string(content)})
			syntax, err := parser.ParseFile(fset, filePath, content, parser.ParseComments|parser.AllErrors)
			if err != nil {
				facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"source_parse", filePath, err.Error()})
				continue // 部分 AST 的位置不保证完整，不用它生成可引用的代码证据。
			}
			if syntax == nil {
				continue
			}
			facts.Entries = append(facts.Entries, extractEntries(fset, syntax, content, importPath, moduleID)...)
			for _, spec := range syntax.Imports {
				imported, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					continue // 语法错误已由解析诊断记录。
				}
				if remapped, ok := pkg.ImportMap[imported]; ok {
					imported = remapped
				}
				target, ok := packages[imported]
				if !ok || target.Dir == "" {
					continue
				}
				targetDir, inside := projectpath.Relative(root, target.Dir)
				if !inside || targetDir == "vendor" || strings.HasPrefix(targetDir, "vendor/") {
					continue
				}
				targetModule := d.ModuleAt(targetDir)
				// 首次接入尚无模块映射，仍保留项目内包依赖供现状分析使用。
				if len(d.Modules) == 0 || moduleID != "" && targetModule != "" && moduleID != targetModule {
					facts.Dependencies = append(facts.Dependencies, Dependency{moduleID, targetModule, importPath, imported, location(fset, spec)})
				}
			}
		}
	}
	for _, m := range d.Modules {
		if !seenModules[m.ID] {
			facts.EmptyModules = append(facts.EmptyModules, m.ID)
		}
	}
	// go list ./... 不遍历嵌套 Go module；显式报告，避免把漏扫理解为无违规。
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == root {
			return nil
		}
		if entry.IsDir() && (strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "vendor" || entry.Name() == "testdata") {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			if _, err := os.Stat(filepath.Join(name, "go.mod")); err == nil {
				relative, _ := projectpath.Relative(root, name)
				facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"nested_module", relative, "首版仅支持单个 Go module，请将嵌套项目单独检查"})
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"directory_read", ".", err.Error()})
	}
	latestBuildFiles, err := readBuildFiles(root)
	if err != nil {
		facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"build_inputs_read", ".", err.Error()})
	} else {
		before, _ := json.Marshal(facts.BuildFiles)
		after, _ := json.Marshal(latestBuildFiles)
		if !bytes.Equal(before, after) {
			facts.Diagnostics = append(facts.Diagnostics, Diagnostic{"build_inputs_changed", ".", "扫描期间模块或依赖清单发生变化，请重新检查"})
		}
	}
	sort.Slice(facts.Diagnostics, func(i, j int) bool {
		a, b := facts.Diagnostics[i], facts.Diagnostics[j]
		return a.Subject+a.Code+a.Message < b.Subject+b.Code+b.Message
	})
	return facts, nil
}

func readBuildFiles(root string) ([]BuildFile, error) {
	files := []BuildFile{}
	for _, relative := range []string{"go.mod", "go.sum", "vendor/modules.txt"} {
		name, err := projectpath.Resolve(root, relative)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(name)
		if os.IsNotExist(err) && relative != "go.mod" {
			continue
		}
		if err != nil {
			return nil, err
		}
		files = append(files, BuildFile{relative, string(content)})
	}
	return files, nil
}

func goCommand(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.WaitDelay = time.Second
	cmd.Dir = root
	// 固定为用户选定的单模块；构建标签由 Options 显式传入并记入报告。
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GOWORK=") && !strings.HasPrefix(variable, "GOFLAGS=") {
			cmd.Env = append(cmd.Env, variable)
		}
	}
	cmd.Env = append(cmd.Env, "GOWORK=off", "GOFLAGS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("执行 go %s 失败：%w；%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}
