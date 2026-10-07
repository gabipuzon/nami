// Package mcpserver exposes saved graph facts through read-only MCP tools.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
	"github.com/gabipuzon/nami/internal/impact"
	"github.com/gabipuzon/nami/internal/query"
	"github.com/gabipuzon/nami/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type snapshotServer struct {
	snapshot   storage.Snapshot
	nodes      map[string]graph.Node
	tree       *hierarchy.Hierarchy
	projection hierarchy.Projection
}

// Load closes SQLite before constructing the server. No tool retains a store
// or accesses the repository, even if the source or database later changes.
func Load(directory, scanID string) (*mcp.Server, error) {
	store, err := storage.OpenReadOnly(directory)
	if err != nil {
		return nil, err
	}
	snapshot, loadErr := store.Load(scanID)
	closeErr := store.Close()
	if loadErr != nil {
		return nil, loadErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return New(snapshot)
}

// New takes ownership of copies of all stored slices so callers cannot mutate
// the snapshot seen by a connected client.
func New(snapshot storage.Snapshot) (*mcp.Server, error) {
	snapshot.Result.Graph.Nodes = slices.Clone(snapshot.Result.Graph.Nodes)
	snapshot.Result.Graph.Edges = slices.Clone(snapshot.Result.Graph.Edges)
	snapshot.Result.Issues = slices.Clone(snapshot.Result.Issues)
	tree, err := hierarchy.New(snapshot.Result.Graph)
	if err != nil {
		return nil, fmt.Errorf("load snapshot hierarchy: %w", err)
	}
	projection, err := hierarchy.ProjectPackages(snapshot.Result.Graph)
	if err != nil {
		return nil, fmt.Errorf("load snapshot projection: %w", err)
	}
	s := &snapshotServer{snapshot: snapshot, tree: tree, projection: projection, nodes: make(map[string]graph.Node)}
	for _, node := range snapshot.Result.Graph.Nodes {
		s.nodes[node.ID] = node
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "nami", Version: "dev"}, &mcp.ServerOptions{
		Instructions: "Read-only facts from one saved nami snapshot. Check nami_scan_info for analysis gaps. Impact means potential dependency reachability, not guaranteed breakage. USES_EXPORT is a stored file relationship, not a call graph.",
	})
	addTool(server, "nami_scan_info", "Describe the loaded snapshot, stored coverage, and analysis gaps.", inputSchema(nil), func(noArgs) (any, error) {
		return scanInfo(s.snapshot), nil
	})
	addTool(server, "nami_search_nodes", "Find saved nodes by case-insensitive substring in name, path, or ID. Ordered by name then ID; default limit 20, maximum 100.", inputSchema(map[string]any{
		"query": stringProperty("Substring to match; an empty string returns the first limited results."),
		"kinds": map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"MODULE", "PACKAGE", "FILE", "FUNCTION", "METHOD", "STRUCT", "INTERFACE", "TYPE", "VARIABLE", "CONSTANT"}}},
		"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": query.MaxSearchLimit},
	}, "query"), s.search)
	addTool(server, "nami_inspect_node", "Inspect a saved node, parent, direct children, and canonical incoming/outgoing edges. Edge kinds retain their stored meaning.", inputSchema(map[string]any{"node_id": stringProperty("Exact saved node ID.")}, "node_id"), s.inspect)
	packageSchema := inputSchema(map[string]any{"package_id": stringProperty("Exact PACKAGE node ID.")}, "package_id")
	addTool(server, "nami_package_dependencies", "Direct dependencies from nami's package projection, including supporting canonical file-import evidence.", packageSchema, func(args packageArgs) (any, error) { return s.related(args, true) })
	addTool(server, "nami_package_dependents", "Direct packages depending on the target, including supporting canonical file-import evidence.", packageSchema, func(args packageArgs) (any, error) { return s.related(args, false) })
	addTool(server, "nami_dependency_path", "Find the deterministic shortest directed IMPORTS path in canonical or package scope. Containment and USES_EXPORT do not count; no path is a successful result.", inputSchema(map[string]any{
		"from_id": stringProperty("Exact starting node ID."), "to_id": stringProperty("Exact destination node ID."),
		"scope": map[string]any{"type": "string", "enum": []string{"canonical", "package"}},
	}, "from_id", "to_id", "scope"), s.path)
	addTool(server, "nami_file_symbols", "Return only persisted declaration nodes directly owned by a FILE.", inputSchema(map[string]any{"file_id": stringProperty("Exact FILE node ID.")}, "file_id"), s.symbols)
	addTool(server, "nami_package_impact", "Return potentially affected packages through known dependency reachability, distances, impact graph, and coverage status. Does not predict definite breakage.", packageSchema, s.packageImpact)
	return server, nil
}

type noArgs struct{}
type searchArgs struct {
	Query *string          `json:"query"`
	Kinds []graph.NodeKind `json:"kinds"`
	Limit *int             `json:"limit"`
}
type nodeArgs struct {
	NodeID string `json:"node_id"`
}
type packageArgs struct {
	PackageID string `json:"package_id"`
}
type fileArgs struct {
	FileID string `json:"file_id"`
}
type pathArgs struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Scope  string `json:"scope"`
}

func (s *snapshotServer) requireNode(id, field string, kind graph.NodeKind) (graph.Node, error) {
	if strings.TrimSpace(id) == "" {
		return graph.Node{}, &toolError{"invalid_argument", field + " is required"}
	}
	node, ok := s.nodes[id]
	if !ok {
		return graph.Node{}, &toolError{"node_not_found", fmt.Sprintf("node %q not found", id)}
	}
	if kind != "" && node.Kind != kind {
		return graph.Node{}, &toolError{"wrong_node_kind", fmt.Sprintf("node %q must be %s; got %s", id, kind, node.Kind)}
	}
	return node, nil
}

func (s *snapshotServer) search(args searchArgs) (any, error) {
	if args.Query == nil {
		return nil, &toolError{"invalid_argument", "query is required"}
	}
	nodes, err := query.Search(s.snapshot.Result.Graph, *args.Query, args.Kinds, args.Limit)
	if err != nil {
		return nil, &toolError{"invalid_argument", err.Error()}
	}
	return struct {
		Nodes []nodeJSON `json:"nodes"`
	}{convertNodes(nodes)}, nil
}

func (s *snapshotServer) nodesFor(ids []string) []nodeJSON {
	nodes := make([]nodeJSON, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, convertNode(s.nodes[id]))
	}
	return nodes
}

func (s *snapshotServer) inspect(args nodeArgs) (any, error) {
	node, err := s.requireNode(args.NodeID, "node_id", "")
	if err != nil {
		return nil, err
	}
	parentID, known, err := s.tree.Parent(node.ID)
	if err != nil {
		return nil, err
	}
	var parent *nodeJSON
	if known {
		value := convertNode(s.nodes[parentID])
		parent = &value
	}
	children, err := s.tree.Children(node.ID)
	if err != nil {
		return nil, err
	}
	var incoming, outgoing []graph.Edge
	for _, edge := range s.snapshot.Result.Graph.Edges {
		if edge.To == node.ID {
			incoming = append(incoming, edge)
		}
		if edge.From == node.ID {
			outgoing = append(outgoing, edge)
		}
	}
	return struct {
		Node     nodeJSON   `json:"node"`
		Parent   *nodeJSON  `json:"parent"`
		Children []nodeJSON `json:"children"`
		Incoming []edgeJSON `json:"incoming_edges"`
		Outgoing []edgeJSON `json:"outgoing_edges"`
	}{convertNode(node), parent, s.nodesFor(children), convertEdges(incoming), convertEdges(outgoing)}, nil
}

type dependencyJSON struct {
	Package  nodeJSON   `json:"package"`
	Evidence []edgeJSON `json:"evidence"`
}

func (s *snapshotServer) related(args packageArgs, dependencies bool) (any, error) {
	node, err := s.requireNode(args.PackageID, "package_id", graph.Package)
	if err != nil {
		return nil, err
	}
	var ids []string
	if dependencies {
		ids, err = query.Dependencies(s.projection.Graph, node.ID)
	} else {
		ids, err = query.Dependents(s.projection.Graph, node.ID)
	}
	if err != nil {
		return nil, err
	}
	items := make([]dependencyJSON, 0, len(ids))
	for _, id := range ids {
		var sources []graph.Edge
		for _, evidence := range s.projection.Evidence {
			if dependencies && evidence.Edge.From == node.ID && evidence.Edge.To == id ||
				!dependencies && evidence.Edge.From == id && evidence.Edge.To == node.ID {
				sources = evidence.Sources
				break
			}
		}
		items = append(items, dependencyJSON{convertNode(s.nodes[id]), convertEdges(sources)})
	}
	key := "dependencies"
	if !dependencies {
		key = "dependents"
	}
	return map[string]any{"target": convertNode(node), key: items}, nil
}

func (s *snapshotServer) path(args pathArgs) (any, error) {
	if args.Scope != "canonical" && args.Scope != "package" {
		return nil, &toolError{"invalid_argument", "scope must be canonical or package"}
	}
	kind := graph.NodeKind("")
	g := s.snapshot.Result.Graph
	if args.Scope == "package" {
		kind, g = graph.Package, s.projection.Graph
	}
	if _, err := s.requireNode(args.FromID, "from_id", kind); err != nil {
		return nil, err
	}
	if _, err := s.requireNode(args.ToID, "to_id", kind); err != nil {
		return nil, err
	}
	ids, err := query.Path(g, args.FromID, args.ToID)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return struct {
		Found bool     `json:"found"`
		Path  []string `json:"path"`
	}{len(ids) > 0, ids}, nil
}

func (s *snapshotServer) symbols(args fileArgs) (any, error) {
	node, err := s.requireNode(args.FileID, "file_id", graph.File)
	if err != nil {
		return nil, err
	}
	children, err := s.tree.Children(node.ID)
	if err != nil {
		return nil, err
	}
	symbols := make([]nodeJSON, 0, len(children))
	for _, id := range children {
		if graph.IsDeclaration(s.nodes[id].Kind) {
			symbols = append(symbols, convertNode(s.nodes[id]))
		}
	}
	return struct {
		File    nodeJSON   `json:"file"`
		Symbols []nodeJSON `json:"symbols"`
	}{convertNode(node), symbols}, nil
}

func (s *snapshotServer) packageImpact(args packageArgs) (any, error) {
	node, err := s.requireNode(args.PackageID, "package_id", graph.Package)
	if err != nil {
		return nil, err
	}
	result, err := impact.Analyze(s.projection.Graph, node.ID)
	if err != nil {
		return nil, err
	}
	type affectedJSON struct {
		Package  nodeJSON `json:"package"`
		Distance int      `json:"distance"`
	}
	affected := make([]affectedJSON, 0, len(result.Affected))
	for _, item := range result.Affected {
		affected = append(affected, affectedJSON{convertNode(s.nodes[item.ID]), item.Distance})
	}
	return struct {
		Target         nodeJSON       `json:"target"`
		Affected       []affectedJSON `json:"affected"`
		Graph          graphJSON      `json:"graph"`
		CoverageStatus string         `json:"coverage_status"`
		Incomplete     bool           `json:"incomplete"`
	}{convertNode(node), affected, convertGraph(result.Graph), s.snapshot.Status, s.snapshot.Status != "complete"}, nil
}

type toolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *toolError) Error() string { return e.Code + ": " + e.Message }

func toolResult(output any, err error) *mcp.CallToolResult {
	result := &mcp.CallToolResult{}
	if err != nil {
		var known *toolError
		if !errors.As(err, &known) {
			known = &toolError{"internal_error", "stored graph operation failed"}
		}
		output = struct {
			Error *toolError `json:"error"`
		}{known}
		result.SetError(known)
	}
	data, marshalErr := json.Marshal(output)
	if marshalErr != nil {
		return toolResult(nil, marshalErr)
	}
	result.StructuredContent = output
	result.Content = []mcp.Content{&mcp.TextContent{Text: string(data)}}
	return result
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func inputSchema(properties map[string]any, required ...string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Low-level SDK handlers let argument failures use the same stable tool-error
// envelope as domain failures. The SDK still owns all MCP framing and sessions.
func addTool[In any](server *mcp.Server, name, description string, schema map[string]any, handle func(In) (any, error)) {
	closedWorld := false
	server.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		raw := bytes.TrimSpace(request.Params.Arguments)
		if len(raw) == 0 {
			raw = []byte("{}")
		}
		if raw[0] != '{' {
			return toolResult(nil, &toolError{"invalid_argument", "arguments must be an object"}), nil
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return toolResult(nil, &toolError{"invalid_argument", "invalid tool arguments"}), nil
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if _, ok := schema["properties"].(map[string]any)[key]; !ok {
				return toolResult(nil, &toolError{"invalid_argument", "unknown argument " + key}), nil
			}
			if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
				return toolResult(nil, &toolError{"invalid_argument", key + " must not be null"}), nil
			}
		}
		var args In
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&args); err != nil {
			return toolResult(nil, &toolError{"invalid_argument", "arguments do not match the tool input schema"}), nil
		}
		output, err := handle(args)
		return toolResult(output, err), nil
	})
}
