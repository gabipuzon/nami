package detect

import (
	"path"
	"strings"
)

// GoFiles selects source candidates; the analyzer decides what they mean.
func GoFiles(files []string) []string {
	var goFiles []string
	for _, file := range files {
		if strings.HasSuffix(file, ".go") {
			goFiles = append(goFiles, file)
		}
	}
	return goFiles
}

// UnsupportedSources identifies source languages planned beyond this Go-only phase.
func UnsupportedSources(files []string) []string {
	var unsupported []string
	for _, file := range files {
		switch strings.ToLower(path.Ext(file)) {
		case ".py", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".mts", ".cts":
			unsupported = append(unsupported, file)
		}
	}
	return unsupported
}
