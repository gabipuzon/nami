package tsjsanalyzer

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func writeFixture(t *testing.T, root string, files map[string]string) []string {
	t.Helper()
	var paths []string
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		switch filepath.Ext(name) {
		case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
			paths = append(paths, name)
		}
	}
	return paths
}
func TestSyntaxResolutionAndIsolation(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "EXECUTED")
	encoded, _ := json.Marshal(marker)
	poison := "require('node:fs').writeFileSync(" + string(encoded) + ",'bad');"
	files := writeFixture(t, root, map[string]string{
		"tsconfig.json": `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["src/*"]}}}`,
		"package.json":  `{"dependencies":{"react":"*"}}`,
		"main.tsx": `import { fn } from './lib.js';
import type { User } from './lib.js';
import './setup';
export { fn } from './lib';
export * from '@/utils';
import fs from 'node:fs';
import React from 'react';
import './ambiguous';
import './missing';
import 'mystery';
import './style.css';
import './broken';
interface User { id: string }
type ID = string;
enum Mode { First }
class Service { constructor() {} run() {} ['computed']() {} field = () => {} }
const fetchUser = async () => <div />;
const { first, second: renamed } = { first: 1, second: 2 };
function overloaded(x: string): string;
function overloaded(x: number): number;
function overloaded(x: unknown) { function nested() {} return x; }
const dynamic = () => import('./lib');`,
		"lib.ts":       "export function fn() {}\nexport interface User {}",
		"setup.js":     poison,
		"src/utils.ts": "export const value = 1;",
		"ambiguous.ts": "", "ambiguous.tsx": "",
		"broken.ts": "const = ;",
		"other.mts": "export const a = () => 1;", "other.cts": "class C { save() {} }",
		"other.jsx": "export const View = () => <span />;",
		"other.mjs": "export function main() {}", "other.cjs": "const x = require('./setup');",
	})
	// Poison sources and environment hooks must not run. A repository compiler
	// must never be imported, even if it would satisfy ordinary Node resolution.
	writeFixture(t, root, map[string]string{"node_modules/typescript/index.js": poison, "poison.cjs": poison})
	t.Setenv("NODE_OPTIONS", "--require "+filepath.Join(root, "poison.cjs"))
	t.Setenv("NODE_PATH", filepath.Join(root, "node_modules"))
	t.Setenv("NODE_V8_COVERAGE", filepath.Join(root, "coverage"))
	first := Analyze(root, files)
	if first.FilesAnalyzed != len(files)-1 || first.FilesFailed != 1 {
		t.Fatalf("files: %+v", first)
	}
	if first.Imports != (ImportCounts{Discovered: 12, InternalResolved: 5, StandardLibrary: 1, External: 1, Unresolved: 4, Unclassified: 1}) {
		t.Fatalf("imports: %+v; issues %+v", first.Imports, first.Issues)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("repository code executed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "coverage")); !os.IsNotExist(err) {
		t.Fatal("Node wrote repository coverage", err)
	}
	second := Analyze(root, files)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("nondeterministic analysis")
	}
	counts := map[graph.NodeKind]int{}
	names := map[string]bool{}
	for _, node := range first.Fragment.Nodes {
		if node.HasExportCount {
			t.Fatal("invented export count")
		}
		if node.Path == "main.tsx" {
			counts[node.Kind]++
			names[node.Name] = true
		}
		if node.Kind == graph.File && strings.HasSuffix(node.Path, ".js") && node.Language != "javascript" {
			t.Fatal("lost JavaScript identity")
		}
		if node.Kind == graph.Package || node.Kind == graph.Module {
			t.Fatal("invented container")
		}
	}
	for _, kind := range []graph.NodeKind{graph.Function, graph.Class, graph.Interface, graph.Type, graph.Variable, graph.Enum} {
		if counts[kind] == 0 {
			t.Errorf("missing %s", kind)
		}
	}
	if !names["Service.computed"] || counts[graph.Method] != 3 || names["nested"] || names["Service.field"] {
		t.Fatal("incorrect declaration scope", names)
	}
	imports := 0
	for _, edge := range first.Fragment.Edges {
		if edge.Kind == graph.Imports {
			imports++
		}
		if edge.Kind == graph.UsesExport {
			t.Fatal("invented export use")
		}
	}
	if imports != 3 || len(first.SourceEvidence) != 5 {
		t.Fatal("occurrences not preserved/deduplicated", imports, len(first.SourceEvidence))
	}
	original, err := os.ReadFile(filepath.Join(root, "main.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	if first.SourceEvidence[0].Hash != graph.SourceHash(original) {
		t.Fatal("source evidence hash mismatch")
	}
}
func TestConditionalRuntimeAndMissingAssets(t *testing.T) {
	t.Setenv("PATH", "")
	if got := Analyze(t.TempDir(), nil); !reflect.DeepEqual(got, Result{}) {
		t.Fatal("empty analysis touched runtime")
	}
	got := Analyze(t.TempDir(), []string{"a.ts"})
	if got.FilesFailed != 1 || got.Issues[0].Kind != "TSJS_RUNTIME_ERROR" {
		t.Fatal(got)
	}
	old := assets
	assets = embed.FS{}
	defer func() { assets = old }()
	if _, _, err := extract(); err == nil {
		t.Fatal("missing compiler accepted")
	}
}
func TestPrivateVerifiedExtraction(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir, cleanup, err := extract()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 || !strings.Contains(filepath.Base(dir), "6.0.3") {
		t.Fatal("unsafe cache directory")
	}
	compiler, _ := assets.ReadFile("assets/typescript.cjs")
	target := filepath.Join(dir, "typescript.cjs")
	if err := verifyArtifact(target, compiler); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if verifyArtifact(target, compiler) == nil {
		t.Fatal("tampered compiler accepted")
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "helper.cjs"), target); err != nil {
		t.Fatal(err)
	}
	if verifyArtifact(target, compiler) == nil {
		t.Fatal("symlink accepted")
	}
}
func TestResolutionPolicies(t *testing.T) {
	files := map[string]bool{"src/a.ts": true, "src/a.js": true, "src/index.ts": true, "src/pick.mts": true, "src/pick.cts": true, "src/a.tsx": true}
	for _, tc := range []struct {
		specifier, target string
		local             bool
	}{
		{"./a.js", "", true}, {"./a.ts", "src/a.ts", true}, {"./pick.mjs", "src/pick.mts", true}, {"./pick.cjs", "src/pick.cts", true}, {"./", "src/index.ts", true}, {"../../outside", "", true}, {"unknown", "", false},
	} {
		target, local, _ := resolve("src/main.ts", tc.specifier, config{}, files)
		if target != tc.target || local != tc.local {
			t.Errorf("%s: %s %v", tc.specifier, target, local)
		}
	}
	meta := &metadata{Path: "tsconfig.json", Value: json.RawMessage(`{"extends":"./base.json","compilerOptions":{"paths":{"@/*":["src/*"]}}}`)}
	c, reason := parseConfig(meta)
	if reason == "" {
		t.Fatal("extends silently accepted")
	}
	if target, _, _ := resolve("main.ts", "@/a", c, files); target != "" {
		t.Fatal("guessed extended alias")
	}
}

func TestAliasesAndPackageEvidence(t *testing.T) {
	files := map[string]bool{"src/a.ts": true, "src/b.ts": true, "src/utils/index.ts": true}
	for _, tc := range []struct {
		configuration, specifier, target string
		ambiguous                        bool
	}{
		{`{"compilerOptions":{"paths":{"exact":["src/a.ts"]}}}`, "exact", "src/a.ts", false},
		{`{"compilerOptions":{"baseUrl":"src"}}`, "a", "src/a.ts", false},
		{`{"compilerOptions":{"paths":{"@/*":["src/*"]}}}`, "@/utils", "src/utils/index.ts", false},
		{`{"compilerOptions":{"paths":{"exact":["src/a","src/b"]}}}`, "exact", "", true},
		{`{"compilerOptions":{"paths":{"@/*":["src/*"],"@/*/index":["src/*"]}}}`, "@/utils/index", "", true},
	} {
		c, reason := parseConfig(&metadata{Path: "tsconfig.json", Value: json.RawMessage(tc.configuration)})
		if reason != "" {
			t.Fatal(reason)
		}
		target, local, gap := resolve("main.ts", tc.specifier, c, files)
		if target != tc.target || !local || (tc.ambiguous && gap == "") {
			t.Fatalf("%s: %s %t %s", tc.specifier, target, local, gap)
		}
	}
	external, reason := parsePackage(&metadata{Value: json.RawMessage(`{"dependencies":{"a":"1"},"devDependencies":{"b":"1"},"peerDependencies":{"@scope/c":"1"},"optionalDependencies":{"d":"1"}}`)})
	if reason != "" || len(external) != 4 || !external[packageName("@scope/c/subpath")] {
		t.Fatal(external, reason)
	}
}

func TestSymlinkedMetadataCannotReadOutsideRepository(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	files := writeFixture(t, root, map[string]string{"main.ts": "import x from '@/x';"})
	writeFixture(t, outside, map[string]string{"tsconfig.json": `{"compilerOptions":{"paths":{"@/*":["*"]}}}`})
	if err := os.Symlink(filepath.Join(outside, "tsconfig.json"), filepath.Join(root, "tsconfig.json")); err != nil {
		t.Fatal(err)
	}
	got := Analyze(root, files)
	if got.FilesAnalyzed != 1 || got.Imports.InternalResolved != 0 || len(got.Issues) < 2 {
		t.Fatal(got)
	}
	if !strings.Contains(got.Issues[0].Reason, "outside repository") {
		t.Fatal(got.Issues)
	}
}
