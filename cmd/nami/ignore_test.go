package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIMapAndShowPreserveExclusionScope(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module example.com/scoped\ngo 1.25.0\n", "kept.go": "package scoped\n", "broken.go": "!!!", ".namiignore": "broken.go\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"map", "--no-serve", root}, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	analysisOutput, footer := splitScanFooter(t, stdout.String(), "SAVED_SCAN")
	if !strings.Contains(analysisOutput, "EXCLUDED_PATH broken.go: .namiignore:1") || !strings.Contains(analysisOutput, "COVERAGE status=complete") || !strings.Contains(analysisOutput, "excluded_paths=1") || !strings.Contains(analysisOutput, "files_skipped=0 files_failed=0") || strings.Contains(analysisOutput, "file:broken.go") {
		t.Fatal(analysisOutput)
	}
	fields := strings.Fields(footer)
	var scanID string
	for _, field := range fields {
		if strings.HasPrefix(field, "id=") {
			scanID = strings.TrimPrefix(field, "id=")
		}
	}
	if scanID == "" {
		t.Fatal(footer)
	}
	if err := os.WriteFile(filepath.Join(root, ".namiignore"), []byte("kept.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"show", root, scanID}, &stdout, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	reopened, _ := splitScanFooter(t, stdout.String(), "STORED_SCAN")
	if reopened != analysisOutput {
		t.Fatal("show recalculated scope", stdout.String())
	}
}

func TestCLIRejectsMalformedIgnoreWithoutSaving(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".namiignore"), []byte("bad[\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"map", "--no-serve", root}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), ".namiignore:1") {
		t.Fatal(code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".nami")); !os.IsNotExist(err) {
		t.Fatal("failed scan created a saved store", err)
	}
}
