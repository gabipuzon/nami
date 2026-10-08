package query

import (
	"fmt"
	"sort"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
)

type EvidencePage struct {
	Items    []graph.SourceEvidence `json:"items"`
	Total    int                    `json:"total"`
	Offset   int                    `json:"offset"`
	PageSize int                    `json:"page_size"`
	Recorded bool                   `json:"recorded"`
}

func SelectEvidence(g graph.Graph, evidence []graph.SourceEvidence, edge graph.Edge, scope string, offset int) (EvidencePage, error) {
	if offset < 0 {
		return EvidencePage{}, fmt.Errorf("offset must be nonnegative")
	}
	if scope != "canonical" && scope != "package" {
		return EvidencePage{}, fmt.Errorf("scope must be canonical or package")
	}
	facts := map[graph.Edge]bool{}
	exists := false
	if scope == "package" {
		projection, err := hierarchy.ProjectPackages(g)
		if err != nil {
			return EvidencePage{}, err
		}
		for _, item := range projection.Evidence {
			if item.Edge == edge {
				exists = true
				for _, source := range item.Sources {
					facts[source] = true
				}
			}
		}
	} else {
		for _, item := range g.Edges {
			if item == edge {
				exists = true
				facts[item] = true
			}
		}
	}
	if !exists {
		return EvidencePage{}, fmt.Errorf("relationship not found")
	}
	all := []graph.SourceEvidence{}
	for _, item := range evidence {
		if facts[item.Edge] {
			all = append(all, item)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Edge.To != b.Edge.To {
			return a.Edge.To < b.Edge.To
		}
		return a.Snippet < b.Snippet
	})
	start := min(offset, len(all))
	return EvidencePage{Items: all[start:min(start+20, len(all))], Total: len(all), Offset: offset, PageSize: 20, Recorded: len(all) > 0}, nil
}

// SupportingFacts also works for legacy scans without source locations.
func SupportingFacts(g graph.Graph, edge graph.Edge, scope string, offset int) ([]graph.Edge, int, error) {
	if offset < 0 {
		return nil, 0, fmt.Errorf("offset must be nonnegative")
	}
	facts := []graph.Edge{}
	if scope == "package" {
		projection, err := hierarchy.ProjectPackages(g)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range projection.Evidence {
			if item.Edge == edge {
				facts = append(facts, item.Sources...)
			}
		}
	} else if scope == "canonical" {
		for _, item := range g.Edges {
			if item == edge {
				facts = append(facts, item)
			}
		}
	} else {
		return nil, 0, fmt.Errorf("scope must be canonical or package")
	}
	if len(facts) == 0 {
		return nil, 0, fmt.Errorf("relationship not found")
	}
	sort.Slice(facts, func(i, j int) bool {
		a, b := facts[i], facts[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Kind < b.Kind
	})
	start := min(offset, len(facts))
	return facts[start:min(start+20, len(facts))], len(facts), nil
}
