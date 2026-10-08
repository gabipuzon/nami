package analysis

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func writeIgnoreFixture(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIgnoredGoSourcesPreserveKnownFactsAndReportMissingTargets(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFixture(t, root, map[string]string{
		"go.mod": "module example.com/scoped\ngo 1.25.0\n", ".namiignore": "drop/\nkept/hidden.go\n",
		"main.go":      "package main\nimport (\"example.com/scoped/kept\";\"example.com/scoped/drop\")\nvar _ = kept.Name\nvar _ = drop.Name\n",
		"kept/kept.go": "package kept\nvar Name = 1\n", "kept/hidden.go": "this source would fail to parse",
		"drop/drop.go": "package drop\nvar Name = 1\n",
	})
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exclusions) != 2 || result.Coverage.FilesAnalyzed != 2 || result.Coverage.FilesFailed != 0 || result.Coverage.FilesSkipped != 0 || result.Coverage.FilesDiscovered != 3 || result.Coverage.Unresolved != 1 || result.Coverage.InternalResolved != 1 {
		t.Fatal(result)
	}
	for _, node := range result.Graph.Nodes {
		if node.Path == "drop" || node.Path == "drop/drop.go" || node.Path == "kept/hidden.go" {
			t.Fatal("excluded node", node)
		}
	}
	if !containsEdge(result.Graph.Edges, graph.Edge{Kind: graph.Imports, From: "file:main.go", To: "package:kept#kept"}) {
		t.Fatal("valid import was lost")
	}
	if len(result.Issues) != 1 || result.Issues[0].Kind != "UNRESOLVED_IMPORT" || result.Issues[0].Import != "example.com/scoped/drop" {
		t.Fatal(result.Issues)
	}
	second, err := Map(root)
	if err != nil || !reflect.DeepEqual(result, second) {
		t.Fatal("unstable exclusion analysis", err)
	}
}

func TestIgnoredPythonModulesRemainAnalysisGaps(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFixture(t, root, map[string]string{".namiignore": "pkg/hidden.py\n", "pkg/__init__.py": "", "pkg/hidden.py": "broken Python !!!", "pkg/kept.py": "from . import hidden\nfrom . import other\n", "pkg/other.py": "def run(): pass\n"})
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exclusions) != 1 || result.Coverage.FilesAnalyzed != 3 || result.Coverage.FilesFailed != 0 || result.Coverage.Unresolved != 1 || result.Coverage.InternalResolved != 1 {
		t.Fatal(result)
	}
	for _, node := range result.Graph.Nodes {
		if node.Path == "pkg/hidden.py" {
			t.Fatal(node)
		}
	}
	if len(result.Issues) != 1 || result.Issues[0].Kind != "UNRESOLVED_IMPORT" {
		t.Fatal(result.Issues)
	}
	if !containsEdge(result.Graph.Edges, graph.Edge{Kind: graph.Imports, From: "file:pkg/kept.py", To: "file:pkg/other.py"}) {
		t.Fatal("valid Python import was lost")
	}
}

func TestIntentionalExclusionsAreNotFailuresOrGaps(t *testing.T) {
	root := t.TempDir()
	writeIgnoreFixture(t, root, map[string]string{".namiignore": "broken.go\nunsupported.ts\n", "go.mod": "module example.com/scoped\ngo 1.25.0\n", "kept.go": "package scoped\n", "broken.go": "!!!", "unsupported.ts": "ignored"})
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage.Status != "complete" || result.Coverage.FilesDiscovered != 2 || result.Coverage.FilesAnalyzed != 1 || result.Coverage.FilesSkipped != 0 || result.Coverage.FilesFailed != 0 || len(result.Issues) != 0 || len(result.Exclusions) != 2 {
		t.Fatal(result)
	}
}
