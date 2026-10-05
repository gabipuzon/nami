package hierarchy

import (
	"fmt"
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
)

type Hierarchy struct {
	nodes    map[string]graph.Node
	parents  map[string]string
	children map[string][]string
}

func New(g graph.Graph) (*Hierarchy, error) {
	h := &Hierarchy{
		nodes: make(map[string]graph.Node), parents: make(map[string]string), children: make(map[string][]string),
	}
	for _, node := range g.Nodes {
		h.nodes[node.ID] = node
	}
	for _, edge := range g.Edges {
		if edge.Kind != graph.Contains {
			continue
		}
		if _, ok := h.nodes[edge.From]; !ok {
			return nil, fmt.Errorf("containment parent %q not found", edge.From)
		}
		if _, ok := h.nodes[edge.To]; !ok {
			return nil, fmt.Errorf("containment child %q not found", edge.To)
		}
		if parent, ok := h.parents[edge.To]; ok {
			if parent != edge.From {
				return nil, fmt.Errorf("node %q has multiple containment parents", edge.To)
			}
			continue
		}
		h.parents[edge.To] = edge.From
		h.children[edge.From] = append(h.children[edge.From], edge.To)
	}
	ids := make([]string, 0, len(h.nodes))
	for id := range h.nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		seen := make(map[string]bool)
		for current := id; current != ""; current = h.parents[current] {
			if seen[current] {
				return nil, fmt.Errorf("containment cycle involving %q", current)
			}
			seen[current] = true
		}
	}
	for childID, parentID := range h.parents {
		parent, child := h.nodes[parentID], h.nodes[childID]
		if parent.Kind != graph.Module || child.Kind != graph.Package {
			if parent.Kind != graph.Package || child.Kind != graph.File {
				return nil, fmt.Errorf("invalid containment %s %q -> %s %q", parent.Kind, parentID, child.Kind, childID)
			}
		}
	}
	for id := range h.children {
		sort.Strings(h.children[id])
	}
	return h, nil
}

func (h *Hierarchy) Parent(id string) (string, bool, error) {
	if _, ok := h.nodes[id]; !ok {
		return "", false, fmt.Errorf("node %q not found", id)
	}
	parent, ok := h.parents[id]
	return parent, ok, nil
}

func (h *Hierarchy) Children(id string) ([]string, error) {
	if _, ok := h.nodes[id]; !ok {
		return nil, fmt.Errorf("node %q not found", id)
	}
	return append([]string(nil), h.children[id]...), nil
}
