package pythonanalyzer

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
)

//go:embed helper.py
var helper string

type Issue struct{ Kind, Path, Import, Reason string }
type ImportCounts struct{ Discovered, InternalResolved, StandardLibrary, External, Unresolved, Unclassified int }
type Result struct {
	SourceEvidence             []graph.SourceEvidence
	Fragment                   graph.Fragment
	Issues                     []Issue
	FilesAnalyzed, FilesFailed int
	Imports                    ImportCounts
}
type importFact struct {
	Module, Name  string
	Level, Line   int
	Column        int
	EndLine       int `json:"end_line"`
	EndColumn     int `json:"end_column"`
	Snippet, Hash string
	Truncated     bool
}
type declaration struct {
	Kind graph.NodeKind
	Name string
	Line int
}
type parsedFile struct {
	Path, Error  string
	Declarations []declaration
	Imports      []importFact
}
type response struct {
	Files  []parsedFile
	Stdlib []string
}

func Analyze(root string, files []string) Result {
	if len(files) == 0 {
		return Result{}
	}
	runtime, err := exec.LookPath("python3")
	if err != nil {
		runtime, err = exec.LookPath("python")
	}
	if err != nil {
		return runtimeFailure(files, "neither python3 nor python is available")
	}
	input, err := json.Marshal(struct {
		Root  string   `json:"root"`
		Files []string `json:"files"`
	}{root, files})
	if err != nil {
		return runtimeFailure(files, err.Error())
	}
	// -I excludes repository paths and environment customizations; -S prevents
	// startup hooks from site packages. Only the embedded helper runs.
	command := exec.Command(runtime, "-I", "-S", "-c", helper)
	command.Stdin = bytes.NewReader(input)
	var output, stderr bytes.Buffer
	command.Stdout = &output
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return runtimeFailure(files, fmt.Sprintf("Python AST helper failed: %v: %s", err, strings.TrimSpace(stderr.String())))
	}
	var parsed response
	if err := json.Unmarshal(output.Bytes(), &parsed); err != nil {
		return runtimeFailure(files, "invalid Python AST response: "+err.Error())
	}
	expected := make(map[string]bool, len(files))
	for _, file := range files {
		expected[file] = true
	}
	if len(parsed.Files) != len(expected) {
		return runtimeFailure(files, "Python AST response did not account for every supplied file")
	}
	for _, file := range parsed.Files {
		if !expected[file.Path] {
			return runtimeFailure(files, "Python AST response included a duplicate or unrequested file")
		}
		delete(expected, file.Path)
	}
	return build(parsed, filepath.Base(root))
}
func runtimeFailure(files []string, reason string) Result {
	return Result{FilesFailed: len(files), Issues: []Issue{{Kind: "PYTHON_RUNTIME_ERROR", Reason: reason}}}
}

func build(parsed response, rootName string) Result {
	result := Result{}
	successful := map[string]parsedFile{}
	packages := map[string]string{}
	for _, file := range parsed.Files {
		if file.Error != "" {
			result.FilesFailed++
			result.Issues = append(result.Issues, Issue{Kind: "FAILED_FILE", Path: file.Path, Reason: file.Error})
			continue
		}
		successful[file.Path] = file
		if path.Base(file.Path) == "__init__.py" {
			packages[path.Dir(file.Path)] = ""
		}
	}
	// The contiguous __init__.py chain establishes a classic package name.
	packageName := func(dir string) string {
		if dir == "." {
			return rootName
		}
		parts := []string{path.Base(dir)}
		for parent := path.Dir(dir); ; parent = path.Dir(parent) {
			if _, ok := packages[parent]; !ok {
				break
			}
			name := path.Base(parent)
			if parent == "." {
				name = rootName
			}
			parts = append([]string{name}, parts...)
			if parent == "." {
				break
			}
		}
		return strings.Join(parts, ".")
	}
	index := map[string][]string{}
	addIndex := func(name, id string) { index[name] = append(index[name], id) }
	for dir := range packages {
		name := packageName(dir)
		packages[dir] = name
		id := "python-package:" + dir
		result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: id, Kind: graph.Package, Language: "python", Path: dir, Name: name})
		addIndex(name, id)
		if _, ok := packages[path.Dir(dir)]; ok && dir != "." {
			result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Contains, From: "python-package:" + path.Dir(dir), To: id})
		}
	}
	paths := make([]string, 0, len(successful))
	for rel := range successful {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		file := successful[rel]
		result.FilesAnalyzed++
		id := "file:" + rel
		result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: id, Kind: graph.File, Language: "python", Path: rel, Name: path.Base(rel), ImportCount: len(file.Imports), HasImportCount: true})
		if name, ok := packages[path.Dir(rel)]; ok {
			result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Contains, From: "python-package:" + path.Dir(rel), To: id})
			if path.Base(rel) != "__init__.py" {
				addIndex(name+"."+strings.TrimSuffix(path.Base(rel), ".py"), id)
			}
		}
		identities := map[string]int{}
		for _, decl := range file.Declarations {
			identities[string(decl.Kind)+":"+decl.Name]++
		}
		for _, decl := range file.Declarations {
			identity := strings.ToLower(string(decl.Kind)) + ":" + rel + "#" + decl.Name
			if identities[string(decl.Kind)+":"+decl.Name] > 1 {
				identity += fmt.Sprintf("@%d", decl.Line)
			}
			result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: identity, Kind: decl.Kind, Language: "python", Path: rel, Name: decl.Name})
			result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Contains, From: id, To: identity})
		}
	}
	knownRoots := map[string]bool{}
	for name := range index {
		knownRoots[strings.Split(name, ".")[0]] = true
	}
	stdlib := map[string]bool{}
	for _, name := range parsed.Stdlib {
		stdlib[name] = true
	}
	for _, rel := range paths {
		for _, fact := range successful[rel].Imports {
			result.Imports.Discovered++
			name := fact.Module
			reason := ""
			if fact.Level > 0 {
				context, ok := packages[path.Dir(rel)]
				if !ok {
					reason = "relative import has no proven classic package context"
				} else {
					parts := strings.Split(context, ".")
					if fact.Level > len(parts) {
						reason = "relative import goes beyond the proven package"
					} else {
						base := strings.Join(parts[:len(parts)-fact.Level+1], ".")
						suffix := fact.Module
						if suffix == "" {
							suffix = fact.Name
						}
						name = base
						if suffix != "" {
							name += "." + suffix
						}
					}
				}
			}
			display := strings.Repeat(".", fact.Level) + fact.Module
			if fact.Name != "" {
				display += fact.Name
			}
			candidates := index[name]
			top := strings.Split(name, ".")[0]
			if reason == "" && len(candidates) == 1 && fact.Level == 0 && stdlib[top] {
				reason = "repository module conflicts with a standard-library name; runtime search order is unknown"
			}
			if reason == "" && len(candidates) == 1 {
				result.Imports.InternalResolved++
				result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Imports, From: "file:" + rel, To: candidates[0]})
				result.SourceEvidence = append(result.SourceEvidence, graph.SourceEvidence{Edge: graph.Edge{Kind: graph.Imports, From: "file:" + rel, To: candidates[0]}, Path: rel, Line: fact.Line, Column: fact.Column, EndLine: fact.EndLine, EndColumn: fact.EndColumn, Snippet: fact.Snippet, Hash: fact.Hash, Truncated: fact.Truncated})
				continue
			}
			if reason == "" && len(candidates) > 1 {
				reason = "multiple repository targets have this module identity"
			}
			if reason == "" && fact.Level == 0 && stdlib[top] {
				result.Imports.StandardLibrary++
				continue
			}
			if reason == "" && knownRoots[top] {
				reason = "module is absent from the proven package structure"
			}
			kind := "UNCLASSIFIED_IMPORT"
			if reason != "" {
				kind = "UNRESOLVED_IMPORT"
				result.Imports.Unresolved++
			} else {
				reason = "no proven repository target or standard-library classification"
				result.Imports.Unclassified++
			}
			result.Issues = append(result.Issues, Issue{Kind: kind, Path: rel, Import: display, Reason: reason})
		}
	}
	normalized, err := graph.Build(result.Fragment)
	if err != nil {
		return runtimeFailure(paths, "construct Python graph: "+err.Error())
	}
	result.Fragment = graph.Fragment{Nodes: normalized.Nodes, Edges: normalized.Edges}
	sort.Slice(result.Issues, func(i, j int) bool {
		a, b := result.Issues[i], result.Issues[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Import != b.Import {
			return a.Import < b.Import
		}
		return a.Reason < b.Reason
	})
	return result
}
