package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
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
	root := filepath.Join("..", "..", "internal", "analysis", "testdata", "fixture")
	var first, second, stderr bytes.Buffer
	if code := run([]string{"map", root}, &first, &stderr); code != 0 {
		t.Fatalf("first map exit code = %d, stderr = %q", code, stderr.String())
	}
	if code := run([]string{"map", root}, &second, &stderr); code != 0 {
		t.Fatalf("second map exit code = %d, stderr = %q", code, stderr.String())
	}
	if first.String() != second.String() {
		t.Fatal("unchanged repository produced different CLI output")
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
