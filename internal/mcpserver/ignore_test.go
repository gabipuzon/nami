package mcpserver

import (
	"github.com/gabipuzon/nami/internal/analysis"
	"testing"
)

func TestMCPPreservesSnapshotExclusionsAndSeparatesIssues(t *testing.T) {
	snapshot := fixture(t)
	snapshot.Status = "complete"
	snapshot.Result.Coverage.Status = "complete"
	snapshot.Result.Issues = nil
	snapshot.Result.Exclusions = []analysis.Exclusion{{Path: "generated/", Reason: `.namiignore:2 pattern "generated/"`}}
	server, err := New(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := connect(t, server)
	snapshot.Result.Exclusions[0].Path = "changed"
	info := decode[scanInfoJSON](t, call(t, ctx, session, "nami_scan_info", nil))
	if len(info.Exclusions) != 1 || info.Exclusions[0].Path != "generated/" || len(info.Issues) != 0 || info.Status != "complete" {
		t.Fatal(info)
	}
	impact := decode[struct {
		ExcludedPaths int  `json:"excluded_paths"`
		Incomplete    bool `json:"incomplete"`
	}](t, call(t, ctx, session, "nami_package_impact", map[string]any{"package_id": packageA}))
	if impact.ExcludedPaths != 1 || impact.Incomplete {
		t.Fatal(impact)
	}

}
