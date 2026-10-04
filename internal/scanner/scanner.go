package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ignoredDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".nami": true,
	"bin": true, "build": true, "dist": true, "node_modules": true, "vendor": true,
}

type Skipped struct {
	Path   string
	Reason string
}

type Result struct {
	Files   []string
	Skipped []Skipped
}

// Scan discovers regular files without interpreting their contents.
func Scan(root string) (Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Result{}, fmt.Errorf("scan %q: %w", root, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("scan %q: not a directory", root)
	}

	result := Result{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && ignoredDirectories[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !entry.Type().IsRegular() {
			if strings.HasSuffix(rel, ".go") {
				result.Skipped = append(result.Skipped, Skipped{Path: rel, Reason: "not a regular file"})
			}
			return nil
		}
		result.Files = append(result.Files, rel)
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("scan %q: %w", root, err)
	}
	sort.Strings(result.Files)
	sort.Slice(result.Skipped, func(i, j int) bool { return result.Skipped[i].Path < result.Skipped[j].Path })
	return result, nil
}
