package goanalyzer

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"nami/internal/graph"
)

type Issue struct {
	Kind   string
	Path   string
	Import string
	Reason string
}

type sourceFile struct {
	path        string
	imports     []string
	moduleKnown bool
}

type module struct {
	dir  string
	path string
}

func Analyze(root string, goFiles, scannedFiles []string) (graph.Fragment, []Issue) {
	modules, issues := loadModules(root, scannedFiles)
	if len(goFiles) > 0 && len(modules) == 0 && !hasGoMod(scannedFiles) {
		issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: ".", Reason: "no go.mod found; internal imports cannot be resolved"})
	}
	fragment := graph.Fragment{}
	packages := make(map[string]graph.Node)
	importable := make(map[string][]string)
	var parsed []sourceFile

	for _, rel := range goFiles {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			issues = append(issues, Issue{Kind: "SKIPPED_FILE", Path: rel, Reason: err.Error()})
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), rel, content, parser.AllErrors)
		if err != nil {
			issues = append(issues, Issue{Kind: "SKIPPED_FILE", Path: rel, Reason: err.Error()})
			continue
		}

		dir := path.Dir(rel)
		packageID := "package:" + dir + "#" + file.Name.Name
		if _, ok := packages[packageID]; !ok {
			packages[packageID] = graph.Node{ID: packageID, Kind: graph.Package, Path: dir, Name: file.Name.Name}
		}
		fileID := "file:" + rel
		fragment.Nodes = append(fragment.Nodes, graph.Node{ID: fileID, Kind: graph.File, Path: rel, Name: path.Base(rel)})
		fragment.Edges = append(fragment.Edges, graph.Edge{Kind: graph.Contains, From: packageID, To: fileID})

		var imports []string
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: rel, Import: imp.Path.Value, Reason: "invalid import path literal"})
				continue
			}
			if importPath != "C" {
				imports = append(imports, importPath)
			}
		}
		mod, moduleKnown := containingModule(dir, modules)
		parsed = append(parsed, sourceFile{path: rel, imports: imports, moduleKnown: moduleKnown})
		if !moduleKnown && len(modules) > 0 {
			issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: rel, Reason: "no enclosing go.mod; internal imports cannot be resolved"})
		}

		if !strings.HasSuffix(file.Name.Name, "_test") {
			if moduleKnown {
				importPath := mod.path
				if dir != mod.dir {
					if mod.dir == "." {
						importPath += "/" + dir
					} else {
						importPath += "/" + strings.TrimPrefix(dir, mod.dir+"/")
					}
				}
				importable[importPath] = append(importable[importPath], packageID)
			}
		}
	}

	for _, pkg := range packages {
		fragment.Nodes = append(fragment.Nodes, pkg)
	}
	for _, file := range parsed {
		for _, imp := range file.imports {
			if !file.moduleKnown {
				if internalModule(imp, modules) || strings.Contains(strings.Split(imp, "/")[0], ".") {
					issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: "repository module path unavailable"})
				}
				continue
			}
			if targets := unique(importable[imp]); len(targets) == 1 {
				fragment.Edges = append(fragment.Edges, graph.Edge{Kind: graph.Imports, From: "file:" + file.path, To: targets[0]})
				continue
			} else if len(targets) > 1 {
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: "multiple internal packages match"})
				continue
			}
			if internalModule(imp, modules) {
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: "internal package not found or could not be analyzed"})
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
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
	return fragment, issues
}

func loadModules(root string, scannedFiles []string) ([]module, []Issue) {
	var modules []module
	var issues []Issue
	for _, rel := range scannedFiles {
		if path.Base(rel) != "go.mod" {
			continue
		}
		dir := path.Dir(rel)
		command := exec.Command("go", "mod", "edit", "-json")
		command.Dir = filepath.Join(root, filepath.FromSlash(dir))
		command.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
		output, err := command.Output()
		if err != nil {
			reason := err.Error()
			if exit, ok := err.(*exec.ExitError); ok {
				reason = strings.TrimSpace(string(exit.Stderr))
			}
			issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: rel, Reason: reason})
			continue
		}
		var data struct {
			Module *struct{ Path string }
		}
		if err := json.Unmarshal(output, &data); err != nil || data.Module == nil || data.Module.Path == "" {
			issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: rel, Reason: "module path unavailable"})
			continue
		}
		modules = append(modules, module{dir: dir, path: data.Module.Path})
	}
	sort.Slice(modules, func(i, j int) bool { return len(modules[i].dir) > len(modules[j].dir) })
	return modules, issues
}

func hasGoMod(files []string) bool {
	for _, file := range files {
		if path.Base(file) == "go.mod" {
			return true
		}
	}
	return false
}

func containingModule(dir string, modules []module) (module, bool) {
	for _, mod := range modules {
		if mod.dir == "." || dir == mod.dir || strings.HasPrefix(dir, mod.dir+"/") {
			return mod, true
		}
	}
	return module{}, false
}

func internalModule(importPath string, modules []module) bool {
	for _, mod := range modules {
		if importPath == mod.path || strings.HasPrefix(importPath, mod.path+"/") {
			return true
		}
	}
	return false
}

func unique(ids []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}
