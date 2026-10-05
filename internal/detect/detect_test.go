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
