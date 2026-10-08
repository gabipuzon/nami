package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

var ignoredDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".nami": true, ".next": true,
	"bin": true, "build": true, "dist": true, "node_modules": true, "vendor": true,
}

type Skipped struct {
	Path   string
	Reason string
}

type Result struct {
	Discovered int
	Files      []string
	Skipped    []Skipped
	Exclusions []Skipped
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

	rules, err := loadIgnore(root)
	if err != nil {
		return Result{}, err
	}
	result := Result{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() && ignoredDirectories[entry.Name()] {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ignoreFile {
			return nil
		}
		if rule := exclusionFor(rules, rel, entry.IsDir()); rule != nil {
			reportedPath := rel
			if entry.IsDir() {
				reportedPath += "/"
			}
			result.Exclusions = append(result.Exclusions, Skipped{Path: reportedPath, Reason: fmt.Sprintf("%s:%d pattern %q", ignoreFile, rule.line, rule.pattern)})
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		result.Discovered++
		if !entry.Type().IsRegular() {
			result.Skipped = append(result.Skipped, Skipped{Path: rel, Reason: "not a regular file"})
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
	sort.Slice(result.Exclusions, func(i, j int) bool { return result.Exclusions[i].Path < result.Exclusions[j].Path })
	return result, nil
}
