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
