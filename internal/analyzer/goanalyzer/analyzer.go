package goanalyzer

import (
	"encoding/json"
	"fmt"
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

type ImportCounts struct {
	Discovered       int
	InternalResolved int
	StandardLibrary  int
	External         int
	Unresolved       int
	Cgo              int
	Unclassified     int
}

type Result struct {
	Fragment      graph.Fragment
	Issues        []Issue
	FilesAnalyzed int
	FilesFailed   int
	Imports       ImportCounts
}

type sourceFile struct {
	path        string
	imports     []string
	moduleKnown bool
	module      module
}

type module struct {
	dir      string
	path     string
	requires map[string]bool
	replaced map[string]bool
	excluded map[string]bool
}

func Analyze(root string, goFiles, scannedFiles []string) Result {
	modules, issues := loadModules(root, scannedFiles)
	workspaceActive, workspaceErr := activeWorkspace(root)
	standard := make(map[string]bool)
	var stdErr error
	if len(goFiles) > 0 {
		standard, stdErr = loadStandardLibrary(root)
		if stdErr != nil {
			issues = append(issues, Issue{Kind: "IMPORT_CLASSIFICATION_ERROR", Path: ".", Reason: stdErr.Error()})
		}
	}
	if len(goFiles) > 0 && len(modules) == 0 && !hasGoMod(scannedFiles) {
		issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: ".", Reason: "no go.mod found; internal imports cannot be resolved"})
	}
	result := Result{Fragment: graph.Fragment{}}
	packages := make(map[string]graph.Node)
	importable := make(map[string][]string)
	var parsed []sourceFile

	for _, rel := range goFiles {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			result.FilesFailed++
			issues = append(issues, Issue{Kind: "FAILED_FILE", Path: rel, Reason: err.Error()})
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), rel, content, parser.AllErrors)
		if err != nil {
			result.FilesFailed++
			issues = append(issues, Issue{Kind: "FAILED_FILE", Path: rel, Reason: err.Error()})
			continue
		}
		result.FilesAnalyzed++

		dir := path.Dir(rel)
		packageID := "package:" + dir + "#" + file.Name.Name
		if _, ok := packages[packageID]; !ok {
			packages[packageID] = graph.Node{ID: packageID, Kind: graph.Package, Path: dir, Name: file.Name.Name}
		}
		fileID := "file:" + rel
		result.Fragment.Nodes = append(result.Fragment.Nodes, graph.Node{ID: fileID, Kind: graph.File, Path: rel, Name: path.Base(rel)})
		result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Contains, From: packageID, To: fileID})

		var imports []string
		for _, imp := range file.Imports {
			result.Imports.Discovered++
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				result.Imports.Unresolved++
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: rel, Import: imp.Path.Value, Reason: "invalid import path literal"})
				continue
			}
			if importPath == "C" {
				result.Imports.Cgo++
				continue
			}
			imports = append(imports, importPath)
		}
		mod, moduleKnown := containingModule(dir, modules)
		parsed = append(parsed, sourceFile{path: rel, imports: imports, moduleKnown: moduleKnown, module: mod})
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
		result.Fragment.Nodes = append(result.Fragment.Nodes, pkg)
	}
	for _, file := range parsed {
		for _, imp := range file.imports {
			if standard[imp] {
				result.Imports.StandardLibrary++
				continue
			}
			if !file.moduleKnown {
				result.Imports.Unclassified++
				issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: "repository module path unavailable"})
				continue
			}
			if targets := unique(importable[imp]); len(targets) == 1 {
				result.Imports.InternalResolved++
				result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Imports, From: "file:" + file.path, To: targets[0]})
				continue
			} else if len(targets) > 1 {
				result.Imports.Unresolved++
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: "multiple internal packages match"})
				continue
			}
			if internalModule(imp, modules) {
				result.Imports.Unresolved++
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: "internal package not found or could not be analyzed"})
			} else if stdErr != nil {
				result.Imports.Unclassified++
				issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: "standard-library list unavailable"})
			} else if externalDependency(imp, file.module) {
				if workspaceErr != nil || workspaceActive {
					result.Imports.Unclassified++
					reason := "Go workspace may override the declared dependency"
					if workspaceErr != nil {
						reason = "Go workspace status unavailable: " + workspaceErr.Error()
					}
					issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: reason})
				} else {
					result.Imports.External++
				}
			} else {
				result.Imports.Unclassified++
				issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: "no evidence of a standard-library, repository, or external dependency"})
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
	result.Issues = issues
	return result
}

func loadStandardLibrary(root string) (map[string]bool, error) {
	command := exec.Command("go", "list", "-e", "std")
	command.Dir = root
	command.Env = append(os.Environ(), "GO111MODULE=off", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off")
	output, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("load standard-library packages: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("load standard-library packages: %w", err)
	}
	standard := make(map[string]bool)
	for _, importPath := range strings.Fields(string(output)) {
		standard[importPath] = true
	}
	return standard, nil
}

func activeWorkspace(root string) (bool, error) {
	command := exec.Command("go", "env", "GOWORK")
	command.Dir = root
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off")
	output, err := command.Output()
	if err != nil {
		return false, err
	}
	workspace := strings.TrimSpace(string(output))
	return workspace != "" && workspace != "off", nil
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
			Module  *struct{ Path string }
			Require []struct{ Path string }
			Replace []struct{ Old struct{ Path string } }
			Exclude []struct{ Path string }
		}
		if err := json.Unmarshal(output, &data); err != nil || data.Module == nil || data.Module.Path == "" {
			issues = append(issues, Issue{Kind: "MODULE_ERROR", Path: rel, Reason: "module path unavailable"})
			continue
		}
		mod := module{
			dir: dir, path: data.Module.Path,
			requires: make(map[string]bool), replaced: make(map[string]bool), excluded: make(map[string]bool),
		}
		for _, required := range data.Require {
			mod.requires[required.Path] = true
		}
		for _, replacement := range data.Replace {
			mod.replaced[replacement.Old.Path] = true
		}
		for _, exclusion := range data.Exclude {
			mod.excluded[exclusion.Path] = true
		}
		modules = append(modules, mod)
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

func externalDependency(importPath string, mod module) bool {
	longest := ""
	for required := range mod.requires {
		if (importPath == required || strings.HasPrefix(importPath, required+"/")) && len(required) > len(longest) {
			longest = required
		}
	}
	return longest != "" && !mod.replaced[longest] && !mod.excluded[longest]
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
