package api

import (
	"net/http"
	"strconv"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/query"
)

type neighborPageJSON struct {
	Nodes  []nodeJSON `json:"nodes"`
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
}

func (h *handler) neighborhood(w http.ResponseWriter, params map[string][]string) {
	id, ok := required(w, params, "node_id")
	if !ok || !h.requireNode(w, id, "") {
		return
	}
	scope, ok := required(w, params, "scope")
	if !ok {
		return
	}
	if scope != "canonical" && scope != "package" {
		writeError(w, 400, "bad_request", "scope must be canonical or package")
		return
	}
	if scope == "package" && !h.requireNode(w, id, graph.Package) {
		return
	}
	if scope == "canonical" && h.nodes[id].Kind != graph.File && h.nodes[id].Kind != graph.Package {
		writeError(w, 400, "wrong_node_kind", "focus requires a FILE or PACKAGE")
		return
	}
	offset := func(key string) (int, bool) {
		values, exists := params[key]
		if !exists {
			return 0, true
		}
		if len(values) != 1 {
			writeError(w, 400, "bad_request", "exactly one "+key+" is allowed")
			return 0, false
		}
		n, err := strconv.Atoi(values[0])
		if err != nil || n < 0 {
			writeError(w, 400, "bad_request", key+" must be a nonnegative integer")
			return 0, false
		}
		return n, true
	}
	dependencyOffset, ok := offset("dependency_offset")
	if !ok {
		return
	}
	dependentOffset, ok := offset("dependent_offset")
	if !ok {
		return
	}
	result, err := query.SelectNeighborhood(h.snapshot.Result.Graph, id, scope, dependencyOffset, dependentOffset)
	if err != nil {
		writeInternalError(w)
		return
	}
	page := func(p query.NeighborPage) neighborPageJSON {
		nodes := make([]nodeJSON, 0, len(p.Nodes))
		for _, n := range p.Nodes {
			nodes = append(nodes, convertNode(n))
		}
		return neighborPageJSON{nodes, p.Total, p.Offset}
	}
	evidence := make([]evidenceJSON, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		sources := make([]edgeJSON, 0, len(item.Sources))
		for _, edge := range item.Sources {
			sources = append(sources, convertEdge(edge))
		}
		evidence = append(evidence, evidenceJSON{convertEdge(item.Edge), sources})
	}
	context := graph.Graph{}
	if h.viewErr == nil {
		ids := map[string]bool{}
		for _, n := range result.Graph.Nodes {
			ids[n.ID] = true
		}
		context = h.view.Subgraph(ids, nil)
	}
	writeJSON(w, 200, struct {
		Context        graphJSON        `json:"context_graph"`
		Focal          nodeJSON         `json:"focal"`
		Scope          string           `json:"scope"`
		Graph          graphJSON        `json:"graph"`
		Dependencies   neighborPageJSON `json:"dependencies"`
		Dependents     neighborPageJSON `json:"dependents"`
		Evidence       []evidenceJSON   `json:"evidence"`
		PageSize       int              `json:"page_size"`
		SnapshotID     string           `json:"snapshot_id"`
		CoverageStatus string           `json:"coverage_status"`
		Incomplete     bool             `json:"incomplete"`
		ExcludedPaths  int              `json:"excluded_paths"`
	}{convertGraph(context), convertNode(result.Focal), scope, convertGraph(result.Graph), page(result.Dependencies), page(result.Dependents), evidence, query.NeighborhoodPageSize, h.snapshot.ID, h.snapshot.Status, h.snapshot.Status != "complete", len(h.snapshot.Result.Exclusions)})
}
