package analysis

import (
	"fmt"
	"path/filepath"
	"sort"

	"nami/internal/analyzer/goanalyzer"
	"nami/internal/detect"
	"nami/internal/graph"
	"nami/internal/scanner"
)

type Result struct {
	Graph  graph.Graph
	Issues []Issue
}

type Issue struct {
	Kind   string
	Path   string
	Import string
	Reason string
}

func Map(root string) (Result, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Result{}, fmt.Errorf("resolve repository path: %w", err)
	}
	scan, err := scanner.Scan(absRoot)
	if err != nil {
		return Result{}, err
	}
	fragment, goIssues := goanalyzer.Analyze(absRoot, detect.GoFiles(scan.Files), scan.Files)
	graph, err := graph.Build(fragment)
	if err != nil {
		return Result{}, err
	}
	var issues []Issue
	for _, issue := range goIssues {
		issues = append(issues, Issue{Kind: issue.Kind, Path: issue.Path, Import: issue.Import, Reason: issue.Reason})
	}
	for _, skipped := range scan.Skipped {
		issues = append(issues, Issue{Kind: "SKIPPED_FILE", Path: skipped.Path, Reason: skipped.Reason})
	}
	for _, file := range detect.UnsupportedSources(scan.Files) {
		issues = append(issues, Issue{Kind: "UNSUPPORTED_FILE", Path: file, Reason: "no analyzer for this source language"})
	}
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Import != b.Import {
			return a.Import < b.Import
		}
		return a.Reason < b.Reason
	})
	return Result{Graph: graph, Issues: issues}, nil
}
