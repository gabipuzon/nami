package query

import (
	"fmt"
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
)

const NeighborhoodPageSize = 20

type NeighborPage struct {
	Nodes  []graph.Node
	Total  int
	Offset int
}

type Neighborhood struct {
	Focal        graph.Node
	Graph        graph.Graph
	Dependencies NeighborPage
	Dependents   NeighborPage
	Evidence     []hierarchy.Evidence
}

// SelectNeighborhood pages each direction independently. Only selected-direction
// incident imports are returned; an opposite direction on another page stays out.
func SelectNeighborhood(canonical graph.Graph, id, scope string, dependencyOffset, dependentOffset int) (Neighborhood, error) {
	if dependencyOffset < 0 || dependentOffset < 0 {
		return Neighborhood{}, fmt.Errorf("offsets must be nonnegative")
	}
	if scope != "canonical" && scope != "package" {
		return Neighborhood{}, fmt.Errorf("scope must be canonical or package")
	}
	g := canonical
	var evidence []hierarchy.Evidence
	if scope == "package" {
		projection, err := hierarchy.ProjectPackages(canonical)
		if err != nil {
			return Neighborhood{}, err
		}
		g, evidence = projection.Graph, projection.Evidence
	}
	nodes := make(map[string]graph.Node, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes[node.ID] = node
	}
	focal, ok := nodes[id]
	if !ok {
		return Neighborhood{}, fmt.Errorf("node %q not found in %s scope", id, scope)
	}
	if scope == "canonical" && focal.Kind != graph.File && focal.Kind != graph.Package {
		return Neighborhood{}, fmt.Errorf("focus requires a FILE or PACKAGE")
	}
	outgoing, incoming := map[string]bool{}, map[string]bool{}
	for _, edge := range g.Edges {
		if edge.Kind != graph.Imports {
			continue
		}
		if edge.From == id {
			outgoing[edge.To] = true
		}
		if edge.To == id {
			incoming[edge.From] = true
		}
	}
	page := func(ids map[string]bool, offset int) NeighborPage {
		all := make([]graph.Node, 0, len(ids))
		for id := range ids {
			all = append(all, nodes[id])
		}
		sort.Slice(all, func(i, j int) bool {
			if all[i].Path != all[j].Path {
				return all[i].Path < all[j].Path
			}
			return all[i].ID < all[j].ID
		})
		start := min(offset, len(all))
		return NeighborPage{Nodes: all[start:min(start+NeighborhoodPageSize, len(all))], Total: len(all), Offset: offset}
	}
	result := Neighborhood{Focal: focal, Dependencies: page(outgoing, dependencyOffset), Dependents: page(incoming, dependentOffset)}
	selectedOut, selectedIn := map[string]bool{}, map[string]bool{}
	visible := map[string]bool{id: true}
	for _, n := range result.Dependencies.Nodes {
		selectedOut[n.ID] = true
		visible[n.ID] = true
	}
	for _, n := range result.Dependents.Nodes {
		selectedIn[n.ID] = true
		visible[n.ID] = true
	}
	facts := map[graph.Edge]bool{}
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports && ((edge.From == id && selectedOut[edge.To]) || (edge.To == id && selectedIn[edge.From])) {
			facts[edge] = true
		}
	}
	fragment := graph.Fragment{}
	for id := range visible {
		fragment.Nodes = append(fragment.Nodes, nodes[id])
	}
	for edge := range facts {
		fragment.Edges = append(fragment.Edges, edge)
	}
	var err error
	result.Graph, err = graph.Build(fragment)
	if err != nil {
		return Neighborhood{}, err
	}
	result.Evidence = []hierarchy.Evidence{}
	for _, item := range evidence {
		if facts[item.Edge] {
			result.Evidence = append(result.Evidence, item)
		}
	}
	return result, nil
}
