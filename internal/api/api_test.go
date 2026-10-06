package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
)

func fixtureSnapshot(t *testing.T) storage.Snapshot {
	t.Helper()
	g, err := graph.Build(graph.Fragment{
		Nodes: []graph.Node{
			{ID: "module:M", Kind: graph.Module, Name: "example.com/app", Path: "."},
			{ID: "package:A", Kind: graph.Package, Name: "a", Path: "a"},
			{ID: "package:B", Kind: graph.Package, Name: "b", Path: "b"},
			{ID: "package:C", Kind: graph.Package, Name: "c", Path: "c"},
			{ID: "package:D", Kind: graph.Package, Name: "d", Path: "d"},
			{ID: "file:a.go", Kind: graph.File, Name: "a.go", Path: "a/a.go"},
			{ID: "file:c.go", Kind: graph.File, Name: "c.go", Path: "c/c.go"},
			{ID: "function:a.go#Run", Kind: graph.Function, Name: "Run", Path: "a/a.go"},
		},
		Edges: []graph.Edge{
			{Kind: graph.Contains, From: "module:M", To: "package:A"},
			{Kind: graph.Contains, From: "module:M", To: "package:B"},
			{Kind: graph.Contains, From: "module:M", To: "package:C"},
			{Kind: graph.Contains, From: "module:M", To: "package:D"},
			{Kind: graph.Contains, From: "package:A", To: "file:a.go"},
			{Kind: graph.Contains, From: "package:C", To: "file:c.go"},
			{Kind: graph.Contains, From: "file:a.go", To: "function:a.go#Run"},
			{Kind: graph.Imports, From: "file:a.go", To: "package:B"},
			{Kind: graph.Imports, From: "file:c.go", To: "package:A"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return storage.Snapshot{
		ID: "scan-one", Root: "/repo", CreatedAt: time.Date(2026, 10, 6, 8, 0, 0, 0, time.UTC),
		Status: "completed_with_gaps",
		Result: analysis.Result{
			Graph:    g,
			Coverage: analysis.Coverage{Status: "completed_with_gaps", FilesDiscovered: 5, FilesAnalyzed: 2, FilesSkipped: 1},
			Issues:   []analysis.Issue{{Kind: "UNSUPPORTED_FILE", Path: "notes.py", Reason: "no analyzer for this source language"}},
		},
	}
}

func request(t *testing.T, handler http.Handler, method, target string) (int, map[string]any, string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("%s %s content type = %q", method, target, got)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("%s %s invalid JSON: %v", method, target, err)
	}
	return recorder.Code, body, recorder.Body.String()
}

func TestHandlerRoutesUseStoredGraph(t *testing.T) {
	snapshot := fixtureSnapshot(t)
	handler := NewHandler(snapshot)
	for _, target := range []string{
		"/api/v1/health", "/api/v1/scan", "/api/v1/graph", "/api/v1/packages",
		"/api/v1/symbols?file_id=file:a.go", "/api/v1/dependencies?node_id=file:a.go",
		"/api/v1/dependents?node_id=package:B", "/api/v1/path?from_id=file:c.go&to_id=package:B",
		"/api/v1/impact?package_id=package:B",
	} {
		firstCode, _, first := request(t, handler, http.MethodGet, target)
		secondCode, _, second := request(t, handler, http.MethodGet, target)
		if firstCode != http.StatusOK || secondCode != firstCode || first != second {
			t.Fatalf("%s unstable response: %d %q, %d %q", target, firstCode, first, secondCode, second)
		}
	}
	_, health, _ := request(t, handler, http.MethodGet, "/api/v1/health")
	if health["status"] != "ok" {
		t.Fatalf("health = %+v", health)
	}
	_, scan, _ := request(t, handler, http.MethodGet, "/api/v1/scan")
	coverage := scan["coverage"].(map[string]any)
	if scan["id"] != "scan-one" || scan["root"] != "/repo" || scan["created_at"] != "2026-10-06T08:00:00Z" || scan["status"] != "completed_with_gaps" || coverage["files_discovered"] != float64(5) || len(scan["issues"].([]any)) != 1 {
		t.Fatalf("scan = %+v", scan)
	}
	_, canonical, _ := request(t, handler, http.MethodGet, "/api/v1/graph")
	canonicalEdges := canonical["edges"].([]any)
	if len(canonical["nodes"].([]any)) != 8 || len(canonicalEdges) != 9 {
		t.Fatalf("canonical graph = %+v", canonical)
	}
	for _, raw := range canonicalEdges {
		edge := raw.(map[string]any)
		if edge["from"] == "package:A" && edge["to"] == "package:B" {
			t.Fatalf("derived edge in canonical graph: %+v", edge)
		}
	}
	_, packages, _ := request(t, handler, http.MethodGet, "/api/v1/packages")
	projected := packages["graph"].(map[string]any)
	if len(projected["edges"].([]any)) != 2 || len(packages["evidence"].([]any)) != 2 {
		t.Fatalf("packages = %+v", packages)
	}
	_, symbols, _ := request(t, handler, http.MethodGet, "/api/v1/symbols?file_id=file:a.go")
	if symbols["file_id"] != "file:a.go" || symbols["symbols"].([]any)[0].(map[string]any)["id"] != "function:a.go#Run" {
		t.Fatalf("symbols = %+v", symbols)
	}
	_, deps, _ := request(t, handler, http.MethodGet, "/api/v1/dependencies?node_id=file:a.go")
	if !reflect.DeepEqual(deps["dependencies"], []any{"package:B"}) {
		t.Fatalf("dependencies = %+v", deps)
	}
	_, dependents, _ := request(t, handler, http.MethodGet, "/api/v1/dependents?node_id=package:B")
	if !reflect.DeepEqual(dependents["dependents"], []any{"file:a.go"}) {
		t.Fatalf("dependents = %+v", dependents)
	}
	_, path, _ := request(t, handler, http.MethodGet, "/api/v1/path?from_id=file:c.go&to_id=package:B")
	if !reflect.DeepEqual(path["path"], []any{}) {
		t.Fatalf("canonical path = %+v", path)
	}
	_, impact, _ := request(t, handler, http.MethodGet, "/api/v1/impact?package_id=package:B")
	wantAffected := []any{map[string]any{"id": "package:A", "distance": float64(1)}, map[string]any{"id": "package:C", "distance": float64(2)}}
	if !reflect.DeepEqual(impact["affected"], wantAffected) || impact["coverage_status"] != "completed_with_gaps" || impact["incomplete"] != true || len(impact["graph"].(map[string]any)["edges"].([]any)) != 2 {
		t.Fatalf("impact = %+v", impact)
	}
	_, canonicalAgain, _ := request(t, handler, http.MethodGet, "/api/v1/graph")
	if !reflect.DeepEqual(canonical, canonicalAgain) {
		t.Fatal("projection or impact changed canonical graph")
	}
}

func TestHandlerErrorsAndEmptyArrays(t *testing.T) {
	handler := NewHandler(fixtureSnapshot(t))
	for _, test := range []struct {
		method string
		target string
		status int
		code   string
	}{
		{"GET", "/api/v1/path", 400, "bad_request"},
		{"GET", "/api/v1/path?from_id=file:a.go&to_id=missing", 404, "node_not_found"},
		{"GET", "/api/v1/symbols?file_id=missing", 404, "node_not_found"},
		{"GET", "/api/v1/symbols?file_id=package:A", 400, "wrong_node_kind"},
		{"GET", "/api/v1/impact?package_id=file:a.go", 400, "wrong_node_kind"},
		{"GET", "/api/v1/dependencies?node_id=missing", 404, "node_not_found"},
		{"GET", "/api/v1/path?from_id=x%ZZ&to_id=package:B", 400, "bad_request"},
		{"GET", "/api/v1/symbols?file_id=file:a.go&file_id=file:c.go", 400, "bad_request"},
		{"POST", "/api/v1/graph", 405, "method_not_allowed"},
		{"GET", "/api/v1/does-not-exist", 404, "route_not_found"},
	} {
		status, body, _ := request(t, handler, test.method, test.target)
		if status != test.status || body["error"].(map[string]any)["code"] != test.code {
			t.Fatalf("%s %s = %d %+v", test.method, test.target, status, body)
		}
	}
	for _, test := range []struct{ target, field string }{
		{"/api/v1/symbols?file_id=file:c.go", "symbols"},
		{"/api/v1/dependencies?node_id=package:D", "dependencies"},
		{"/api/v1/dependents?node_id=package:C", "dependents"},
		{"/api/v1/path?from_id=package:B&to_id=package:D", "path"},
		{"/api/v1/impact?package_id=package:C", "affected"},
	} {
		status, body, _ := request(t, handler, "GET", test.target)
		if status != 200 || !reflect.DeepEqual(body[test.field], []any{}) {
			t.Fatalf("%s empty %s = %+v", test.target, test.field, body)
		}
	}
	complete := fixtureSnapshot(t)
	complete.Status = "complete"
	complete.Result.Coverage.Status = "complete"
	complete.Result.Issues = nil
	_, scan, _ := request(t, NewHandler(complete), "GET", "/api/v1/scan")
	_, impact, _ := request(t, NewHandler(complete), "GET", "/api/v1/impact?package_id=package:D")
	if !reflect.DeepEqual(scan["issues"], []any{}) || impact["incomplete"] != false || !reflect.DeepEqual(impact["affected"], []any{}) {
		t.Fatalf("empty issues or complete impact = %+v %+v", scan, impact)
	}
}

func TestHandlerKeepsLoadedSnapshotAfterNewScanAndSourceRemoval(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.go")
	if err := os.WriteFile(source, []byte("package example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	first := fixtureSnapshot(t).Result
	firstID, err := store.Save(root, first)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(firstID.ID)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(loaded)
	second := fixtureSnapshot(t).Result
	var secondEdges []graph.Edge
	for _, edge := range second.Graph.Edges {
		if edge != (graph.Edge{Kind: graph.Imports, From: "file:a.go", To: "package:B"}) {
			secondEdges = append(secondEdges, edge)
		}
	}
	second.Graph.Edges = secondEdges
	if _, err := store.Save(root, second); err != nil {
		t.Fatal(err)
	}
	store.Close()
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"/api/v1/scan", "/api/v1/graph", "/api/v1/packages", "/api/v1/symbols?file_id=file:a.go",
		"/api/v1/dependencies?node_id=file:a.go", "/api/v1/dependents?node_id=package:B",
		"/api/v1/path?from_id=file:a.go&to_id=package:B", "/api/v1/impact?package_id=package:B",
	} {
		status, body, _ := request(t, handler, "GET", target)
		if status != 200 {
			t.Fatalf("%s after source removal = %d %+v", target, status, body)
		}
		if target == "/api/v1/scan" && body["id"] != firstID.ID {
			t.Fatalf("handler changed snapshots: %+v", body)
		}
		if target == "/api/v1/dependencies?node_id=file:a.go" && !reflect.DeepEqual(body["dependencies"], []any{"package:B"}) {
			t.Fatalf("handler changed graph: %+v", body)
		}
	}
}

func TestQueryIDsWithReservedCharacters(t *testing.T) {
	handler := NewHandler(storage.Snapshot{Result: analysis.Result{Graph: graph.Graph{Nodes: []graph.Node{{ID: "file:internal/auth/service.go#part", Kind: graph.File}}}}})
	status, body, _ := request(t, handler, "GET", "/api/v1/symbols?file_id=file%3Ainternal%2Fauth%2Fservice.go%23part")
	if status != 200 || body["file_id"] != "file:internal/auth/service.go#part" || !reflect.DeepEqual(body["symbols"], []any{}) {
		t.Fatalf("encoded node ID = %d %+v", status, body)
	}
	if strings.Contains(body["file_id"].(string), "%") {
		t.Fatal("node ID was not decoded")
	}
}
