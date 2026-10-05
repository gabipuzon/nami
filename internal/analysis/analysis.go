package analysis

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gabipuzon/nami/internal/analyzer/goanalyzer"
	"github.com/gabipuzon/nami/internal/detect"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/scanner"
)

type Result struct {
	Graph    graph.Graph
	Coverage Coverage
	Issues   []Issue
}

type Coverage struct {
	Status               string
	FilesDiscovered      int
	SupportedSourceFiles int
	FilesAnalyzed        int
	FilesSkipped         int
	FilesFailed          int
	ImportsDiscovered    int
	InternalResolved     int
	StandardLibrary      int
	External             int
	Unresolved           int
	Cgo                  int
	Unclassified         int
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
	goFiles := detect.GoFiles(scan.Files)
	var skippedPaths []string
	for _, skipped := range scan.Skipped {
		skippedPaths = append(skippedPaths, skipped.Path)
	}
	unsupported := detect.UnsupportedSources(scan.Files)
	goResult := goanalyzer.Analyze(absRoot, goFiles, scan.Files)
	graph, err := graph.Build(goResult.Fragment)
	if err != nil {
		return Result{}, err
	}
	var issues []Issue
	for _, issue := range goResult.Issues {
		issues = append(issues, Issue{Kind: issue.Kind, Path: issue.Path, Import: issue.Import, Reason: issue.Reason})
	}
	for _, skipped := range scan.Skipped {
		issues = append(issues, Issue{Kind: "SKIPPED_FILE", Path: skipped.Path, Reason: skipped.Reason})
	}
	for _, file := range unsupported {
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
	coverage := Coverage{
		Status:               "complete",
		FilesDiscovered:      scan.Discovered,
		SupportedSourceFiles: len(goFiles) + len(detect.GoFiles(skippedPaths)),
		FilesAnalyzed:        goResult.FilesAnalyzed,
		FilesSkipped:         len(scan.Skipped) + len(unsupported),
		FilesFailed:          goResult.FilesFailed,
		ImportsDiscovered:    goResult.Imports.Discovered,
		InternalResolved:     goResult.Imports.InternalResolved,
		StandardLibrary:      goResult.Imports.StandardLibrary,
		External:             goResult.Imports.External,
		Unresolved:           goResult.Imports.Unresolved,
		Cgo:                  goResult.Imports.Cgo,
		Unclassified:         goResult.Imports.Unclassified,
	}
	if len(issues) > 0 {
		coverage.Status = "completed_with_gaps"
	}
	return Result{Graph: graph, Coverage: coverage, Issues: issues}, nil
}
