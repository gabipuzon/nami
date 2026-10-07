package graph

import (
	"fmt"
	"sort"
)

type NodeKind string

const (
	Module    NodeKind = "MODULE"
	Package   NodeKind = "PACKAGE"
	File      NodeKind = "FILE"
	Function  NodeKind = "FUNCTION"
	Method    NodeKind = "METHOD"
	Struct    NodeKind = "STRUCT"
	Interface NodeKind = "INTERFACE"
	Type      NodeKind = "TYPE"
	Variable  NodeKind = "VARIABLE"
	Constant  NodeKind = "CONSTANT"
)

func IsDeclaration(kind NodeKind) bool {
	switch kind {
	case Function, Method, Struct, Interface, Type, Variable, Constant:
		return true
	default:
		return false
	}
}

type EdgeKind string

const (
	Contains   EdgeKind = "CONTAINS"
	Imports    EdgeKind = "IMPORTS"
	UsesExport EdgeKind = "USES_EXPORT"
)

type Node struct {
	ID              string
	Kind            NodeKind
	Path            string
	Name            string
	ImportCount     int
	ExportCount     int
	HasSourceCounts bool
}

type Edge struct {
	Kind EdgeKind
	From string
	To   string
}

type Fragment struct {
	Nodes []Node
	Edges []Edge
}

type Graph struct {
	Nodes []Node
	Edges []Edge
}

// Build owns graph identity checks, deduplication, and stable ordering.
func Build(fragments ...Fragment) (Graph, error) {
	nodes := make(map[string]Node)
	edges := make(map[Edge]bool)
	for _, fragment := range fragments {
		for _, node := range fragment.Nodes {
			if node.ID == "" {
				return Graph{}, fmt.Errorf("graph node has empty ID")
			}
			if existing, ok := nodes[node.ID]; ok && existing != node {
				return Graph{}, fmt.Errorf("conflicting graph node %q", node.ID)
			}
			nodes[node.ID] = node
		}
		for _, edge := range fragment.Edges {
			edges[edge] = true
		}
	}

	graph := Graph{}
	for _, node := range nodes {
		graph.Nodes = append(graph.Nodes, node)
	}
	sort.Slice(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].Kind != graph.Nodes[j].Kind {
			return graph.Nodes[i].Kind < graph.Nodes[j].Kind
		}
		return graph.Nodes[i].ID < graph.Nodes[j].ID
	})
	for edge := range edges {
		if _, ok := nodes[edge.From]; !ok {
			return Graph{}, fmt.Errorf("edge %s has unknown source %q", edge.Kind, edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return Graph{}, fmt.Errorf("edge %s has unknown target %q", edge.Kind, edge.To)
		}
		graph.Edges = append(graph.Edges, edge)
	}
	sort.Slice(graph.Edges, func(i, j int) bool {
		a, b := graph.Edges[i], graph.Edges[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return graph, nil
}
