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
	if len(first.Issues) != 3 || first.Issues[0].Kind != "UNCLASSIFIED_IMPORT" || first.Issues[0].Import != "github.com/external/thing" || first.Issues[1].Kind != "UNRESOLVED_IMPORT" || first.Issues[1].Import != "example.com/fixture/missing" || first.Issues[2].Kind != "UNSUPPORTED_FILE" || first.Issues[2].Path != "notes.py" {
		t.Fatalf("issues = %+v", first.Issues)
	}
	wantCoverage := Coverage{
		Status: "completed_with_gaps", FilesDiscovered: 8, SupportedSourceFiles: 5,
		FilesAnalyzed: 5, FilesSkipped: 1, FilesFailed: 0,
		ImportsDiscovered: 8, InternalResolved: 5, StandardLibrary: 1,
		Unresolved: 1, Unclassified: 1,
	}
	if first.Coverage != wantCoverage {
		t.Fatalf("coverage = %+v, want %+v", first.Coverage, wantCoverage)
	}
}

func TestMapCompleteCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "lib"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"go.mod":     "module example.com/clean\nrequire github.com/external/thing v1.0.0\n",
		"main.go":    "package main\nimport (\"fmt\"; \"example.com/clean/lib\"; \"github.com/external/thing\")\n",
		"lib/lib.go": "package lib\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	want := Coverage{
		Status: "complete", FilesDiscovered: 3, SupportedSourceFiles: 2,
		FilesAnalyzed: 2, ImportsDiscovered: 3, InternalResolved: 1,
		StandardLibrary: 1, External: 1,
	}
	if result.Coverage != want || len(result.Issues) != 0 {
		t.Fatalf("coverage = %+v, issues = %+v", result.Coverage, result.Issues)
	}
}

func TestReplacedDependencyIsNotClassifiedExternal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/replaced\nrequire github.com/external/thing v1.0.0\nreplace github.com/external/thing => ./local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport \"github.com/external/thing\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.External != 0 || result.Coverage.Unclassified != 1 || len(result.Issues) != 1 || result.Issues[0].Kind != "UNCLASSIFIED_IMPORT" {
		t.Fatalf("coverage = %+v, issues = %+v", result.Coverage, result.Issues)
	}
}

func TestWorkspaceDependencyIsNotClassifiedExternal(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.com/workspace\nrequire github.com/external/thing v1.0.0\n",
		"go.work": "go 1.25.0\nuse .\n",
		"main.go": "package main\nimport \"github.com/external/thing\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.External != 0 || result.Coverage.Unclassified != 1 || len(result.Issues) != 1 || result.Issues[0].Kind != "UNCLASSIFIED_IMPORT" {
		t.Fatalf("coverage = %+v, issues = %+v", result.Coverage, result.Issues)
	}
}

func TestUndeclaredImportsAreUnclassifiedRegardlessOfPathShape(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/unknown\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport (\"github.com/unknown/pkg\"; \"mystery/pkg\")\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.External != 0 || result.Coverage.Unresolved != 0 || result.Coverage.Unclassified != 2 || len(result.Issues) != 2 {
		t.Fatalf("coverage = %+v, issues = %+v", result.Coverage, result.Issues)
	}
	for _, issue := range result.Issues {
		if issue.Kind != "UNCLASSIFIED_IMPORT" || issue.Reason == "" {
			t.Fatalf("issue = %+v", issue)
		}
	}
}

func TestMapKeepsValidFilesAfterParseFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/partial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"good", "broken"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "good", "good.go"), []byte("package good\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken", "bad.go"), []byte("package\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || len(result.Issues) != 1 || result.Issues[0].Kind != "FAILED_FILE" || result.Issues[0].Path != "broken/bad.go" {
		t.Fatalf("result = %+v", result)
	}
	if result.Coverage.Status != "completed_with_gaps" || result.Coverage.FilesAnalyzed != 1 || result.Coverage.FilesFailed != 1 || result.Coverage.FilesSkipped != 0 {
		t.Fatalf("coverage = %+v", result.Coverage)
	}
}

func TestMapMissingDirectory(t *testing.T) {
	if _, err := Map(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory returned no error")
	}
}

func TestMapWithoutModuleReportsResolutionLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nimport (\"fmt\"; \"example.com/unknown\")\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Graph.Nodes) != 2 || len(result.Issues) != 2 || result.Issues[0].Kind != "MODULE_ERROR" || result.Issues[1].Kind != "UNCLASSIFIED_IMPORT" {
		t.Fatalf("result = %+v", result)
	}
	if result.Coverage.ImportsDiscovered != 2 || result.Coverage.StandardLibrary != 1 || result.Coverage.Unclassified != 1 {
		t.Fatalf("coverage = %+v", result.Coverage)
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
	if result.Coverage.FilesDiscovered != 3 || result.Coverage.SupportedSourceFiles != 2 || result.Coverage.FilesSkipped != 1 || result.Coverage.FilesAnalyzed != 1 {
		t.Fatalf("coverage = %+v", result.Coverage)
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
