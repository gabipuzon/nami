package main

import (
	"bytes"
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

func TestMapIsNotAvailable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"map", "."}, &stdout, &stderr); code != 2 {
		t.Fatalf("run(map .) exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unknown command: map") {
		t.Fatalf("run(map .) stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}
