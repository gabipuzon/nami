package goanalyzer

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
)

// declarationFragment records only declarations directly owned by the parsed file.
func declarationFragment(file *ast.File, rel string) (graph.Fragment, int) {
	fragment := graph.Fragment{}
	exportCount := 0
	initCount := 0
	fileID := "file:" + rel
	add := func(kind graph.NodeKind, prefix, identity, name string) {
		if name == "_" {
			return
		}
		id := prefix + ":" + rel + "#" + identity
		fragment.Nodes = append(fragment.Nodes, graph.Node{ID: id, Kind: kind, Path: rel, Name: name})
		fragment.Edges = append(fragment.Edges, graph.Edge{Kind: graph.Contains, From: fileID, To: id})
		// Each emitted exported identifier is one source declaration in this view.
		if ast.IsExported(name) || (kind == graph.Method && ast.IsExported(name[strings.LastIndex(name, ".")+1:])) {
			exportCount++
		}
	}
	for _, declaration := range file.Decls {
		switch decl := declaration.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil {
				name := decl.Name.Name
				if name == "init" {
					initCount++
					name += "@" + strconv.Itoa(initCount)
				}
				add(graph.Function, "function", name, decl.Name.Name)
				continue
			}
			if len(decl.Recv.List) == 0 {
				continue
			}
			receiver := receiverName(decl.Recv.List[0].Type)
			if receiver != "" {
				name := receiver + "." + decl.Name.Name
				add(graph.Method, "method", name, name)
			}
		case *ast.GenDecl:
			for _, specification := range decl.Specs {
				switch spec := specification.(type) {
				case *ast.TypeSpec:
					kind := graph.Type
					prefix := "type"
					if spec.Assign == 0 {
						switch spec.Type.(type) {
						case *ast.StructType:
							kind, prefix = graph.Struct, "struct"
						case *ast.InterfaceType:
							kind, prefix = graph.Interface, "interface"
						}
					}
					add(kind, prefix, spec.Name.Name, spec.Name.Name)
				case *ast.ValueSpec:
					kind, prefix := graph.Variable, "variable"
					if decl.Tok == token.CONST {
						kind, prefix = graph.Constant, "constant"
					}
					for _, name := range spec.Names {
						add(kind, prefix, name.Name, name.Name)
					}
				}
			}
		}
	}
	return fragment, exportCount
}

func receiverName(expr ast.Expr) string {
	switch receiver := expr.(type) {
	case *ast.Ident:
		return receiver.Name
	case *ast.StarExpr:
		return receiverName(receiver.X)
	case *ast.IndexExpr:
		return receiverName(receiver.X)
	case *ast.IndexListExpr:
		return receiverName(receiver.X)
	case *ast.ParenExpr:
		return receiverName(receiver.X)
	default:
		return ""
	}
}
