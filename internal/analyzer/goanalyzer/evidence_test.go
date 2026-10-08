package goanalyzer

import (
	"github.com/gabipuzon/nami/internal/graph"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestSourceSpansUsePhysicalLinesAndEditorColumns(t *testing.T) {
	content := []byte("package p\n//line imaginary.go:100\nimport \"example.test/dep\"\n")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", content, 0)
	if err != nil {
		t.Fatal(err)
	}
	fact := sourceEvidence(sourceFile{path: "p.go", file: file, fset: fset, content: content, hash: graph.SourceHash(content)}, file.Imports[0], graph.Edge{Kind: graph.Imports})
	if fact.Line != 3 || fact.Column != 8 || fact.Snippet != "\"example.test/dep\"" {
		t.Fatal(fact)
	}
	unicode := []byte("α😀ref")
	if column := sourceColumn(unicode, strings.Index(string(unicode), "ref")); column != 4 {
		t.Fatal("UTF-16 editor column", column)
	}
}
