package impact

import (
	"fmt"
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
)

type Affected struct {
	ID       string
	Distance int
}

type Result struct {
	Target   string
	Affected []Affected
	Graph    graph.Graph
}

// Analyze follows only explicit imports in reverse, using breadth-first search
// so each discovered node receives its shortest dependency distance.
func Analyze(g graph.Graph, target string) (Result, error) {
	nodes := make(map[string]graph.Node, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes[node.ID] = node
	}
	if _, ok := nodes[target]; !ok {
		return Result{}, fmt.Errorf("node %q not found", target)
	}

	reverse := make(map[string][]string)
	for _, edge := range g.Edges {
		if edge.Kind == graph.Imports {
			reverse[edge.To] = append(reverse[edge.To], edge.From)
		}
	}
	for id := range reverse {
		sort.Strings(reverse[id])
	}

	distances := map[string]int{target: 0}
	queue := []string{target}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		for _, dependent := range reverse[current] {
			if _, seen := distances[dependent]; seen {
				continue
			}
			distances[dependent] = distances[current] + 1
			queue = append(queue, dependent)
		}
	}

	result := Result{Target: target}
	fragment := graph.Fragment{}
	for id, distance := range distances {
		fragment.Nodes = append(fragment.Nodes, nodes[id])
		if id != target {
			result.Affected = append(result.Affected, Affected{ID: id, Distance: distance})
		}
	}
	sort.Slice(result.Affected, func(i, j int) bool {
		a, b := result.Affected[i], result.Affected[j]
		if a.Distance != b.Distance {
			return a.Distance < b.Distance
		}
		return a.ID < b.ID
	})
	for _, edge := range g.Edges {
		if edge.Kind != graph.Imports {
			continue
		}
		if _, from := distances[edge.From]; !from {
			continue
		}
		if _, to := distances[edge.To]; to {
			fragment.Edges = append(fragment.Edges, edge)
		}
	}
	var err error
	result.Graph, err = graph.Build(fragment)
	return result, err
}
