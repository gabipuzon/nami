package tsjsanalyzer

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
)

type Issue struct{ Kind, Path, Import, Reason string }
type ImportCounts struct{ Discovered, InternalResolved, StandardLibrary, External, Unresolved, Unclassified int }
type Result struct {
	SourceEvidence             []graph.SourceEvidence
	Fragment                   graph.Fragment
	Issues                     []Issue
	FilesAnalyzed, FilesFailed int
	Imports                    ImportCounts
}
type syntaxImport struct {
	Specifier     string
	Line, Column  int
	EndLine       int `json:"end_line"`
	EndColumn     int `json:"end_column"`
	Snippet, Hash string
	Truncated     bool
}
type declaration struct {
	Kind graph.NodeKind
	Name string
}
type metadata struct {
	Path, Error string
	Value       json.RawMessage
}
type parsedFile struct {
	Path, Error  string
	Declarations []declaration
	Imports      []syntaxImport
	Limitations  []string
	Config       *metadata
	Package      *metadata
}
type response struct {
	Files  []parsedFile
	Stdlib []string
}

func marshalInput(root string, files []string) ([]byte, error) {
	return json.Marshal(struct {
		Root  string   `json:"root"`
		Files []string `json:"files"`
	}{root, files})
}
func failure(files []string, err error) Result {
	return Result{FilesFailed: len(files), Issues: []Issue{{Kind: "TSJS_RUNTIME_ERROR", Reason: err.Error()}}}
}
func Analyze(root string, files []string) Result {
	if len(files) == 0 {
		return Result{}
	}
	output, err := parse(root, files)
	if err != nil {
		return failure(files, err)
	}
	var parsed response
	if err := json.Unmarshal(output, &parsed); err != nil {
		return failure(files, err)
	}
	expected := map[string]bool{}
	for _, file := range files {
		expected[file] = true
	}
	if len(parsed.Files) != len(expected) {
		return failure(files, fmt.Errorf("helper did not account for all files"))
	}
	for _, file := range parsed.Files {
		if !expected[file.Path] {
			return failure(files, fmt.Errorf("duplicate or unrequested helper path"))
		}
		delete(expected, file.Path)
	}
	return build(parsed)
}
func build(parsed response) Result {
	result := Result{}
	successful := map[string]bool{}
	sort.Slice(parsed.Files, func(i, j int) bool { return parsed.Files[i].Path < parsed.Files[j].Path })
	issue := func(kind, file, specifier, reason string) {
		result.Issues = append(result.Issues, Issue{kind, file, specifier, reason})
	}
	for _, file := range parsed.Files {
		if file.Error != "" {
			result.FilesFailed++
			issue("FAILED_FILE", file.Path, "", file.Error)
			continue
		}
		successful[file.Path] = true
		result.FilesAnalyzed++
		language := "javascript"
		switch path.Ext(file.Path) {
		case ".ts", ".tsx", ".mts", ".cts":
			language = "typescript"
		}
		id := "file:" + file.Path
		result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: id, Kind: graph.File, Path: file.Path, Name: path.Base(file.Path), Language: language, HasImportCount: true, ImportCount: len(file.Imports)})
		ordinals := map[string]int{}
		for _, decl := range file.Declarations {
			key := string(decl.Kind) + ":" + decl.Name
			ordinal := ordinals[key]
			ordinals[key]++
			// Tuple encoding prevents quoted/computed names and path separators
			// from colliding with duplicate-declaration suffixes.
			identity, _ := json.Marshal([]string{string(decl.Kind), file.Path, decl.Name, fmt.Sprint(ordinal)})
			symbol := "tsjs-declaration:" + string(identity)
			result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: symbol, Kind: decl.Kind, Path: file.Path, Name: decl.Name, Language: language})
			result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Contains, From: id, To: symbol})
		}
	}
	stdlib := map[string]bool{}
	for _, name := range parsed.Stdlib {
		stdlib[name] = true
		if !strings.HasPrefix(name, "node:") {
			stdlib["node:"+name] = true
		}
	}
	for _, file := range parsed.Files {
		if !successful[file.Path] {
			continue
		}
		config, configErr := parseConfig(file.Config)
		external, packageErr := parsePackage(file.Package)
		for _, reason := range append(file.Limitations, configErr, packageErr) {
			if reason != "" {
				issue("TSJS_LIMITATION", file.Path, "", reason)
			}
		}
		for _, fact := range file.Imports {
			result.Imports.Discovered++
			if stdlib[fact.Specifier] {
				result.Imports.StandardLibrary++
				continue
			}
			target, local, reason := resolve(file.Path, fact.Specifier, config, successful)
			if local && reason == "" {
				result.Imports.InternalResolved++
				edge := graph.Edge{Kind: graph.Imports, From: "file:" + file.Path, To: "file:" + target}
				result.Fragment.Edges = append(result.Fragment.Edges, edge)
				result.SourceEvidence = append(result.SourceEvidence, graph.SourceEvidence{Edge: edge, Path: file.Path, Line: fact.Line, Column: fact.Column, EndLine: fact.EndLine, EndColumn: fact.EndColumn, Snippet: fact.Snippet, Hash: fact.Hash, Truncated: fact.Truncated})
			} else if local {
				result.Imports.Unresolved++
				issue("UNRESOLVED_IMPORT", file.Path, fact.Specifier, reason)
			} else if external[packageName(fact.Specifier)] {
				result.Imports.External++
			} else {
				result.Imports.Unclassified++
				issue("UNCLASSIFIED_IMPORT", file.Path, fact.Specifier, "no proven local target, builtin or declared package")
			}
		}
	}
	normalized, err := graph.Build(result.Fragment)
	if err != nil {
		paths := make([]string, 0, len(parsed.Files))
		for _, file := range parsed.Files {
			paths = append(paths, file.Path)
		}
		return failure(paths, err)
	}
	result.Fragment = graph.Fragment{Nodes: normalized.Nodes, Edges: normalized.Edges}
	return result
}
