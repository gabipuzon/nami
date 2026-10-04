package analysis

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"nami/internal/graph"
)

func TestMapFixture(t *testing.T) {
	root := filepath.Join("testdata", "fixture")
	first, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("unchanged repository produced different results")
	}

	var packages, files, contains, imports int
	for _, node := range first.Graph.Nodes {
		switch node.Kind {
		case graph.Package:
			packages++
		case graph.File:
			files++
		}
		if strings.Contains(node.ID, "ignored") {
			t.Fatalf("build output entered graph: %s", node.ID)
		}
	}
	for _, edge := range first.Graph.Edges {
		switch edge.Kind {
		case graph.Contains:
			contains++
		case graph.Imports:
			imports++
		}
	}
	if packages != 4 || files != 5 || contains != 5 || imports != 5 {
		t.Fatalf("packages=%d files=%d contains=%d imports=%d", packages, files, contains, imports)
	}
	wantEdges := []graph.Edge{
		{Kind: graph.Contains, From: "package:.#main", To: "file:main.go"},
		{Kind: graph.Contains, From: "package:alpha#alpha", To: "file:alpha/a.go"},
		{Kind: graph.Contains, From: "package:alpha#alpha", To: "file:alpha/second.go"},
		{Kind: graph.Contains, From: "package:beta#beta", To: "file:beta/b.go"},
		{Kind: graph.Contains, From: "package:gamma#gamma", To: "file:gamma/g.go"},
		{Kind: graph.Imports, From: "file:main.go", To: "package:alpha#alpha"},
		{Kind: graph.Imports, From: "file:main.go", To: "package:gamma#gamma"},
		{Kind: graph.Imports, From: "file:alpha/a.go", To: "package:beta#beta"},
		{Kind: graph.Imports, From: "file:alpha/second.go", To: "package:beta#beta"},
		{Kind: graph.Imports, From: "file:beta/b.go", To: "package:gamma#gamma"},
	}
	for _, want := range wantEdges {
		if !containsEdge(first.Graph.Edges, want) {
			t.Errorf("missing edge %+v", want)
		}
	}
	if len(first.Issues) != 2 || first.Issues[0].Kind != "UNRESOLVED_IMPORT" || first.Issues[0].Import != "example.com/fixture/missing" || first.Issues[1].Kind != "UNSUPPORTED_FILE" || first.Issues[1].Path != "notes.py" {
		t.Fatalf("issues = %+v", first.Issues)
	}
}

func TestMapKeepsValidFilesAfterParseFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/partial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "good.go"), []byte("package good\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bad.go"), []byte("package\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || len(result.Issues) != 1 || result.Issues[0].Kind != "SKIPPED_FILE" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMapMissingDirectory(t *testing.T) {
	if _, err := Map(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory returned no error")
	}
}

func TestMapWithoutModuleReportsResolutionLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || len(result.Issues) != 1 || result.Issues[0].Kind != "MODULE_ERROR" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMapReportsSkippedGoSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/symlink\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "good.go"), []byte("package good\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("good.go", filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || len(result.Issues) != 1 || result.Issues[0].Path != "linked.go" || result.Issues[0].Kind != "SKIPPED_FILE" {
		t.Fatalf("result = %+v", result)
	}
}

func containsEdge(edges []graph.Edge, target graph.Edge) bool {
	for _, edge := range edges {
		if edge == target {
			return true
		}
	}
	return false
}
