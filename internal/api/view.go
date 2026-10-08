package api

import (
	"net/http"
	"net/url"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
	"github.com/gabipuzon/nami/internal/query"
)

type nodePageJSON struct {
	Nodes  []nodeJSON `json:"nodes"`
	Total  int        `json:"total"`
	Offset int        `json:"offset"`
}

func convertPage(p query.NeighborPage) nodePageJSON {
	nodes := []nodeJSON{}
	for _, n := range p.Nodes {
		nodes = append(nodes, convertNode(n))
	}
	return nodePageJSON{nodes, p.Total, p.Offset}
}

type projectionJSON struct {
	Graph              graphJSON      `json:"graph"`
	Evidence           []evidenceJSON `json:"evidence"`
	RelationshipTotals map[string]int `json:"relationship_totals"`
}
type detailJSON struct {
	Graph       graphJSON      `json:"graph"`
	Projection  projectionJSON `json:"package_projection"`
	ChildCounts map[string]int `json:"child_counts"`
	FileCounts  map[string]int `json:"file_counts"`
}

func edgeKey(e graph.Edge) string { return string(e.Kind) + ":" + e.From + "->" + e.To }
func (h *handler) detail(g graph.Graph, evidence []hierarchy.Evidence) detailJSON {
	provided := map[graph.Edge]bool{}
	for _, item := range evidence {
		provided[item.Edge] = true
	}
	for _, item := range h.view.SubsetEvidence(g) {
		if !provided[item.Edge] {
			evidence = append(evidence, item)
		}
	}
	counts := map[string]int{}
	fileCounts := map[string]int{}
	for _, n := range g.Nodes {
		counts[n.ID] = len(h.view.Children[n.ID])
		if n.Kind == graph.Package {
			fileCounts[n.ID] = 0
			for _, child := range h.view.Children[n.ID] {
				if child.Kind == graph.File {
					fileCounts[n.ID]++
				}
			}
		}
	}
	sources := []evidenceJSON{}
	for _, item := range evidence {
		edges := []edgeJSON{}
		for _, e := range item.Sources {
			edges = append(edges, convertEdge(e))
		}
		sources = append(sources, evidenceJSON{convertEdge(item.Edge), edges})
	}
	totals := map[string]int{}
	projectedNodes := map[string]bool{}
	projectedEdges := []graph.Edge{}
	for _, item := range evidence {
		projectedEdges = append(projectedEdges, item.Edge)
		projectedNodes[item.Edge.From] = true
		projectedNodes[item.Edge.To] = true
	}
	projected := graph.Graph{Nodes: []graph.Node{}, Edges: projectedEdges}
	for _, n := range h.view.Projection.Graph.Nodes {
		if projectedNodes[n.ID] {
			projected.Nodes = append(projected.Nodes, n)
		}
	}
	for _, item := range evidence {
		totals[edgeKey(item.Edge)] = h.view.EvidenceTotals[item.Edge]
	}

	return detailJSON{convertGraph(g), projectionJSON{convertGraph(projected), sources, totals}, counts, fileCounts}
}
func (h *handler) viewRoute(w http.ResponseWriter, params url.Values, route string) {
	if h.viewErr != nil {
		writeInternalError(w)
		return
	}
	if route == "overview" {
		counts := map[string]int{"packages": 0, "files": 0, "declarations": 0}
		for _, n := range h.snapshot.Result.Graph.Nodes {
			if n.Kind == graph.Package {
				counts["packages"]++
			}
			if n.Kind == graph.File {
				counts["files"]++
			}
			if graph.IsDeclaration(n.Kind) {
				counts["declarations"]++
			}
		}
		detail := h.detail(h.view.Overview(), nil)
		detail.Projection.Graph = convertGraph(h.view.Projection.Graph)
		for _, item := range h.view.Projection.Evidence {
			detail.Projection.RelationshipTotals[edgeKey(item.Edge)] = len(item.Sources)
		}
		writeJSON(w, 200, struct {
			detailJSON
			Counts map[string]int `json:"counts"`
		}{detail, counts})
		return
	}
	offset, err := nonnegative(params, "offset")
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	if route == "search" {
		text, ok := required(w, params, "query")
		if !ok {
			return
		}
		p, err := h.view.SearchPage(text, offset)
		if err != nil {
			writeError(w, 400, "bad_request", err.Error())
			return
		}
		writeJSON(w, 200, convertPage(p))
		return
	}
	id, ok := required(w, params, "node_id")
	if !ok {
		return
	}
	if _, exists := h.nodes[id]; !exists {
		writeError(w, 404, "node_not_found", "node not found")
		return
	}
	if route == "file-detail" {
		if !h.requireNode(w, id, graph.File) {
			return
		}
		ids := map[string]bool{id: true}
		for _, child := range h.view.Children[id] {
			ids[child.ID] = true
		}
		writeJSON(w, 200, h.detail(h.view.Subgraph(ids, nil), nil))
		return
	}
	if route == "package-detail" {
		g, evidence, err := h.view.PackageDetail(id)
		if err != nil {
			writeError(w, 400, "bad_request", err.Error())
			return
		}
		writeJSON(w, 200, h.detail(g, evidence))
		return
	}
	children, _ := h.view.ChildrenPage(id, offset)
	ids := map[string]bool{id: true}
	for _, n := range children.Nodes {
		ids[n.ID] = true
	}
	if route == "children" {
		writeJSON(w, 200, struct {
			nodePageJSON
			detailJSON
		}{convertPage(children), h.detail(h.view.Subgraph(ids, nil), nil)})
		return
	}
	dependencyOffset, err := nonnegative(params, "dependency_offset")
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	dependentOffset, err := nonnegative(params, "dependent_offset")
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	outgoing, _ := h.view.RelatedPage(id, false, dependencyOffset)
	incoming, _ := h.view.RelatedPage(id, true, dependentOffset)
	selected := map[string]bool{}
	for _, p := range []query.NeighborPage{outgoing, incoming} {
		for _, n := range p.Nodes {
			ids[n.ID] = true
			selected[n.ID] = true
		}
	}
	relationships := []graph.Edge{}
	for _, e := range h.view.Graph.Edges {
		if e.Kind != graph.Contains && ((e.From == id && selected[e.To]) || (e.To == id && selected[e.From])) {
			relationships = append(relationships, e)
		}
	}
	parent := ""
	if p, ok := h.view.Parents[id]; ok {
		parent = p
	}
	writeJSON(w, 200, struct {
		detailJSON
		Node         nodeJSON     `json:"node"`
		Parent       string       `json:"parent"`
		Children     nodePageJSON `json:"children"`
		Dependencies nodePageJSON `json:"dependencies"`
		Dependents   nodePageJSON `json:"dependents"`
	}{h.detail(h.view.Subgraph(ids, relationships), nil), convertNode(h.nodes[id]), parent, convertPage(children), convertPage(outgoing), convertPage(incoming)})
}
