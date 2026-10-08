package query

import (
	"fmt"
	"testing"

	"github.com/gabipuzon/nami/internal/graph"
)

func TestEvidencePagesOccurrencesWithoutChangingEdges(t *testing.T) {
	edge := graph.Edge{Kind: graph.Imports, From: "file:a.go", To: "package:b"}
	g := graph.Graph{Nodes: []graph.Node{{ID: "file:a.go", Kind: graph.File}, {ID: "package:a", Kind: graph.Package}, {ID: "package:b", Kind: graph.Package}}, Edges: []graph.Edge{{Kind: graph.Contains, From: "package:a", To: "file:a.go"}, edge}}
	evidence := []graph.SourceEvidence{}
	for i := 44; i >= 0; i-- {
		evidence = append(evidence, graph.SourceEvidence{Edge: edge, Path: "a.go", Line: i + 1, Snippet: fmt.Sprint(i)})
	}
	for _, scope := range []string{"canonical", "package"} {
		target := edge
		if scope == "package" {
			target.From = "package:a"
		}
		for _, offset := range []int{0, 20, 40, 60} {
			p, err := SelectEvidence(g, evidence, target, scope, offset)
			if err != nil {
				t.Fatal(err)
			}
			if p.Total != 45 || len(p.Items) != min(20, max(0, 45-offset)) {
				t.Fatal(p)
			}
			if len(p.Items) > 0 && p.Items[0].Line != offset+1 {
				t.Fatal(p)
			}
		}
	}
	if _, err := SelectEvidence(g, evidence, edge, "canonical", -1); err == nil {
		t.Fatal("negative offset")
	}
	if _, err := SelectEvidence(g, evidence, edge, "invalid", 0); err == nil {
		t.Fatal("invalid scope")
	}
	if p, err := SelectEvidence(g, nil, edge, "canonical", 0); err != nil || p.Recorded || p.Total != 0 {
		t.Fatal(p, err)
	}
	if len(g.Edges) != 2 {
		t.Fatal("query changed graph")
	}
}
