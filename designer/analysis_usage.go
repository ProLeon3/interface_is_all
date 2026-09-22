package designer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/source"
)

// directCapabilityUse 核对可静态连接到接口依据的包成员使用，不证明运行时行为。
// 方法接收者、注入及反射缺少类型信息时必须标 uncertain，不能用无关函数充当依据。
func directCapabilityUse(d design.Design, files map[string]source.SourceFile, usage, capability Evidence, caller, targetOwner string) bool {
	type member struct{ packagePath, packageName, name string }
	var members []member
	overlaps := func(fset *token.FileSet, node ast.Node, loc source.Location) bool {
		return fset.PositionFor(node.Pos(), false).Line <= loc.EndLine && fset.PositionFor(node.End(), false).Line >= loc.Line
	}
	for _, loc := range capability.Locations {
		file, ok := files[loc.File]
		if !ok || d.ModuleAt(path.Dir(file.Path)) != targetOwner {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file.Path, file.Content, 0)
		if err != nil {
			continue
		}
		add := func(name string, node ast.Node) {
			if !ast.IsExported(name) || !overlaps(fset, node, loc) {
				return
			}
			// 协作目标也须落在该能力声明中，不能只引用目标模块的无关文件。
			for _, target := range usage.Locations {
				if target.File == loc.File && overlaps(fset, node, target) {
					members = append(members, member{file.PackagePath, parsed.Name.Name, name})
					break
				}
			}
		}
		for _, decl := range parsed.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				if decl.Recv == nil {
					add(decl.Name.Name, decl)
				}
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							add(name.Name, spec)
						}
					case *ast.TypeSpec:
						add(spec.Name.Name, spec)
					}
				}
			}
		}
	}
	for _, loc := range usage.Locations {
		file := files[loc.File]
		if d.ModuleAt(path.Dir(file.Path)) != caller {
			continue
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, file.Path, file.Content, 0)
		if err != nil {
			continue
		}
		imports := map[string]string{}
		for _, spec := range parsed.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			for _, target := range members {
				if target.packagePath == "" || importPath != target.packagePath {
					continue
				}
				name := target.packageName
				if spec.Name != nil {
					name = spec.Name.Name
				}
				if name != "." && name != "_" {
					imports[name] = importPath
				}
			}
		}
		found := false
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !overlaps(fset, selector, loc) {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			// 本地对象绑定排除明显的同名变量遮蔽，不猜测动态接收者的类型。
			if !ok || qualifier.Obj != nil || imports[qualifier.Name] == "" {
				return true
			}
			for _, target := range members {
				if imports[qualifier.Name] == target.packagePath && selector.Sel.Name == target.name {
					found = true
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}
