package detect

import (
	"reflect"
	"testing"
)

func TestUnsupportedSources(t *testing.T) {
	files := []string{
		"a.rs", "b.java", "c.rb", "d.c", "e.cpp", "f.cs", "g.php", "h.swift", "i.kt",
		"j.go", "README.md",
	}
	want := files[:9]
	if got := UnsupportedSources(files); !reflect.DeepEqual(got, want) {
		t.Fatalf("unsupported files = %q, want %q", got, want)
	}
}

func TestPythonSourcesAreSupported(t *testing.T) {
	files := []string{"main.go", "users/service.py", "notes.txt", "other.ts"}
	if got := PythonFiles(files); !reflect.DeepEqual(got, []string{"users/service.py"}) {
		t.Fatalf("Python candidates: %v", got)
	}
	if got := UnsupportedSources(files); !reflect.DeepEqual(got, []string{"other.ts"}) {
		t.Fatalf("unsupported: %v", got)
	}
}
