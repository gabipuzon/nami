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

	"github.com/gabipuzon/nami/internal/graph"
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

type packageLookup struct {
	dir                 string
	name                string
	reason              string
	metadataUnavailable bool
}

type workspaceState struct {
	active         bool
	sourceIncluded bool
	err            error
}

func Analyze(root string, goFiles, scannedFiles []string) Result {
	modules, issues := loadModules(root, scannedFiles)
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
	packagesByDir := make(map[string][]string)
	packageLookups := make(map[string]packageLookup)
	workspaceStates := make(map[string]workspaceState)
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

		if moduleKnown && !strings.HasSuffix(file.Name.Name, "_test") {
			packagesByDir[dir] = append(packagesByDir[dir], packageID)
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
			lookup := resolveLocalPackage(root, file.module, imp, packageLookups, workspaceStates)
			if lookup.metadataUnavailable {
				lookup = sameModulePackage(root, file.module, imp, modules, workspaceStates)
			}
			if lookup.dir != "" {
				var matching []string
				for _, id := range packagesByDir[lookup.dir] {
					if lookup.name == "" || packages[id].Name == lookup.name {
						matching = append(matching, id)
					}
				}
				targets := unique(matching)
				if len(targets) == 1 {
					result.Imports.InternalResolved++
					result.Fragment.Edges = append(result.Fragment.Edges, graph.Edge{Kind: graph.Imports, From: "file:" + file.path, To: targets[0]})
					continue
				}
				result.Imports.Unresolved++
				reason := "local package not found or could not be analyzed"
				if len(targets) > 1 {
					reason = "multiple local packages match"
				}
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: reason})
				continue
			}
			if hasImportPrefix(imp, file.module.path) && !shadowedModuleImport(imp, file.module) {
				result.Imports.Unresolved++
				reason := "same-module package not found or could not be analyzed"
				if lookup.reason != "" {
					reason = lookup.reason
				}
				issues = append(issues, Issue{Kind: "UNRESOLVED_IMPORT", Path: file.path, Import: imp, Reason: reason})
				continue
			}
			if stdErr != nil {
				result.Imports.Unclassified++
				issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: "standard-library list unavailable"})
			} else if externalDependency(imp, file.module) {
				workspace := workspaceForModule(root, file.module, workspaceStates)
				if workspace.err != nil || workspace.active {
					result.Imports.Unclassified++
					reason := "Go workspace may override the declared dependency"
					if workspace.err != nil {
						reason = "Go workspace status unavailable: " + workspace.err.Error()
					} else if !workspace.sourceIncluded {
						reason = "source module is not included in active Go workspace"
					}
					issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: reason})
				} else {
					result.Imports.External++
				}
			} else {
				result.Imports.Unclassified++
				reason := "no evidence of a standard-library, repository, or external dependency"
				if lookup.reason != "" {
					reason = lookup.reason
				} else if matchesScannedModule(imp, modules) {
					reason = "matching scanned module has no proven local resolution"
				}
				issues = append(issues, Issue{Kind: "UNCLASSIFIED_IMPORT", Path: file.path, Import: imp, Reason: reason})
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

func matchesScannedModule(importPath string, modules []module) bool {
	for _, mod := range modules {
		if hasImportPrefix(importPath, mod.path) {
			return true
		}
	}
	return false
}

func hasImportPrefix(importPath, modulePath string) bool {
	return importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
}

func resolveLocalPackage(root string, source module, importPath string, cache map[string]packageLookup, workspaces map[string]workspaceState) packageLookup {
	key := source.dir + "\x00" + importPath
	if lookup, ok := cache[key]; ok {
		return lookup
	}
	workspace := workspaceForModule(root, source, workspaces)
	if workspace.err != nil {
		return packageLookup{reason: "Go workspace status unavailable: " + workspace.err.Error()}
	}
	if workspace.active && !workspace.sourceIncluded {
		return packageLookup{reason: "source module is not included in active Go workspace"}
	}
	command := exec.Command("go", "list", "-e", "-json", "-mod=readonly", importPath)
	command.Dir = filepath.Join(root, filepath.FromSlash(source.dir))
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	output, err := command.Output()
	if err != nil {
		reason := err.Error()
		if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
			reason = strings.TrimSpace(string(exit.Stderr))
		}
		lookup := packageLookup{reason: "Go package metadata unavailable: " + reason, metadataUnavailable: true}
		cache[key] = lookup
		return lookup
	}
	var data struct {
		ImportPath string
		Dir        string
		Name       string
		GoFiles    []string
		CgoFiles   []string
		Error      *struct{ Err string }
	}
	if err := json.Unmarshal(output, &data); err != nil {
		lookup := packageLookup{reason: "Go package metadata unavailable: " + err.Error()}
		cache[key] = lookup
		return lookup
	}
	if data.Error != nil || data.ImportPath != importPath || data.Dir == "" || data.Name == "" || len(data.GoFiles)+len(data.CgoFiles) == 0 {
		reason := "Go did not resolve an analyzable local package"
		if data.Error != nil && data.Error.Err != "" {
			reason = "Go package resolution failed: " + data.Error.Err
		}
		lookup := packageLookup{reason: reason}
		cache[key] = lookup
		return lookup
	}
	localRoot, rootErr := filepath.EvalSymlinks(root)
	resolved, dirErr := filepath.EvalSymlinks(data.Dir)
	if rootErr != nil || dirErr != nil {
		lookup := packageLookup{reason: "Go package directory could not be verified"}
		cache[key] = lookup
		return lookup
	}
	rel, err := filepath.Rel(localRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		lookup := packageLookup{reason: "Go resolved package outside scanned repository"}
		cache[key] = lookup
		return lookup
	}
	lookup := packageLookup{dir: filepath.ToSlash(rel), name: data.Name}
	cache[key] = lookup
	return lookup
}

func sameModulePackage(root string, source module, importPath string, modules []module, workspaces map[string]workspaceState) packageLookup {
	// Unrelated missing go.sum entries can stop go list before it reports a same-module package.
	if !hasImportPrefix(importPath, source.path) {
		return packageLookup{}
	}
	workspace := workspaceForModule(root, source, workspaces)
	if workspace.err != nil || workspace.active {
		return packageLookup{}
	}
	if shadowedModuleImport(importPath, source) {
		return packageLookup{}
	}
	rel := strings.TrimPrefix(importPath, source.path)
	dir := path.Join(source.dir, strings.TrimPrefix(rel, "/"))
	mod, ok := containingModule(dir, modules)
	if !ok || mod.dir != source.dir {
		return packageLookup{}
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return packageLookup{}
	}
	localRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return packageLookup{}
	}
	physical, err := filepath.Rel(localRoot, resolved)
	if err != nil || physical == ".." || strings.HasPrefix(physical, ".."+string(filepath.Separator)) {
		return packageLookup{}
	}
	return packageLookup{dir: filepath.ToSlash(physical)}
}

func shadowedModuleImport(importPath string, source module) bool {
	for required := range source.requires {
		if len(required) > len(source.path) && hasImportPrefix(importPath, required) {
			return true
		}
	}
	for replaced := range source.replaced {
		if len(replaced) > len(source.path) && hasImportPrefix(importPath, replaced) {
			return true
		}
	}
	return false
}

func lookupModule(root string, source module) string {
	command := exec.Command("go", "list", "-m", "-json", source.path)
	command.Dir = filepath.Join(root, filepath.FromSlash(source.dir))
	command.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	var data struct{ Dir string }
	if json.Unmarshal(output, &data) != nil {
		return ""
	}
	return data.Dir
}

func sameModuleDirectory(root, relativeDir, resolvedDir string) bool {
	if resolvedDir == "" {
		return false
	}
	local, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(relativeDir)))
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(resolvedDir)
	return err == nil && local == resolved
}

func workspaceForModule(root string, mod module, cache map[string]workspaceState) workspaceState {
	if state, ok := cache[mod.dir]; ok {
		return state
	}
	active, err := activeWorkspace(filepath.Join(root, filepath.FromSlash(mod.dir)))
	state := workspaceState{active: active, sourceIncluded: !active, err: err}
	if active && err == nil {
		state.sourceIncluded = sameModuleDirectory(root, mod.dir, lookupModule(root, mod))
	}
	cache[mod.dir] = state
	return state
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
