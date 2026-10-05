package hierarchy

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/query"
)

func testGraph(t *testing.T) graph.Graph {
	t.Helper()
	g, err := graph.Build(graph.Fragment{
		Nodes: []graph.Node{
			{ID: "M", Kind: graph.Module},
			{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}, {ID: "C", Kind: graph.Package},
			{ID: "a.go", Kind: graph.File}, {ID: "b.go", Kind: graph.File}, {ID: "c.go", Kind: graph.File}, {ID: "d.go", Kind: graph.File},
		},
		Edges: []graph.Edge{
			{Kind: graph.Contains, From: "M", To: "B"},
			{Kind: graph.Contains, From: "M", To: "A"},
			{Kind: graph.Contains, From: "M", To: "C"},
			{Kind: graph.Contains, From: "A", To: "b.go"},
			{Kind: graph.Contains, From: "A", To: "a.go"},
			{Kind: graph.Contains, From: "A", To: "d.go"},
			{Kind: graph.Contains, From: "B", To: "c.go"},
			{Kind: graph.Imports, From: "b.go", To: "B"},
			{Kind: graph.Imports, From: "a.go", To: "B"},
			{Kind: graph.Imports, From: "d.go", To: "B"},
			{Kind: graph.Imports, From: "c.go", To: "C"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestParentAndChildren(t *testing.T) {
	h, err := New(testGraph(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, want string }{{"a.go", "A"}, {"A", "M"}} {
		parent, ok, err := h.Parent(test.id)
		if err != nil || !ok || parent != test.want {
			t.Fatalf("parent(%s) = %q, %t, %v", test.id, parent, ok, err)
		}
	}
	children, err := h.Children("M")
	if err != nil || !reflect.DeepEqual(children, []string{"A", "B", "C"}) {
		t.Fatalf("module children = %v, %v", children, err)
	}
	children, err = h.Children("A")
	if err != nil || !reflect.DeepEqual(children, []string{"a.go", "b.go", "d.go"}) {
		t.Fatalf("package children = %v, %v", children, err)
	}
	if _, _, err := h.Parent("missing"); err == nil {
		t.Fatal("missing node accepted")
	}
}

func TestInvalidContainmentRejected(t *testing.T) {
	for _, test := range []struct {
		name  string
		nodes []graph.Node
		edges []graph.Edge
		want  string
	}{
		{"multiple parents", []graph.Node{{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}, {ID: "F", Kind: graph.File}}, []graph.Edge{{Kind: graph.Contains, From: "A", To: "F"}, {Kind: graph.Contains, From: "B", To: "F"}}, "multiple containment parents"},
		{"cycle", []graph.Node{{ID: "A", Kind: graph.Package}, {ID: "B", Kind: graph.Package}}, []graph.Edge{{Kind: graph.Contains, From: "A", To: "B"}, {Kind: graph.Contains, From: "B", To: "A"}}, "containment cycle"},
		{"module contains file", []graph.Node{{ID: "M", Kind: graph.Module}, {ID: "F", Kind: graph.File}}, []graph.Edge{{Kind: graph.Contains, From: "M", To: "F"}}, "invalid containment"},
		{"file contains package", []graph.Node{{ID: "F", Kind: graph.File}, {ID: "A", Kind: graph.Package}}, []graph.Edge{{Kind: graph.Contains, From: "F", To: "A"}}, "invalid containment"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, err := graph.Build(graph.Fragment{Nodes: test.nodes, Edges: test.edges})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := New(g); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validation error = %v", err)
			}
		})
	}
}

func TestPackageProjectionKeepsEvidenceAndCanonicalGraph(t *testing.T) {
	g := testGraph(t)
	before := graph.Graph{Nodes: append([]graph.Node(nil), g.Nodes...), Edges: append([]graph.Edge(nil), g.Edges...)}
	var first Projection
	for i := 0; i < 5; i++ {
		projection, err := ProjectPackages(g)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = projection
		} else if !reflect.DeepEqual(first, projection) {
			t.Fatal("projection changed for unchanged graph")
		}
	}
	if !reflect.DeepEqual(g, before) {
		t.Fatal("projection mutated canonical graph")
	}
	if len(first.Graph.Nodes) != 3 || !reflect.DeepEqual(first.Graph.Edges, []graph.Edge{
		{Kind: graph.Imports, From: "A", To: "B"}, {Kind: graph.Imports, From: "B", To: "C"},
	}) {
		t.Fatalf("folded graph = %+v", first.Graph)
	}
	if len(first.Evidence) != 2 || !reflect.DeepEqual(first.Evidence[0], Evidence{
		Edge: graph.Edge{Kind: graph.Imports, From: "A", To: "B"},
		Sources: []graph.Edge{
			{Kind: graph.Imports, From: "a.go", To: "B"},
			{Kind: graph.Imports, From: "b.go", To: "B"},
			{Kind: graph.Imports, From: "d.go", To: "B"},
		},
	}) {
		t.Fatalf("projection evidence = %+v", first.Evidence)
	}
	dependencies, err := query.Dependencies(first.Graph, "A")
	if err != nil || !reflect.DeepEqual(dependencies, []string{"B"}) {
		t.Fatalf("folded dependencies = %v, %v", dependencies, err)
	}
	dependents, err := query.Dependents(first.Graph, "C")
	if err != nil || !reflect.DeepEqual(dependents, []string{"B"}) {
		t.Fatalf("folded dependents = %v, %v", dependents, err)
	}
	path, err := query.Path(first.Graph, "A", "C")
	if err != nil || !reflect.DeepEqual(path, []string{"A", "B", "C"}) {
		t.Fatalf("folded path = %v, %v", path, err)
	}
}

func TestOldGraphWithoutModulesCanStillFold(t *testing.T) {
	g := testGraph(t)
	var nodes []graph.Node
	var edges []graph.Edge
	for _, node := range g.Nodes {
		if node.Kind != graph.Module {
			nodes = append(nodes, node)
		}
	}
	for _, edge := range g.Edges {
		if edge.From != "M" {
			edges = append(edges, edge)
		}
	}
	old := graph.Graph{Nodes: nodes, Edges: edges}
	projection, err := ProjectPackages(old)
	if err != nil || len(projection.Graph.Edges) != 2 {
		t.Fatalf("old graph projection = %+v, %v", projection, err)
	}
}
