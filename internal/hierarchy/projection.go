package hierarchy

import (
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
)

type Evidence struct {
	Edge    graph.Edge
	Sources []graph.Edge
}

type Projection struct {
	Graph    graph.Graph
	Evidence []Evidence
}

func ProjectPackages(g graph.Graph) (Projection, error) {
	h, err := New(g)
	if err != nil {
		return Projection{}, err
	}
	var packages []graph.Node
	for _, node := range g.Nodes {
		if node.Kind == graph.Package {
			packages = append(packages, node)
		}
	}
	support := make(map[graph.Edge]map[graph.Edge]bool)
	for _, edge := range g.Edges {
		if edge.Kind != graph.Imports || h.nodes[edge.From].Kind != graph.File || h.nodes[edge.To].Kind != graph.Package {
			continue
		}
		parentID, ok := h.parents[edge.From]
		if !ok || h.nodes[parentID].Kind != graph.Package {
			continue
		}
		derived := graph.Edge{Kind: graph.Imports, From: parentID, To: edge.To}
		if support[derived] == nil {
			support[derived] = make(map[graph.Edge]bool)
		}
		support[derived][edge] = true
	}
	var edges []graph.Edge
	for edge := range support {
		edges = append(edges, edge)
	}
	projected, err := graph.Build(graph.Fragment{Nodes: packages, Edges: edges})
	if err != nil {
		return Projection{}, err
	}
	result := Projection{Graph: projected}
	for _, edge := range projected.Edges {
		evidence := Evidence{Edge: edge}
		for source := range support[edge] {
			evidence.Sources = append(evidence.Sources, source)
		}
		sort.Slice(evidence.Sources, func(i, j int) bool {
			a, b := evidence.Sources[i], evidence.Sources[j]
			if a.From != b.From {
				return a.From < b.From
			}
			return a.To < b.To
		})
		result.Evidence = append(result.Evidence, evidence)
	}
	return result, nil
}
