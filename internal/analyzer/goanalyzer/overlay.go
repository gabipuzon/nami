package goanalyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Go's metadata command otherwise sees files outside the supplied source set.
// A deletion overlay omits explicitly excluded Go files without changing source
// on disk or manufacturing replacement source for package resolution.
func exclusionOverlay(root string, exclusions []string) (string, error) {
	replacements := make(map[string]string)
	for _, relative := range exclusions {
		if strings.HasSuffix(relative, ".go") {
			replacements[filepath.Join(root, filepath.FromSlash(relative))] = ""
		}
	}
	if len(replacements) == 0 {
		return "", nil
	}
	content, err := json.Marshal(struct{ Replace map[string]string }{replacements})
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "nami-go-exclusions-*.json")
	if err != nil {
		return "", fmt.Errorf("create Go exclusion overlay: %w", err)
	}
	name := file.Name()
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(name)
		return "", fmt.Errorf("write Go exclusion overlay: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close Go exclusion overlay: %w", err)
	}
	return name, nil
}
