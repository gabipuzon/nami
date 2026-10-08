package api

import (
	"net/http"
	"testing"
)

func TestNeighborhoodSnapshotAndValidation(t *testing.T) {
	h := NewHandler(fixtureSnapshot(t))
	code, body, _ := request(t, h, http.MethodGet, "/api/v1/neighborhood?node_id=file:a.go&scope=canonical")
	if code != 200 || body["snapshot_id"] != "scan-one" || body["coverage_status"] != "completed_with_gaps" || body["incomplete"] != true || body["page_size"] != float64(20) {
		t.Fatal(code, body)
	}
	if body["dependencies"].(map[string]any)["total"] != float64(1) {
		t.Fatal(body)
	}
	code, body, _ = request(t, h, http.MethodGet, "/api/v1/neighborhood?node_id=package:A&scope=package")
	if code != 200 || len(body["evidence"].([]any)) != 2 {
		t.Fatal(code, body)
	}
	for _, params := range []string{"node_id=file:a.go", "node_id=file:a.go&scope=bad", "node_id=file:a.go&scope=package", "node_id=function:a.go%23Run&scope=canonical", "node_id=file:a.go&scope=canonical&dependency_offset=-1", "node_id=file:a.go&scope=canonical&dependent_offset=x", "node_id=file:a.go&scope=canonical&dependent_offset=1&dependent_offset=2", "node_id=file:a.go&scope=canonical&dependency_offset=999999999999999999999999"} {
		code, _, _ := request(t, h, http.MethodGet, "/api/v1/neighborhood?"+params)
		if code != 400 {
			t.Fatalf("%s: %d", params, code)
		}
	}
	code, _, _ = request(t, h, http.MethodGet, "/api/v1/neighborhood?node_id=absent&scope=canonical")
	if code != 404 {
		t.Fatal(code)
	}
	code, _, _ = request(t, h, http.MethodPost, "/api/v1/neighborhood")
	if code != 405 {
		t.Fatal(code)
	}
}
