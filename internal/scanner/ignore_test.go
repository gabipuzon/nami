package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeScanFile(t *testing.T, root, name, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestIgnorePatternSemantics(t *testing.T) {
	for _, test := range []struct {
		pattern, path   string
		directory, want bool
	}{
		{"*.go", "a.go", false, true}, {"*.go", "nested/a.go", false, true}, {"*.go", "a.py", false, false},
		{"/a.go", "a.go", false, true}, {"/a.go", "nested/a.go", false, false},
		{"nested/a.go", "nested/a.go", false, true}, {"nested/a.go", "other/nested/a.go", false, false},
		{"cache/", "nested/cache", true, true}, {"cache/", "cache", false, false},
		{"a/**/b.py", "a/b.py", false, true}, {"a/**/b.py", "a/x/y/b.py", false, true},
		{"**/b.py", "b.py", false, true}, {"**/b.py", "a/b.py", false, true},
		{"a/**", "a", true, false}, {"a/**", "a/x/y.py", false, true},
		{"a/*/b.py", "a/x/y/b.py", false, false}, {"a/*/b.py", "a/x/b.py", false, true},
		{"foo?", "fooa", false, true}, {"foo?", "foo", false, false},
		{"[a-z].py", "b.py", false, true}, {"[!a-z].py", "2.py", false, true}, {"[!a-z].py", "b.py", false, false},
		{"[[:digit:]].py", "2.py", false, true}, {"[]a].py", "] .py", false, false}, {"[]a].py", "].py", false, true},
		{`\#literal`, "#literal", false, true}, {`\!literal`, "!literal", false, true}, {`a\*b`, "a*b", false, true},
		{`nested\/a.go`, "nested/a.go", false, true},
		{`[\a].py`, "a.py", false, true},
		{`a/**`, "a/new\nline.py", false, true},
		{"space\\ ", "space ", false, true}, {"space   ", "space", false, true},
		{"literal#hash", "literal#hash", false, true}, {"# comment", "# comment", false, false},
		{"\n\r\n*.go\r\n", "x.go", false, true}, {"**", "nested/a.go", false, true},
	} {
		t.Run(test.pattern+":"+test.path, func(t *testing.T) {
			rules, err := parseIgnore(test.pattern)
			if err != nil {
				t.Fatal(err)
			}
			got := exclusionFor(rules, test.path, test.directory) != nil
			if got != test.want {
				t.Fatalf("pattern %q path %q directory %t = %t", test.pattern, test.path, test.directory, got)
			}
		})
	}
	for _, content := range []string{`broken[`, `oops\`, `!`, `/`, `[z-a]`, `[[:nonsense:]]`} {
		if _, err := parseIgnore(content); err == nil || !strings.Contains(err.Error(), ".namiignore:1") {
			t.Fatalf("malformed %q = %v", content, err)
		}
	}
}

func TestScanIgnorePruningAndNegation(t *testing.T) {
	root := t.TempDir()
	writeScanFile(t, root, ignoreFile, "# Scope\n*.go\n!keep.go\n/cache/\n!cache/rescue.py\nparts/*\n!parts/keep/\n!node_modules/\n")
	for _, name := range []string{"a.go", "nested/a.go", "keep.go", "nested/keep.go", "cache/bad.py", "cache/rescue.py", "parts/drop/a.py", "parts/keep/a.py", "nested/cache/a.py", "node_modules/lib/a.py", ".git/config", ".nami/scans.db", "main.py"} {
		writeScanFile(t, root, name, "")
	}
	result, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"keep.go", "main.py", "nested/cache/a.py", "nested/keep.go", "parts/keep/a.py"}
	if !reflect.DeepEqual(result.Files, want) || result.Discovered != len(want) || len(result.Skipped) != 0 {
		t.Fatal(result)
	}
	paths := []string{}
	for _, excluded := range result.Exclusions {
		paths = append(paths, excluded.Path)
		if !strings.Contains(excluded.Reason, ".namiignore:") {
			t.Fatal(excluded)
		}
	}
	if !reflect.DeepEqual(paths, []string{"a.go", "cache/", "nested/a.go", "parts/drop/"}) {
		t.Fatal(paths)
	}
	second, err := Scan(root)
	if err != nil || !reflect.DeepEqual(result, second) {
		t.Fatal("unstable scan", second, err)
	}
}

func TestIgnoreFileFailuresAndSymlinks(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		if _, err := Scan(t.TempDir()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ignoreFile), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Scan(root); err == nil {
			t.Fatal("nonregular ignore file accepted")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, "rules", "*.go")
		if err := os.Symlink("rules", filepath.Join(root, ignoreFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := Scan(root); err == nil {
			t.Fatal("symlink ignore file accepted")
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, ignoreFile, "")
		if err := os.Chmod(filepath.Join(root, ignoreFile), 0000); err != nil {
			t.Fatal(err)
		}
		if _, err := Scan(root); err == nil {
			t.Fatal("unreadable ignore file accepted")
		}
	})
	t.Run("invalid-text", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, ignoreFile, "\x00")
		if _, err := Scan(root); err == nil {
			t.Fatal("binary ignore file accepted")
		}
	})
	t.Run("empty-is-configuration", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, ignoreFile, "")
		writeScanFile(t, root, "main.py", "")
		result, err := Scan(root)
		if err != nil || result.Discovered != 1 || len(result.Exclusions) != 0 {
			t.Fatal(result, err)
		}
	})
	t.Run("excluded-symlink", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, ignoreFile, "link\n")
		writeScanFile(t, root, "target.py", "")
		if err := os.Symlink("target.py", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		result, err := Scan(root)
		if err != nil || len(result.Exclusions) != 1 || len(result.Skipped) != 0 {
			t.Fatal(result, err)
		}
	})
	t.Run("directory-rule-does-not-follow-symlink", func(t *testing.T) {
		root := t.TempDir()
		writeScanFile(t, root, ignoreFile, "link/\n")
		if err := os.Mkdir(filepath.Join(root, "target"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
			t.Fatal(err)
		}
		result, err := Scan(root)
		if err != nil || len(result.Exclusions) != 0 || len(result.Skipped) != 1 {
			t.Fatal(result, err)
		}
	})
}

// Compare filesystem-level matching with Git, including its parent pruning rule.
// Git is an existing project tool; an unavailable check fails explicitly.
func TestIgnoreMatchesGit(t *testing.T) {
	for _, patterns := range []string{"*.go\n!keep.go\n", "parent/\n!parent/keep.py\n", "parent/*\n!parent/keep.py\n", "parent/**\n!parent/sub/\n!parent/sub/keep.py\n", "**/sub/*.py\n", "[[:digit:]].py\n", "\\#name\n\\!name\n", "name\\ \n", "parent\\/keep.py\n", "[\\a].py\n"} {
		t.Run(patterns, func(t *testing.T) {
			root := t.TempDir()
			writeScanFile(t, root, ignoreFile, patterns)
			writeScanFile(t, root, ".gitignore", patterns)
			names := []string{"main.go", "keep.go", "nested/main.go", "nested/keep.go", "parent/drop.py", "parent/keep.py", "parent/sub/drop.py", "parent/sub/keep.py", "sub/a.py", "1.py", "#name", "!name", "name "}
			for _, name := range names {
				writeScanFile(t, root, name, "")
			}
			if output, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("Git comparison cannot run: %v: %s", err, output)
			}
			result, err := Scan(root)
			if err != nil {
				t.Fatal(err)
			}
			included := map[string]bool{}
			for _, name := range result.Files {
				included[name] = true
			}
			for _, name := range names {
				output, err := exec.Command("git", "-C", root, "check-ignore", "--no-index", "-q", "--", name).CombinedOutput()
				ignored := err == nil
				if err != nil {
					if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
						t.Fatalf("Git comparison failed: %v: %s", err, output)
					}
				}
				if ignored == included[name] {
					t.Fatalf("Git and nami disagree for %q, rules %q", name, patterns)
				}
			}
		})
	}
}
