package query

import (
	"fmt"
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
)

func Dependencies(g graph.Graph, id string) ([]string, error) {
	if err := requireNode(g, id); err != nil {
		return nil, err
	}
	var result []string
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports && edge.From == id {
			result = append(result, edge.To)
		}
	}
	sort.Strings(result)
	return result, nil
}

func Dependents(g graph.Graph, id string) ([]string, error) {
	if err := requireNode(g, id); err != nil {
		return nil, err
	}
	var result []string
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports && edge.To == id {
			result = append(result, edge.From)
		}
	}
	sort.Strings(result)
	return result, nil
}

// Path returns nil when both nodes exist but no directed IMPORTS path connects them.
func Path(g graph.Graph, from, to string) ([]string, error) {
	if err := requireNode(g, from); err != nil {
		return nil, err
	}
	if err := requireNode(g, to); err != nil {
		return nil, err
	}
	if from == to {
		return []string{from}, nil
	}
	adjacent := make(map[string][]string)
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports {
			adjacent[edge.From] = append(adjacent[edge.From], edge.To)
		}
	}
	for id := range adjacent {
		sort.Strings(adjacent[id])
	}
	visited := map[string]bool{from: true}
	previous := make(map[string]string)
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacent[current] {
			if visited[next] {
				continue
			}
			visited[next] = true
			previous[next] = current
			if next == to {
				path := []string{to}
				for id := to; id != from; {
					id = previous[id]
					path = append(path, id)
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path, nil
			}
			queue = append(queue, next)
		}
	}
	return nil, nil
}

func requireNode(g graph.Graph, id string) error {
	for _, node := range g.Nodes {
		if node.ID == id {
			return nil
		}
	}
	return fmt.Errorf("node %q not found", id)
}
