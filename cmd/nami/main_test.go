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
		"files_discovered=8 supported_source_files=5 files_analyzed=5 files_skipped=1 files_failed=0",
		"imports_discovered=8 internal_imports_resolved=5 standard_library_imports=1 external_imports=0 unresolved_imports=1 cgo_imports=0 unclassified_imports=1",
		"UNCLASSIFIED_IMPORT main.go \"github.com/external/thing\"",
		"UNRESOLVED_IMPORT alpha/second.go",
		"UNSUPPORTED_FILE notes.py",
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
