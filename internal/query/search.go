package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
)

const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
)

// Search matches saved node fields only. A nil limit selects the default;
// an explicitly supplied zero is invalid rather than an unbounded search.
func Search(g graph.Graph, text string, kinds []graph.NodeKind, limit *int) ([]graph.Node, error) {
	count := DefaultSearchLimit
	if limit != nil {
		count = *limit
	}
	if count < 1 || count > MaxSearchLimit {
		return nil, fmt.Errorf("limit must be between 1 and %d", MaxSearchLimit)
	}
	allowed := make(map[graph.NodeKind]bool, len(kinds))
	for _, kind := range kinds {
		if kind != graph.Module && kind != graph.Package && kind != graph.File && !graph.IsDeclaration(kind) {
			return nil, fmt.Errorf("unknown node kind %q", kind)
		}
		allowed[kind] = true
	}
	text = strings.ToLower(text)
	nodes := make([]graph.Node, 0)
	for _, node := range g.Nodes {
		if len(allowed) > 0 && !allowed[node.Kind] {
			continue
		}
		if strings.Contains(strings.ToLower(node.Name), text) || strings.Contains(strings.ToLower(node.Path), text) || strings.Contains(strings.ToLower(node.ID), text) {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].ID < nodes[j].ID
	})
	if len(nodes) > count {
		nodes = nodes[:count]
	}
	return nodes, nil
}
