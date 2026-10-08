package analysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestSavedEvidenceUsesParsedGoAndPythonBytes(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{"go.mod": "module example.test/app\n\ngo 1.25\n", "b/b.go": "package b\nvar Value = 1\n", "a/a.go": "package a\nimport \"example.test/app/b\"\nvar X = b.Value\nvar Y = b.Value\n", "pkg/__init__.py": "", "pkg/b.py": "value = 1\n", "pkg/a.py": "from . import b\nfrom . import b\n"}
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Map(root)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, fact := range result.SourceEvidence {
		counts[fact.Path+":"+string(fact.Edge.Kind)]++
		if fact.Hash != graph.SourceHash([]byte(files[fact.Path])) || fact.Line < 1 || fact.Snippet == "" {
			t.Fatal(fact)
		}
	}
	if counts["a/a.go:IMPORTS"] != 1 || counts["a/a.go:USES_EXPORT"] != 2 || counts["pkg/a.py:IMPORTS"] != 2 {
		t.Fatal(counts)
	}
	if err := os.WriteFile(filepath.Join(root, "a/a.go"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, fact := range result.SourceEvidence {
		if fact.Path == "a/a.go" && fact.Hash == graph.SourceHash([]byte("changed")) {
			t.Fatal("evidence changed")
		}
	}
}
