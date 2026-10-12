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

// UnsupportedSources identifies source languages without an implemented analyzer.
func UnsupportedSources(files []string) []string {
	var unsupported []string
	for _, file := range files {
		switch strings.ToLower(path.Ext(file)) {
		case ".rs", ".java", ".rb", ".c", ".cpp", ".cs", ".php", ".swift", ".kt":
			unsupported = append(unsupported, file)
		}
	}
	return unsupported
}

func PythonFiles(files []string) []string {
	var selected []string
	for _, file := range files {
		if strings.HasSuffix(file, ".py") {
			selected = append(selected, file)
		}
	}
	return selected
}

func TSJSFiles(files []string) []string {
	var selected []string
	for _, file := range files {
		switch path.Ext(file) {
		case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
			selected = append(selected, file)
		}
	}
	return selected
}
