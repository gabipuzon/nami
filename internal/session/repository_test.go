package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gabipuzon/nami/internal/storage"
)

func TestRepositoryRescanUsesCurrentSourceAndIgnoreRules(t *testing.T) {
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test/refresh\n\ngo 1.25\n")
	write("a.go", "package refresh\nvar Before = 1\n")
	scan := RepositoryScan(root)
	old, err := scan()
	if err != nil {
		t.Fatal(err)
	}
	write("a.go", "package refresh\nvar After = 2\n")
	write("generated.go", "malformed generated source")
	write(".namiignore", "/generated.go\n")
	current, err := scan()
	if err != nil {
		t.Fatal(err)
	}
	if current.ID == old.ID || current.CreatedAt.IsZero() || current.Status != "complete" || len(current.Result.Exclusions) != 1 {
		t.Fatal(current)
	}
	found := false
	for _, node := range current.Result.Graph.Nodes {
		if node.Name == "Before" {
			t.Fatal("old declaration survived")
		}
		if node.Name == "After" {
			found = true
		}
	}
	if !found {
		t.Fatal("new declaration missing")
	}
	write(".namiignore", "[\n")
	if _, err := scan(); err == nil {
		t.Fatal("malformed config did not fail")
	}
	store, err := storage.OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scans, err := store.List()
	if err != nil || len(scans) != 2 {
		t.Fatal(scans, err)
	}
	restored, err := store.Load(old.ID)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, node := range restored.Result.Graph.Nodes {
		if node.Name == "Before" {
			found = true
		}
	}
	if !found {
		t.Fatal("previous snapshot changed")
	}
}
