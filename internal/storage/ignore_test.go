package storage

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
)

func TestExclusionsPersistSeparately(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module example.com/scoped\ngo 1.25.0\n", "kept.go": "package scoped\n", "broken.go": "!!!", ".namiignore": "broken.go\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Exclusions) != 1 || result.Coverage.Status != "complete" {
		t.Fatal(result)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := store.Save(root, result)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".namiignore"), []byte("kept.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	saved, err := reader.Load(summary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved.Result, result) || saved.Status != "complete" || len(saved.Result.Issues) != 0 {
		t.Fatal("reopening changed scan scope", saved)
	}
	var version int
	if err := reader.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != schemaVersion {
		t.Fatal(version, err)
	}
	newResult, err := analysis.Map(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(newResult.Exclusions) != 1 || newResult.Exclusions[0].Path != "kept.go" || newResult.Coverage.FilesFailed != 1 {
		t.Fatal(newResult)
	}
}
