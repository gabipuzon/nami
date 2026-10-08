package pythonanalyzer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
)

func fixture(t *testing.T, sources map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var files []string
	for rel, source := range sources {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, rel)
	}
	return root, files
}
func TestPythonFactsAndIsolation(t *testing.T) {
	sources := map[string]string{}
	for _, rel := range []string{"standalone.py", "users/__init__.py", "users/models.py", "users/service.py", "users/admin/__init__.py", "users/admin/service.py", "broken.py"} {
		source, err := os.ReadFile(filepath.Join("testdata", "fixture", rel))
		if err != nil {
			t.Fatal(err)
		}
		sources[rel] = string(source)
	}
	root, files := fixture(t, sources)
	first := Analyze(root, files)
	second := Analyze(root, files)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("nondeterministic analysis")
	}
	if first.FilesAnalyzed != 6 || first.FilesFailed != 1 || first.Imports.Discovered != 10 || first.Imports.InternalResolved != 5 || first.Imports.StandardLibrary != 3 || first.Imports.Unresolved != 1 || first.Imports.Unclassified != 1 {
		t.Fatalf("result: %+v", first)
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("repository source executed")
	}
	nodes := map[string]graph.Node{}
	for _, node := range first.Fragment.Nodes {
		nodes[node.ID] = node
		if node.Language != "python" {
			t.Fatalf("missing language: %+v", node)
		}
		if node.Kind == graph.File && (!node.HasImportCount || node.HasExportCount) {
			t.Fatalf("invalid counts: %+v", node)
		}
	}
	for _, id := range []string{"file:standalone.py", "python-package:users", "python-package:users/admin", "function:users/service.py#build_user", "function:users/service.py#fetch", "class:users/service.py#UserService", "method:users/service.py#UserService.create", "method:users/service.py#UserService.update"} {
		if _, ok := nodes[id]; !ok {
			t.Errorf("missing %s", id)
		}
	}
	for _, id := range []string{"file:broken.py", "function:users/service.py#nested", "class:users/service.py#Local", "class:users/service.py#Nested"} {
		if _, ok := nodes[id]; ok {
			t.Errorf("unexpected %s", id)
		}
	}
	g, err := graph.Build(first.Fragment)
	if err != nil {
		t.Fatal(err)
	}
	h, err := hierarchy.New(g)
	if err != nil {
		t.Fatal(err)
	}
	if parent, ok, err := h.Parent("python-package:users/admin"); err != nil || !ok || parent != "python-package:users" {
		t.Fatalf("nested package parent: %s %v %v", parent, ok, err)
	}
	if _, ok, err := h.Parent("file:standalone.py"); err != nil || ok {
		t.Fatal("standalone file given a synthetic parent")
	}
	projection, err := hierarchy.ProjectPackages(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Graph.Edges) != 2 {
		t.Fatalf("projection: %+v", projection)
	}
	for _, item := range projection.Evidence {
		for _, edge := range item.Sources {
			if nodes[edge.To].Kind != graph.File {
				t.Fatalf("unexpected evidence: %+v", edge)
			}
		}
	}
}
func TestAmbiguousAndUnprovenModules(t *testing.T) {
	root, files := fixture(t, map[string]string{"a/users/__init__.py": "", "b/users/__init__.py": "", "standalone.py": "import users\nimport neighbor\nimport ns.service\nfrom . import neighbor\n", "neighbor.py": "", "ns/service.py": ""})
	result := Analyze(root, files)
	if result.Imports.InternalResolved != 0 || result.Imports.Unresolved != 2 || result.Imports.Unclassified != 2 {
		t.Fatalf("result: %+v", result)
	}
}
func TestMissingRuntime(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result := Analyze(t.TempDir(), []string{"a.py", "b.py"})
	if result.FilesFailed != 2 || len(result.Fragment.Nodes) != 0 || len(result.Issues) != 1 || result.Issues[0].Kind != "PYTHON_RUNTIME_ERROR" {
		t.Fatalf("result: %+v", result)
	}
	if result := Analyze(t.TempDir(), nil); len(result.Issues) != 0 {
		t.Fatal("runtime required without Python sources")
	}
}

func TestRootClassicPackage(t *testing.T) {
	root, files := fixture(t, map[string]string{"__init__.py": "", "models.py": "class User:\n pass\n", "service.py": "from . import models\n"})
	result := Analyze(root, files)
	if result.FilesAnalyzed != 3 || result.Imports.InternalResolved != 1 || len(result.Issues) != 0 {
		t.Fatalf("result: %+v", result)
	}
	for _, node := range result.Fragment.Nodes {
		if node.Kind == graph.Package && (node.ID != "python-package:." || node.Name != filepath.Base(root)) {
			t.Fatalf("root package: %+v", node)
		}
	}
}

func TestRuntimeIsolationIgnoresPythonPath(t *testing.T) {
	root, files := fixture(t, map[string]string{"json.py": "raise RuntimeError('repository json imported')\n", "sitecustomize.py": "raise RuntimeError('repository startup hook executed')\n", "normal.py": "import json\n"})
	t.Setenv("PYTHONPATH", root)
	result := Analyze(root, files)
	if result.FilesAnalyzed != 3 || result.FilesFailed != 0 || result.Imports.StandardLibrary != 1 {
		t.Fatalf("isolation result: %+v", result)
	}
}

func TestStandardLibraryShadowIsNotGuessed(t *testing.T) {
	root, files := fixture(t, map[string]string{"json/__init__.py": "", "app.py": "import json\n"})
	result := Analyze(root, files)
	if result.Imports.InternalResolved != 0 || result.Imports.Unresolved != 1 {
		t.Fatalf("shadow result: %+v", result)
	}
}

func TestIncompleteRuntimeResponseFailsLoudly(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "python3"), []byte("#!/bin/sh\nprintf '{}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", runtimeDir)
	result := Analyze(t.TempDir(), []string{"module.py"})
	if result.FilesFailed != 1 || len(result.Issues) != 1 || result.Issues[0].Kind != "PYTHON_RUNTIME_ERROR" {
		t.Fatalf("incomplete response accepted: %+v", result)
	}
}
