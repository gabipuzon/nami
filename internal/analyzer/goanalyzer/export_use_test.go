package goanalyzer

import (
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestExportUseRequiresImportBindingAndUniqueProvider(t *testing.T) {
	parse := func(source string) []graph.Edge {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "consumer.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		uses := typeCheckImportUses(file, fset, map[string]string{"example.com/dep": "dep"})
		edges, _ := exportUseEdges(file, "consumer.go", "dep", "example.com/dep", uses, map[string][]string{"Name": {"file:provider.go"}})
		return edges
	}
	want := []graph.Edge{{Kind: graph.UsesExport, From: "file:consumer.go", To: "file:provider.go"}}
	if got := parse("package consumer\nimport dep \"example.com/dep\"\nvar Value = dep.Name\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("qualified import use = %+v", got)
	}
	if got := parse("package consumer\nimport dep \"example.com/dep\"\nfunc f() { dep := struct{ Name string }{}; _ = dep.Name }\n"); len(got) != 0 {
		t.Fatalf("shadowed import created export-use evidence: %+v", got)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "consumer.go", "package consumer\nimport dep \"example.com/dep\"\nvar Value = dep.Name\n", parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	uses := typeCheckImportUses(file, fset, map[string]string{"example.com/dep": "dep"})
	if got, unresolved := exportUseEdges(file, "consumer.go", "dep", "example.com/dep", uses, map[string][]string{"Name": {"file:first.go", "file:second.go"}}); len(got) != 0 || !reflect.DeepEqual(unresolved, []string{"Name"}) {
		t.Fatalf("ambiguous export created evidence: %+v, unresolved=%+v", got, unresolved)
	}
}
