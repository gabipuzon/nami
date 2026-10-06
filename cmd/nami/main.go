package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/hierarchy"
	"github.com/gabipuzon/nami/internal/impact"
	"github.com/gabipuzon/nami/internal/query"
	"github.com/gabipuzon/nami/internal/storage"
)

const usage = "Nami — local-first codebase navigator\n\nUsage:\n  nami map <directory>\n  nami scans <directory>\n  nami show <directory> <scan-id>\n  nami packages <directory> <scan-id>\n  nami symbols <directory> <scan-id> <file-node-id>\n  nami impact <directory> <scan-id> <package-node-id>\n  nami dependencies <directory> <scan-id> <node-id>\n  nami dependents <directory> <scan-id> <node-id>\n  nami path <directory> <scan-id> <from-node-id> <to-node-id>\n  nami serve <directory> <scan-id>\n  nami [--help]\n"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	switch args[0] {
	case "serve":
		return serve(args, stdout, stderr)
	case "map":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: nami map <directory>")
			return 2
		}
		result, err := analysis.Map(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Save(args[1], result)
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "SAVED_SCAN id=%s created_at=%s status=%s\n", scan.ID, scan.CreatedAt.Format(time.RFC3339Nano), scan.Status)
		printResult(stdout, result)
		return 0
	case "scans":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: nami scans <directory>")
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scans, err := store.List()
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, scan := range scans {
			fmt.Fprintf(stdout, "SAVED_SCAN id=%s created_at=%s status=%s\n", scan.ID, scan.CreatedAt.Format(time.RFC3339Nano), scan.Status)
		}
		return 0
	case "show":
		if len(args) != 3 {
			fmt.Fprintln(stderr, "usage: nami show <directory> <scan-id>")
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Load(args[2])
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "STORED_SCAN id=%s root=%s created_at=%s status=%s\n", scan.ID, scan.Root, scan.CreatedAt.Format(time.RFC3339Nano), scan.Status)
		printResult(stdout, scan.Result)
		return 0
	case "packages":
		if len(args) != 3 {
			fmt.Fprintln(stderr, "usage: nami packages <directory> <scan-id>")
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Load(args[2])
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		projection, err := hierarchy.ProjectPackages(scan.Result.Graph)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, node := range projection.Graph.Nodes {
			fmt.Fprintf(stdout, "PACKAGE %s\n", node.ID)
		}
		for _, edge := range projection.Graph.Edges {
			fmt.Fprintf(stdout, "IMPORTS %s -> %s\n", edge.From, edge.To)
		}
		return 0
	case "symbols":
		if len(args) != 4 {
			fmt.Fprintln(stderr, "usage: nami symbols <directory> <scan-id> <file-node-id>")
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Load(args[2])
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		nodes := make(map[string]graph.Node)
		for _, node := range scan.Result.Graph.Nodes {
			nodes[node.ID] = node
		}
		file, ok := nodes[args[3]]
		if !ok {
			fmt.Fprintf(stderr, "node %q not found\n", args[3])
			return 1
		}
		if file.Kind != graph.File {
			fmt.Fprintf(stderr, "node %q is not a FILE\n", args[3])
			return 1
		}
		h, err := hierarchy.New(scan.Result.Graph)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		children, err := h.Children(file.ID)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		for _, id := range children {
			node := nodes[id]
			fmt.Fprintf(stdout, "SYMBOL %s %s %s\n", node.Kind, node.ID, node.Name)
		}
		return 0
	case "impact":
		if len(args) != 4 {
			fmt.Fprintln(stderr, "usage: nami impact <directory> <scan-id> <package-node-id>")
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Load(args[2])
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		var target graph.Node
		found := false
		for _, node := range scan.Result.Graph.Nodes {
			if node.ID == args[3] {
				target, found = node, true
				break
			}
		}
		if !found {
			fmt.Fprintf(stderr, "node %q not found\n", args[3])
			return 1
		}
		if target.Kind != graph.Package {
			fmt.Fprintf(stderr, "node %q is not a PACKAGE\n", args[3])
			return 1
		}
		projection, err := hierarchy.ProjectPackages(scan.Result.Graph)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		result, err := impact.Analyze(projection.Graph, target.ID)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "IMPACT_TARGET %s\n", result.Target)
		if len(result.Affected) == 0 {
			fmt.Fprintln(stdout, "AFFECTED none")
		}
		for _, affected := range result.Affected {
			fmt.Fprintf(stdout, "AFFECTED distance=%d %s\n", affected.Distance, affected.ID)
		}
		fmt.Fprintf(stdout, "IMPACT_COVERAGE status=%s\n", scan.Result.Coverage.Status)
		if scan.Result.Coverage.Status != "complete" {
			fmt.Fprintln(stdout, "IMPACT_WARNING stored scan has analysis gaps; impact may be incomplete")
		}
		return 0
	case "dependencies", "dependents", "path":
		wantArgs := 4
		if args[0] == "path" {
			wantArgs = 5
		}
		if len(args) != wantArgs {
			if args[0] == "path" {
				fmt.Fprintln(stderr, "usage: nami path <directory> <scan-id> <from-node-id> <to-node-id>")
			} else {
				fmt.Fprintf(stderr, "usage: nami %s <directory> <scan-id> <node-id>\n", args[0])
			}
			return 2
		}
		store, err := storage.Open(args[1])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		scan, err := store.Load(args[2])
		store.Close()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		var ids []string
		switch args[0] {
		case "dependencies":
			ids, err = query.Dependencies(scan.Result.Graph, args[3])
		case "dependents":
			ids, err = query.Dependents(scan.Result.Graph, args[3])
		case "path":
			ids, err = query.Path(scan.Result.Graph, args[3], args[4])
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if len(ids) == 0 {
			switch args[0] {
			case "dependencies":
				fmt.Fprintln(stdout, "dependencies: none")
			case "dependents":
				fmt.Fprintln(stdout, "dependents: none")
			case "path":
				fmt.Fprintln(stdout, "NO_PATH")
			}
			return 0
		}
		prefix := map[string]string{"dependencies": "DEPENDENCY", "dependents": "DEPENDENT", "path": "PATH"}[args[0]]
		for _, id := range ids {
			fmt.Fprintf(stdout, "%s %s\n", prefix, id)
		}
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
		return 2
	}
}

func printResult(stdout io.Writer, result analysis.Result) {
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
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
