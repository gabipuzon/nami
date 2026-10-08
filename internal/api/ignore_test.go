package api

import (
	"net/http"
	"testing"

	"github.com/gabipuzon/nami/internal/analysis"
)

func TestScanExclusionsAreSeparateFromIssues(t *testing.T) {
	snapshot := fixtureSnapshot(t)
	snapshot.Status = "complete"
	snapshot.Result.Coverage.Status = "complete"
	snapshot.Result.Issues = nil
	snapshot.Result.Exclusions = []analysis.Exclusion{{Path: "generated/", Reason: `.namiignore:1 pattern "generated/"`}}
	code, body, _ := request(t, NewHandler(snapshot), http.MethodGet, "/api/v1/scan")
	if code != 200 || body["status"] != "complete" || len(body["issues"].([]any)) != 0 || len(body["exclusions"].([]any)) != 1 {
		t.Fatal(code, body)
	}
	exclusion := body["exclusions"].([]any)[0].(map[string]any)
	if exclusion["path"] != "generated/" || exclusion["reason"] != snapshot.Result.Exclusions[0].Reason {
		t.Fatal(exclusion)
	}
	for _, route := range []string{"/api/v1/impact?package_id=package:A", "/api/v1/neighborhood?node_id=package:A&scope=package"} {
		code, scoped, _ := request(t, NewHandler(snapshot), http.MethodGet, route)
		if code != 200 || scoped["excluded_paths"] != float64(1) || scoped["incomplete"] != false {
			t.Fatal(code, scoped)
		}
	}
	snapshot.Result.Exclusions = nil
	_, old, _ := request(t, NewHandler(snapshot), http.MethodGet, "/api/v1/scan")
	if len(old["exclusions"].([]any)) != 0 {
		t.Fatal(old)
	}
}
