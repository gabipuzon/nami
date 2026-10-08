package analysis

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gabipuzon/nami/internal/analyzer/goanalyzer"
	"github.com/gabipuzon/nami/internal/analyzer/pythonanalyzer"
	"github.com/gabipuzon/nami/internal/detect"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/scanner"
)

type Result struct {
	SourceEvidence []graph.SourceEvidence
	Graph          graph.Graph
	Coverage       Coverage
	Issues         []Issue
	Exclusions     []Exclusion
}

type Exclusion struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
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
	pythonFiles := detect.PythonFiles(scan.Files)
	var skippedPaths []string
	for _, skipped := range scan.Skipped {
		skippedPaths = append(skippedPaths, skipped.Path)
	}
	unsupported := detect.UnsupportedSources(scan.Files)
	var excludedPaths []string
	for _, excluded := range scan.Exclusions {
		excludedPaths = append(excludedPaths, excluded.Path)
	}
	goResult := goanalyzer.AnalyzeWithExclusions(absRoot, goFiles, scan.Files, excludedPaths)
	pythonResult := pythonanalyzer.Analyze(absRoot, pythonFiles)
	graph, err := graph.Build(goResult.Fragment, pythonResult.Fragment)
	if err != nil {
		return Result{}, err
	}
	var issues []Issue
	for _, issue := range goResult.Issues {
		issues = append(issues, Issue{Kind: issue.Kind, Path: issue.Path, Import: issue.Import, Reason: issue.Reason})
	}
	for _, issue := range pythonResult.Issues {
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
		SupportedSourceFiles: len(goFiles) + len(pythonFiles) + len(detect.GoFiles(skippedPaths)) + len(detect.PythonFiles(skippedPaths)),
		FilesAnalyzed:        goResult.FilesAnalyzed + pythonResult.FilesAnalyzed,
		FilesSkipped:         len(scan.Skipped) + len(unsupported),
		FilesFailed:          goResult.FilesFailed + pythonResult.FilesFailed,
		ImportsDiscovered:    goResult.Imports.Discovered + pythonResult.Imports.Discovered,
		InternalResolved:     goResult.Imports.InternalResolved + pythonResult.Imports.InternalResolved,
		StandardLibrary:      goResult.Imports.StandardLibrary + pythonResult.Imports.StandardLibrary,
		External:             goResult.Imports.External + pythonResult.Imports.External,
		Unresolved:           goResult.Imports.Unresolved + pythonResult.Imports.Unresolved,
		Cgo:                  goResult.Imports.Cgo,
		Unclassified:         goResult.Imports.Unclassified + pythonResult.Imports.Unclassified,
	}
	if len(issues) > 0 {
		coverage.Status = "completed_with_gaps"
	}
	var exclusions []Exclusion
	for _, excluded := range scan.Exclusions {
		exclusions = append(exclusions, Exclusion{Path: excluded.Path, Reason: excluded.Reason})
	}
	evidence := append(goResult.SourceEvidence, pythonResult.SourceEvidence...)
	sort.SliceStable(evidence, func(i, j int) bool {
		a, b := evidence[i], evidence[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Edge.Kind != b.Edge.Kind {
			return a.Edge.Kind < b.Edge.Kind
		}
		if a.Edge.To != b.Edge.To {
			return a.Edge.To < b.Edge.To
		}
		return a.Snippet < b.Snippet
	})
	return Result{SourceEvidence: evidence, Graph: graph, Coverage: coverage, Issues: issues, Exclusions: exclusions}, nil
}
