package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
)

// ViewIndex describes saved facts only. It is built once per loaded snapshot.
type ViewIndex struct {
	Graph          graph.Graph
	Projection     hierarchy.Projection
	EvidenceTotals map[graph.Edge]int
	Nodes          map[string]graph.Node
	Parents        map[string]string
	Children       map[string][]graph.Node
}

func NewViewIndex(g graph.Graph) (*ViewIndex, error) {
	tree, err := hierarchy.New(g)
	if err != nil {
		return nil, err
	}
	projection, err := hierarchy.ProjectPackages(g)
	if err != nil {
		return nil, err
	}
	v := &ViewIndex{Graph: g, Projection: projection, EvidenceTotals: map[graph.Edge]int{}, Nodes: map[string]graph.Node{}, Parents: map[string]string{}, Children: map[string][]graph.Node{}}
	for _, item := range projection.Evidence {
		v.EvidenceTotals[item.Edge] = len(item.Sources)
	}
	for _, n := range g.Nodes {
		v.Nodes[n.ID] = n
	}
	for _, n := range g.Nodes {
		if parent, ok, _ := tree.Parent(n.ID); ok {
			v.Parents[n.ID] = parent
			v.Children[parent] = append(v.Children[parent], n)
		}
	}
	for _, nodes := range v.Children {
		sortNodes(nodes)
	}
	return v, nil
}
func sortNodes(nodes []graph.Node) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Path != nodes[j].Path {
			return nodes[i].Path < nodes[j].Path
		}
		return nodes[i].ID < nodes[j].ID
	})
}
func pageNodes(nodes []graph.Node, offset int) NeighborPage {
	start := min(offset, len(nodes))
	return NeighborPage{Nodes: append([]graph.Node{}, nodes[start:min(start+20, len(nodes))]...), Offset: offset, Total: len(nodes)}
}
func (v *ViewIndex) ChildrenPage(id string, offset int) (NeighborPage, error) {
	if offset < 0 {
		return NeighborPage{}, fmt.Errorf("offset must be nonnegative")
	}
	if _, ok := v.Nodes[id]; !ok {
		return NeighborPage{}, fmt.Errorf("node not found")
	}
	return pageNodes(v.Children[id], offset), nil
}
func (v *ViewIndex) SearchPage(term string, offset int) (NeighborPage, error) {
	if offset < 0 {
		return NeighborPage{}, fmt.Errorf("offset must be nonnegative")
	}
	term = strings.ToLower(strings.TrimSpace(term))
	nodes := []graph.Node{}
	for _, n := range v.Graph.Nodes {
		if n.Kind != graph.Module && (strings.Contains(strings.ToLower(n.Name), term) || strings.Contains(strings.ToLower(n.Path), term) || strings.Contains(strings.ToLower(n.ID), term)) {
			nodes = append(nodes, n)
		}
	}
	sortNodes(nodes)
	return pageNodes(nodes, offset), nil
}
func (v *ViewIndex) AncestorIDs(ids map[string]bool) {
	for id := range ids {
		for parent := v.Parents[id]; parent != ""; parent = v.Parents[parent] {
			ids[parent] = true
		}
	}
}

// Subgraph includes only requested canonical relationships and their containment.
func (v *ViewIndex) Subgraph(ids map[string]bool, relationships []graph.Edge) graph.Graph {
	for _, e := range relationships {
		ids[e.From] = true
		ids[e.To] = true
	}
	v.AncestorIDs(ids)
	nodes := []graph.Node{}
	edges := append([]graph.Edge{}, relationships...)
	for _, n := range v.Graph.Nodes {
		if ids[n.ID] {
			nodes = append(nodes, n)
		}
	}
	for _, e := range v.Graph.Edges {
		if e.Kind == graph.Contains && ids[e.From] && ids[e.To] {
			edges = append(edges, e)
		}
	}
	g, _ := graph.Build(graph.Fragment{Nodes: nodes, Edges: edges})
	return g
}
func (v *ViewIndex) Overview() graph.Graph {
	ids := map[string]bool{}
	for _, n := range v.Graph.Nodes {
		if n.Kind == graph.Module || n.Kind == graph.Package || (n.Kind == graph.File && v.Parents[n.ID] == "") {
			ids[n.ID] = true
		}
	}
	// Imports outside the package projection still belong in the overview. Keep
	// their exact file facts and containment context, including packaged endpoints.
	projected := map[graph.Edge]bool{}
	for _, item := range v.Projection.Evidence {
		for _, source := range item.Sources {
			projected[source] = true
		}
	}
	relationships := []graph.Edge{}
	for _, e := range v.Graph.Edges {
		if e.Kind == graph.Imports && !projected[e] {
			relationships = append(relationships, e)
		}
	}

	return v.Subgraph(ids, relationships)
}
func (v *ViewIndex) PackageDetail(id string) (graph.Graph, []hierarchy.Evidence, error) {
	n, ok := v.Nodes[id]
	if !ok || n.Kind != graph.Package {
		return graph.Graph{}, nil, fmt.Errorf("package_id must identify a PACKAGE")
	}
	ids := map[string]bool{id: true}
	files := map[string]bool{}
	for _, child := range v.Children[id] {
		if child.Kind == graph.File {
			ids[child.ID] = true
			files[child.ID] = true
		}
	}
	relationships := []graph.Edge{}
	for _, e := range v.Graph.Edges {
		if e.Kind != graph.Contains && (files[e.From] || files[e.To]) {
			relationships = append(relationships, e)
		}
	}
	evidence := []hierarchy.Evidence{}
	for _, item := range v.Projection.Evidence {
		if item.Edge.From == id || item.Edge.To == id {
			evidence = append(evidence, item)
			relationships = append(relationships, item.Sources...)
		}
	}
	return v.Subgraph(ids, relationships), evidence, nil
}
func (v *ViewIndex) RelatedPage(id string, incoming bool, offset int) (NeighborPage, error) {
	n, ok := v.Nodes[id]
	if !ok || offset < 0 {
		return NeighborPage{}, fmt.Errorf("invalid node or offset")
	}
	g := v.Graph
	if n.Kind == graph.Package {
		g = v.Projection.Graph
	}
	ids := map[string]bool{}
	for _, e := range g.Edges {
		if e.Kind != graph.Contains {
			if incoming && e.To == id {
				ids[e.From] = true
			}
			if !incoming && e.From == id {
				ids[e.To] = true
			}
		}
	}
	nodes := []graph.Node{}
	for id := range ids {
		nodes = append(nodes, v.Nodes[id])
	}
	sortNodes(nodes)
	return pageNodes(nodes, offset), nil
}

// SubsetEvidence associates returned canonical imports with their proven projection.
// A partial view must not mistake unloaded supporting evidence for an absent projection.
func (v *ViewIndex) SubsetEvidence(g graph.Graph) []hierarchy.Evidence {
	known := map[graph.Edge]bool{}
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports {
			known[edge] = true
		}
	}
	evidence := []hierarchy.Evidence{}
	for _, item := range v.Projection.Evidence {
		sources := []graph.Edge{}
		for _, source := range item.Sources {
			if known[source] {
				sources = append(sources, source)
			}
		}
		if len(sources) > 0 {
			evidence = append(evidence, hierarchy.Evidence{Edge: item.Edge, Sources: sources})
		}
	}
	return evidence
}
