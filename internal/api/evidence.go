package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/query"
)

func nonnegative(params url.Values, key string) (int, error) {
	values := params[key]
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 {
		return 0, fmt.Errorf("exactly one %s is allowed", key)
	}
	n, err := strconv.Atoi(values[0])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be nonnegative", key)
	}
	return n, nil
}
func (h *handler) evidence(w http.ResponseWriter, params url.Values) {
	from, ok := required(w, params, "from")
	if !ok {
		return
	}
	to, ok := required(w, params, "to")
	if !ok {
		return
	}
	kind, ok := required(w, params, "kind")
	if !ok {
		return
	}
	scope, ok := required(w, params, "scope")
	if !ok {
		return
	}
	offset, err := nonnegative(params, "offset")
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	page, err := query.SelectEvidence(h.snapshot.Result.Graph, h.snapshot.Result.SourceEvidence, graph.Edge{Kind: graph.EdgeKind(kind), From: from, To: to}, scope, offset)
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	writeJSON(w, 200, page)
}
func (h *handler) sourceStatus(w http.ResponseWriter, params url.Values) {
	id, ok := required(w, params, "file_id")
	if !ok {
		return
	}
	if !h.requireNode(w, id, graph.File) {
		return
	}
	node := h.nodes[id]
	status, location := "not_recorded", ""
	hash := ""
	for _, e := range h.snapshot.Result.SourceEvidence {
		if e.Path == node.Path {
			hash = e.Hash
			break
		}
	}
	if hash != "" {
		status, location = checkCurrentSource(h.snapshot.Root, node.Path, hash)
	}

	writeJSON(w, 200, struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}{status, location})
}

func (h *handler) relationshipFacts(w http.ResponseWriter, params url.Values) {
	from, ok := required(w, params, "from")
	if !ok {
		return
	}
	to, ok := required(w, params, "to")
	if !ok {
		return
	}
	kind, ok := required(w, params, "kind")
	if !ok {
		return
	}
	scope, ok := required(w, params, "scope")
	if !ok {
		return
	}
	offset, err := nonnegative(params, "offset")
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	facts, total, err := query.SupportingFacts(h.snapshot.Result.Graph, graph.Edge{Kind: graph.EdgeKind(kind), From: from, To: to}, scope, offset)
	if err != nil {
		writeError(w, 400, "bad_request", err.Error())
		return
	}
	if h.viewErr != nil {
		writeInternalError(w)
		return
	}
	items := []edgeJSON{}
	for _, edge := range facts {
		items = append(items, convertEdge(edge))
	}
	writeJSON(w, 200, struct {
		Items  []edgeJSON `json:"items"`
		Total  int        `json:"total"`
		Offset int        `json:"offset"`
		Graph  graphJSON  `json:"graph"`
	}{items, total, offset, convertGraph(h.view.Subgraph(map[string]bool{}, facts))})
}

// Hash current regular files without loading another copy of their contents.
func checkCurrentSource(root, relative, expected string) (string, string) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", ""
		}
		return "unreadable", ""
	}
	location := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(location)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", ""
		}
		return "unreadable", ""
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "unreadable", ""
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "unreadable", ""
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "unreadable", ""
	}
	digest := sha256.New()
	_, readErr := io.Copy(digest, file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return "unreadable", ""
	}
	if hex.EncodeToString(digest.Sum(nil)) == expected {
		return "unchanged", location
	}
	return "changed", location
}
