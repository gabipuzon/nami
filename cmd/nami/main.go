package main

import (
	"fmt"
	"io"
	"os"

	"nami/internal/analysis"
)

const usage = "Nami — local-first codebase navigator\n\nUsage:\n  nami map <directory>\n  nami [--help]\n"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if args[0] == "map" {
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: nami map <directory>")
			return 2
		}
		result, err := analysis.Map(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, node := range result.Graph.Nodes {
			fmt.Fprintf(stdout, "%s %s %s\n", node.Kind, node.ID, node.Name)
		}
		for _, edge := range result.Graph.Edges {
			fmt.Fprintf(stdout, "%s %s -> %s\n", edge.Kind, edge.From, edge.To)
		}
		coverage := result.Coverage
		fmt.Fprintf(stdout, "COVERAGE status=%s\n", coverage.Status)
		fmt.Fprintf(stdout, "COVERAGE files_discovered=%d supported_source_files=%d files_analyzed=%d files_skipped=%d files_failed=%d\n",
			coverage.FilesDiscovered, coverage.SupportedSourceFiles, coverage.FilesAnalyzed, coverage.FilesSkipped, coverage.FilesFailed)
		fmt.Fprintf(stdout, "COVERAGE imports_discovered=%d internal_imports_resolved=%d standard_library_imports=%d external_imports=%d unresolved_imports=%d cgo_imports=%d unclassified_imports=%d\n",
			coverage.ImportsDiscovered, coverage.InternalResolved, coverage.StandardLibrary, coverage.External, coverage.Unresolved, coverage.Cgo, coverage.Unclassified)
		for _, issue := range result.Issues {
			if issue.Import == "" {
				fmt.Fprintf(stdout, "%s %s: %s\n", issue.Kind, issue.Path, issue.Reason)
			} else {
				fmt.Fprintf(stdout, "%s %s %q: %s\n", issue.Kind, issue.Path, issue.Import, issue.Reason)
			}
		}
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
	return 2
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
