package source

import (
	"go/ast"
	"go/token"
)

func location(fset *token.FileSet, node ast.Node) Location {
	// 使用物理位置，避免 //line 指令把证据重定向到未扫描文件或虚构行号。
	start, end := fset.PositionFor(node.Pos(), false), fset.PositionFor(node.End(), false)
	return Location{start.Filename, start.Line, start.Column, end.Line}
}

func extractEntries(fset *token.FileSet, file *ast.File, source []byte, pkg, moduleID string) []Entry {
	entries := []Entry{}
	add := func(kind, name, receiver string, node ast.Node) {
		start, end := fset.PositionFor(node.Pos(), false).Offset, fset.PositionFor(node.End(), false).Offset
		qualified := name
		if receiver != "" {
			qualified = receiver + "." + name
		}
		entries = append(entries, Entry{
			ID: pkg + ":" + kind + ":" + qualified, ModuleID: moduleID, PackagePath: pkg,
			Kind: kind, Name: name, Receiver: receiver, Code: string(source[start:end]), Location: location(fset, node),
		})
	}
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if !declaration.Name.IsExported() {
				continue
			}
			kind, receiver := "function", ""
			if declaration.Recv != nil && len(declaration.Recv.List) > 0 {
				kind, receiver = "method", receiverName(declaration.Recv.List[0].Type)
			}
			add(kind, declaration.Name.Name, receiver, declaration)
		case *ast.GenDecl:
			for _, spec := range declaration.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					if spec.Name.IsExported() {
						add("type", spec.Name.Name, "", spec)
					}
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						if name.IsExported() {
							add(declaration.Tok.String(), name.Name, "", spec)
						}
					}
				}
			}
		}
	}
	return entries
}

// receiverName 同时处理指针接收者和带一个或多个类型参数的接收者。
func receiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.IndexExpr:
		return receiverName(expr.X)
	case *ast.IndexListExpr:
		return receiverName(expr.X)
	case *ast.ParenExpr:
		return receiverName(expr.X)
	}
	return ""
}
