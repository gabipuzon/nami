package mcpserver

import (
	"sort"
	"time"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
)

type nodeJSON struct {
	ID          string         `json:"id"`
	Kind        graph.NodeKind `json:"kind"`
	Path        string         `json:"path"`
	Name        string         `json:"name"`
	Language    string         `json:"language,omitempty"`
	ImportCount *int           `json:"import_count,omitempty"`
	ExportCount *int           `json:"export_count,omitempty"`
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

type scanInfoJSON struct {
	ID        string       `json:"id"`
	Root      string       `json:"root"`
	CreatedAt string       `json:"created_at"`
	Status    string       `json:"status"`
	Coverage  coverageJSON `json:"coverage"`
	Issues    []issueJSON  `json:"issues"`
}

func scanInfo(snapshot storage.Snapshot) scanInfoJSON {
	c := snapshot.Result.Coverage
	issues := make([]issueJSON, 0, len(snapshot.Result.Issues))
	for _, issue := range snapshot.Result.Issues {
		issues = append(issues, issueJSON{issue.Kind, issue.Path, issue.Import, issue.Reason})
	}
	return scanInfoJSON{
		ID: snapshot.ID, Root: snapshot.Root, CreatedAt: snapshot.CreatedAt.Format(time.RFC3339Nano), Status: snapshot.Status,
		Coverage: coverageJSON{c.FilesDiscovered, c.SupportedSourceFiles, c.FilesAnalyzed, c.FilesSkipped, c.FilesFailed,
			c.ImportsDiscovered, c.InternalResolved, c.StandardLibrary, c.External, c.Unresolved, c.Cgo, c.Unclassified},
		Issues: issues,
	}
}

func convertNode(node graph.Node) nodeJSON {
	result := nodeJSON{ID: node.ID, Kind: node.Kind, Path: node.Path, Name: node.Name}
	result.Language = node.Language
	if node.Kind == graph.File && node.HasImportCount {
		result.ImportCount = &node.ImportCount
	}
	if node.Kind == graph.File && node.HasExportCount {
		result.ExportCount = &node.ExportCount
	}
	return result
}

func convertNodes(nodes []graph.Node) []nodeJSON {
	result := make([]nodeJSON, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, convertNode(node))
	}
	return result
}

func convertEdges(edges []graph.Edge) []edgeJSON {
	result := make([]edgeJSON, 0, len(edges))
	for _, edge := range edges {
		result = append(result, edgeJSON{edge.Kind, edge.From, edge.To})
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
	return result
}

func convertGraph(g graph.Graph) graphJSON {
	nodes := convertNodes(g.Nodes)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return graphJSON{nodes, convertEdges(g.Edges)}
}
