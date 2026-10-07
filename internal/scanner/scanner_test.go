package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestScanSkipsGeneratedNextBuild(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".next"), 0700); err != nil {
		t.Fatal(err)
	}
	for name := range map[string]bool{"main.go": true, ".next/generated.js": true} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Discovered != 1 || !reflect.DeepEqual(result.Files, []string{"main.go"}) {
		t.Fatalf("generated build entered scan: %+v", result)
	}
}
