package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
)

func TestHelp(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%q) exit code = %d, want 0", args, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") || stderr.Len() != 0 {
			t.Fatalf("run(%q) stdout = %q, stderr = %q", args, stdout.String(), stderr.String())
		}
	}
}

func TestMapRequiresDirectory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"map"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(map) exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage: nami map <directory>") {
		t.Fatalf("run(map) stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestMapReportsCoverageDeterministically(t *testing.T) {
	source := filepath.Join("..", "..", "internal", "analysis", "testdata", "fixture")
	root := t.TempDir()
	err := filepath.WalkDir(source, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".nami" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(source, sourcePath)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var first, second, stderr bytes.Buffer
	if code := run([]string{"map", root}, &first, &stderr); code != 0 {
		t.Fatalf("first map exit code = %d, stderr = %q", code, stderr.String())
	}
	if code := run([]string{"map", root}, &second, &stderr); code != 0 {
		t.Fatalf("second map exit code = %d, stderr = %q", code, stderr.String())
	}
	firstLines := strings.SplitN(first.String(), "\n", 2)
	secondLines := strings.SplitN(second.String(), "\n", 2)
	if len(firstLines) != 2 || len(secondLines) != 2 || !strings.HasPrefix(firstLines[0], "SAVED_SCAN id=") || !strings.HasPrefix(secondLines[0], "SAVED_SCAN id=") || firstLines[0] == secondLines[0] || firstLines[1] != secondLines[1] {
		t.Fatal("unchanged repository produced different analysis output or reused a scan ID")
	}
	for _, want := range []string{
		"COVERAGE status=completed_with_gaps",
		"files_discovered=8 supported_source_files=6 files_analyzed=6 files_skipped=0 files_failed=0",
		"imports_discovered=8 internal_imports_resolved=5 standard_library_imports=1 external_imports=0 unresolved_imports=1 cgo_imports=0 unclassified_imports=1",
		"UNCLASSIFIED_IMPORT main.go \"github.com/external/thing\"",
		"UNRESOLVED_IMPORT alpha/second.go",
		"FILE file:notes.py notes.py",
	} {
		if !strings.Contains(first.String(), want) {
			t.Fatalf("map output missing %q:\n%s", want, first.String())
		}
	}
}

func TestMapMissingDirectoryFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"map", filepath.Join(t.TempDir(), "missing")}, &stdout, &stderr); code != 1 {
		t.Fatalf("map exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "no such file or directory") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestSavedScanCommandsReadStoredResult(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.com/cli\ngo 1.25.0\n",
		"main.go": "package main\nimport \"fmt\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mapped, listed, shown, stderr bytes.Buffer
	if code := run([]string{"map", root}, &mapped, &stderr); code != 0 {
		t.Fatalf("map exit code = %d, stderr = %q", code, stderr.String())
	}
	firstLine, original, ok := strings.Cut(mapped.String(), "\n")
	if !ok || !strings.HasPrefix(firstLine, "SAVED_SCAN id=") {
		t.Fatalf("map output = %q", mapped.String())
	}
	id := strings.Fields(strings.TrimPrefix(firstLine, "SAVED_SCAN id="))[0]
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"scans", root}, &listed, &stderr); code != 0 || !strings.Contains(listed.String(), id) {
		t.Fatalf("scans exit code = %d, stdout = %q, stderr = %q", code, listed.String(), stderr.String())
	}
	if code := run([]string{"show", root, id}, &shown, &stderr); code != 0 {
		t.Fatalf("show exit code = %d, stderr = %q", code, stderr.String())
	}
	storedLine, storedResult, ok := strings.Cut(shown.String(), "\n")
	if !ok || !strings.HasPrefix(storedLine, "STORED_SCAN id="+id) || storedResult != original {
		t.Fatalf("stored result changed: %q", shown.String())
	}
}

func TestQueryCommandsUseRequestedSnapshot(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	nodes := []graph.Node{{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}, {ID: "C", Kind: graph.Package}}
	first, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph:    graph.Graph{Nodes: nodes, Edges: []graph.Edge{{Kind: graph.Imports, From: "A", To: "B"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph:    graph.Graph{Nodes: nodes, Edges: []graph.Edge{{Kind: graph.Imports, From: "A", To: "C"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	for _, test := range []struct {
		args []string
		want string
		code int
	}{
		{[]string{"dependencies", root, first.ID, "A"}, "DEPENDENCY B\n", 0},
		{[]string{"dependencies", root, second.ID, "A"}, "DEPENDENCY C\n", 0},
		{[]string{"dependents", root, first.ID, "B"}, "DEPENDENT A\n", 0},
		{[]string{"path", root, first.ID, "A", "B"}, "PATH A\nPATH B\n", 0},
		{[]string{"path", root, second.ID, "A", "B"}, "NO_PATH\n", 0},
		{[]string{"dependencies", root, first.ID, "B"}, "dependencies: none\n", 0},
		{[]string{"dependencies", root, first.ID, "missing"}, `node "missing" not found`, 1},
	} {
		var stdout, stderr bytes.Buffer
		code := run(test.args, &stdout, &stderr)
		if code != test.code {
			t.Fatalf("run(%q) = %d: %q", test.args, code, stderr.String())
		}
		got := stdout.String()
		if code != 0 {
			got = stderr.String()
		}
		if !strings.Contains(got, test.want) || (code == 0 && got != test.want) {
			t.Fatalf("run(%q) = %q, want %q", test.args, got, test.want)
		}
	}
}

func TestPackagesCommandFoldsStoredFileImports(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: "package:A", Kind: graph.Package}, {ID: "package:B", Kind: graph.Package},
				{ID: "file:a.go", Kind: graph.File},
			},
			Edges: []graph.Edge{
				{Kind: graph.Contains, From: "package:A", To: "file:a.go"},
				{Kind: graph.Imports, From: "file:a.go", To: "package:B"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"packages", root, summary.ID}, &stdout, &stderr); code != 0 {
		t.Fatalf("packages exit code = %d, stderr = %q", code, stderr.String())
	}
	want := "PACKAGE package:A\nPACKAGE package:B\nIMPORTS package:A -> package:B\n"
	if stdout.String() != want {
		t.Fatalf("packages output = %q, want %q", stdout.String(), want)
	}
	store, err = storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.Load(summary.ID)
	if err != nil || len(loaded.Result.Graph.Edges) != 2 {
		t.Fatalf("projection changed stored edges: %+v, %v", loaded.Result.Graph.Edges, err)
	}
}

func TestSymbolsReadsSavedDeclarations(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":  "module example.com/symbols\ngo 1.25.0\n",
		"main.go": "package main\nfunc Run() {}\nfunc init() {}\nfunc init() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var mapped, stderr bytes.Buffer
	if code := run([]string{"map", root}, &mapped, &stderr); code != 0 {
		t.Fatalf("map = %d: %s", code, stderr.String())
	}
	firstLine, _, _ := strings.Cut(mapped.String(), "\n")
	id := strings.Fields(strings.TrimPrefix(firstLine, "SAVED_SCAN id="))[0]
	if err := os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	var symbols bytes.Buffer
	if code := run([]string{"symbols", root, id, "file:main.go"}, &symbols, &stderr); code != 0 {
		t.Fatalf("symbols = %d: %s", code, stderr.String())
	}
	want := "SYMBOL FUNCTION function:main.go#Run Run\nSYMBOL FUNCTION function:main.go#init@1 init\nSYMBOL FUNCTION function:main.go#init@2 init\n"
	if symbols.String() != want {
		t.Fatalf("symbols = %q, want %q", symbols.String(), want)
	}
	for _, test := range []struct{ node, want string }{
		{"missing", `node "missing" not found`},
		{"package:.#main", `is not a FILE`},
	} {
		var out, errOut bytes.Buffer
		if code := run([]string{"symbols", root, id, test.node}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), test.want) {
			t.Fatalf("symbols(%s) = %d, %q", test.node, code, errOut.String())
		}
	}
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Save(root, analysis.Result{Coverage: analysis.Coverage{Status: "complete"}, Graph: graph.Graph{Nodes: []graph.Node{{ID: "file:old.go", Kind: graph.File}}}})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	var oldSymbols, oldErr bytes.Buffer
	if code := run([]string{"symbols", root, old.ID, "file:old.go"}, &oldSymbols, &oldErr); code != 0 || oldSymbols.Len() != 0 {
		t.Fatalf("old snapshot symbols = %d, %q, %q", code, oldSymbols.String(), oldErr.String())
	}
}

func TestImpactUsesStoredPackageProjectionAndCoverage(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	if err := os.WriteFile(source, []byte("package example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	nodes := []graph.Node{
		{ID: "package:A", Kind: graph.Package}, {ID: "package:B", Kind: graph.Package},
		{ID: "package:C", Kind: graph.Package}, {ID: "file:a.go", Kind: graph.File},
		{ID: "file:c.go", Kind: graph.File}, {ID: "function:a.go#Run", Kind: graph.Function},
	}
	containment := []graph.Edge{
		{Kind: graph.Contains, From: "package:A", To: "file:a.go"},
		{Kind: graph.Contains, From: "package:C", To: "file:c.go"},
		{Kind: graph.Contains, From: "file:a.go", To: "function:a.go#Run"},
	}
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "completed_with_gaps"},
		Graph: graph.Graph{Nodes: nodes, Edges: append(append([]graph.Edge(nil), containment...),
			graph.Edge{Kind: graph.Imports, From: "file:a.go", To: "package:B"},
			graph.Edge{Kind: graph.Imports, From: "file:c.go", To: "package:A"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Save(root, analysis.Result{
		Coverage: analysis.Coverage{Status: "complete"},
		Graph: graph.Graph{Nodes: nodes, Edges: append(append([]graph.Edge(nil), containment...),
			graph.Edge{Kind: graph.Imports, From: "file:c.go", To: "package:B"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ scanID, target, want string }{
		{first.ID, "package:B", "IMPACT_TARGET package:B\nAFFECTED distance=1 package:A\nAFFECTED distance=2 package:C\nIMPACT_COVERAGE status=completed_with_gaps\nIMPACT_WARNING stored scan has analysis gaps; impact may be incomplete\n"},
		{second.ID, "package:B", "IMPACT_TARGET package:B\nAFFECTED distance=1 package:C\nIMPACT_COVERAGE status=complete\n"},
		{first.ID, "package:C", "IMPACT_TARGET package:C\nAFFECTED none\nIMPACT_COVERAGE status=completed_with_gaps\nIMPACT_WARNING stored scan has analysis gaps; impact may be incomplete\n"},
	} {
		var out, errOut bytes.Buffer
		if code := run([]string{"impact", root, test.scanID, test.target}, &out, &errOut); code != 0 || out.String() != test.want {
			t.Fatalf("impact(%s, %s) = %d, %q, %q", test.scanID, test.target, code, out.String(), errOut.String())
		}
	}
	for _, target := range []string{"file:a.go", "function:a.go#Run", "missing"} {
		var out, errOut bytes.Buffer
		if code := run([]string{"impact", root, first.ID, target}, &out, &errOut); code != 1 || out.Len() != 0 {
			t.Fatalf("impact(%s) = %d, %q, %q", target, code, out.String(), errOut.String())
		}
		want := "is not a PACKAGE"
		if target == "missing" {
			want = `node "missing" not found`
		}
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("impact(%s) error = %q", target, errOut.String())
		}
	}
}

func TestServeRequiresSavedSnapshot(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve", root, "missing"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `snapshot "missing" not found`) {
		t.Fatalf("serve missing snapshot = %d, %q, %q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"serve", root}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "usage: nami serve") {
		t.Fatalf("serve usage = %d, %q, %q", code, stdout.String(), stderr.String())
	}
}
