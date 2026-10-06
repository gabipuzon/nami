package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
	"github.com/gabipuzon/nami/internal/impact"
	"github.com/gabipuzon/nami/internal/query"
	"github.com/gabipuzon/nami/internal/storage"
)

type nodeJSON struct {
	ID   string         `json:"id"`
	Kind graph.NodeKind `json:"kind"`
	Path string         `json:"path"`
	Name string         `json:"name"`
}

type edgeJSON struct {
	Kind graph.EdgeKind `json:"kind"`
	From string         `json:"from"`
	To   string         `json:"to"`
}

type graphJSON struct {
	Nodes []nodeJSON `json:"nodes"`
	Edges []edgeJSON `json:"edges"`
}

type evidenceJSON struct {
	Edge    edgeJSON   `json:"edge"`
	Sources []edgeJSON `json:"sources"`
}

type coverageJSON struct {
	FilesDiscovered         int `json:"files_discovered"`
	SupportedSourceFiles    int `json:"supported_source_files"`
	FilesAnalyzed           int `json:"files_analyzed"`
	FilesSkipped            int `json:"files_skipped"`
	FilesFailed             int `json:"files_failed"`
	ImportsDiscovered       int `json:"imports_discovered"`
	InternalImportsResolved int `json:"internal_imports_resolved"`
	StandardLibraryImports  int `json:"standard_library_imports"`
	ExternalImports         int `json:"external_imports"`
	UnresolvedImports       int `json:"unresolved_imports"`
	CgoImports              int `json:"cgo_imports"`
	UnclassifiedImports     int `json:"unclassified_imports"`
}

type issueJSON struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Import string `json:"import"`
	Reason string `json:"reason"`
}

type affectedJSON struct {
	ID       string `json:"id"`
	Distance int    `json:"distance"`
}

type handler struct {
	snapshot storage.Snapshot
	nodes    map[string]graph.Node
}

// NewHandler serves one already loaded snapshot and never accesses its store.
func NewHandler(snapshot storage.Snapshot) http.Handler {
	nodes := make(map[string]graph.Node, len(snapshot.Result.Graph.Nodes))
	for _, node := range snapshot.Result.Graph.Nodes {
		nodes[node.ID] = node
	}
	return &handler{snapshot: snapshot, nodes: nodes}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/v1/health", "/api/v1/scan", "/api/v1/graph", "/api/v1/packages",
		"/api/v1/symbols", "/api/v1/dependencies", "/api/v1/dependents", "/api/v1/path", "/api/v1/impact":
	default:
		writeError(w, http.StatusNotFound, "route_not_found", "API route not found")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed")
		return
	}
	params, err := parseParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	switch r.URL.Path {
	case "/api/v1/health":
		writeJSON(w, http.StatusOK, struct {
			Status string `json:"status"`
		}{Status: "ok"})
	case "/api/v1/scan":
		h.scan(w)
	case "/api/v1/graph":
		writeJSON(w, http.StatusOK, convertGraph(h.snapshot.Result.Graph))
	case "/api/v1/packages":
		projection, err := hierarchy.ProjectPackages(h.snapshot.Result.Graph)
		if err != nil {
			writeInternalError(w)
			return
		}
		evidence := make([]evidenceJSON, 0, len(projection.Evidence))
		for _, item := range projection.Evidence {
			sources := make([]edgeJSON, 0, len(item.Sources))
			for _, source := range item.Sources {
				sources = append(sources, convertEdge(source))
			}
			evidence = append(evidence, evidenceJSON{Edge: convertEdge(item.Edge), Sources: sources})
		}
		writeJSON(w, http.StatusOK, struct {
			Graph    graphJSON      `json:"graph"`
			Evidence []evidenceJSON `json:"evidence"`
		}{convertGraph(projection.Graph), evidence})
	case "/api/v1/symbols":
		h.symbols(w, params)
	case "/api/v1/dependencies":
		h.related(w, params, true)
	case "/api/v1/dependents":
		h.related(w, params, false)
	case "/api/v1/path":
		h.path(w, params)
	case "/api/v1/impact":
		h.impact(w, params)
	}
}

func (h *handler) scan(w http.ResponseWriter) {
	s := h.snapshot
	c := s.Result.Coverage
	issues := make([]issueJSON, 0, len(s.Result.Issues))
	for _, issue := range s.Result.Issues {
		issues = append(issues, convertIssue(issue))
	}
	writeJSON(w, http.StatusOK, struct {
		ID        string       `json:"id"`
		Root      string       `json:"root"`
		CreatedAt string       `json:"created_at"`
		Status    string       `json:"status"`
		Coverage  coverageJSON `json:"coverage"`
		Issues    []issueJSON  `json:"issues"`
	}{s.ID, s.Root, s.CreatedAt.Format(time.RFC3339Nano), s.Status, coverageJSON{
		FilesDiscovered: c.FilesDiscovered, SupportedSourceFiles: c.SupportedSourceFiles,
		FilesAnalyzed: c.FilesAnalyzed, FilesSkipped: c.FilesSkipped, FilesFailed: c.FilesFailed,
		ImportsDiscovered: c.ImportsDiscovered, InternalImportsResolved: c.InternalResolved,
		StandardLibraryImports: c.StandardLibrary, ExternalImports: c.External,
		UnresolvedImports: c.Unresolved, CgoImports: c.Cgo, UnclassifiedImports: c.Unclassified,
	}, issues})
}

func (h *handler) symbols(w http.ResponseWriter, params map[string][]string) {
	id, ok := required(w, params, "file_id")
	if !ok || !h.requireNode(w, id, graph.File) {
		return
	}
	tree, err := hierarchy.New(h.snapshot.Result.Graph)
	if err != nil {
		writeInternalError(w)
		return
	}
	children, err := tree.Children(id)
	if err != nil {
		writeInternalError(w)
		return
	}
	symbols := make([]nodeJSON, 0, len(children))
	for _, child := range children {
		symbols = append(symbols, convertNode(h.nodes[child]))
	}
	writeJSON(w, http.StatusOK, struct {
		FileID  string     `json:"file_id"`
		Symbols []nodeJSON `json:"symbols"`
	}{id, symbols})
}

func (h *handler) related(w http.ResponseWriter, params map[string][]string, dependencies bool) {
	id, ok := required(w, params, "node_id")
	if !ok || !h.requireNode(w, id, "") {
		return
	}
	var ids []string
	var err error
	if dependencies {
		ids, err = query.Dependencies(h.snapshot.Result.Graph, id)
	} else {
		ids, err = query.Dependents(h.snapshot.Result.Graph, id)
	}
	if err != nil {
		writeInternalError(w)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	if dependencies {
		writeJSON(w, http.StatusOK, struct {
			NodeID       string   `json:"node_id"`
			Dependencies []string `json:"dependencies"`
		}{id, ids})
	} else {
		writeJSON(w, http.StatusOK, struct {
			NodeID     string   `json:"node_id"`
			Dependents []string `json:"dependents"`
		}{id, ids})
	}
}

func (h *handler) path(w http.ResponseWriter, params map[string][]string) {
	from, ok := required(w, params, "from_id")
	if !ok {
		return
	}
	to, ok := required(w, params, "to_id")
	if !ok || !h.requireNode(w, from, "") || !h.requireNode(w, to, "") {
		return
	}
	ids, err := query.Path(h.snapshot.Result.Graph, from, to)
	if err != nil {
		writeInternalError(w)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, struct {
		FromID string   `json:"from_id"`
		ToID   string   `json:"to_id"`
		Path   []string `json:"path"`
	}{from, to, ids})
}

func (h *handler) impact(w http.ResponseWriter, params map[string][]string) {
	id, ok := required(w, params, "package_id")
	if !ok || !h.requireNode(w, id, graph.Package) {
		return
	}
	projection, err := hierarchy.ProjectPackages(h.snapshot.Result.Graph)
	if err != nil {
		writeInternalError(w)
		return
	}
	result, err := impact.Analyze(projection.Graph, id)
	if err != nil {
		writeInternalError(w)
		return
	}
	affected := make([]affectedJSON, 0, len(result.Affected))
	for _, item := range result.Affected {
		affected = append(affected, affectedJSON{ID: item.ID, Distance: item.Distance})
	}
	writeJSON(w, http.StatusOK, struct {
		Target         string         `json:"target"`
		Affected       []affectedJSON `json:"affected"`
		Graph          graphJSON      `json:"graph"`
		CoverageStatus string         `json:"coverage_status"`
		Incomplete     bool           `json:"incomplete"`
	}{id, affected, convertGraph(result.Graph), h.snapshot.Status, h.snapshot.Status != "complete"})
}

func (h *handler) requireNode(w http.ResponseWriter, id string, kind graph.NodeKind) bool {
	node, ok := h.nodes[id]
	if !ok {
		writeError(w, http.StatusNotFound, "node_not_found", fmt.Sprintf("node %q not found", id))
		return false
	}
	if kind != "" && node.Kind != kind {
		writeError(w, http.StatusBadRequest, "wrong_node_kind", fmt.Sprintf("node %q is not a %s", id, kind))
		return false
	}
	return true
}

func convertNode(node graph.Node) nodeJSON {
	return nodeJSON{ID: node.ID, Kind: node.Kind, Path: node.Path, Name: node.Name}
}

func convertEdge(edge graph.Edge) edgeJSON {
	return edgeJSON{Kind: edge.Kind, From: edge.From, To: edge.To}
}

func convertGraph(g graph.Graph) graphJSON {
	nodes := make([]nodeJSON, 0, len(g.Nodes))
	edges := make([]edgeJSON, 0, len(g.Edges))
	for _, node := range g.Nodes {
		nodes = append(nodes, convertNode(node))
	}
	for _, edge := range g.Edges {
		edges = append(edges, convertEdge(edge))
	}
	return graphJSON{Nodes: nodes, Edges: edges}
}

func convertIssue(issue analysis.Issue) issueJSON {
	return issueJSON{Kind: issue.Kind, Path: issue.Path, Import: issue.Import, Reason: issue.Reason}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{code, message}})
}

func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal_error", "stored graph operation failed")
}
