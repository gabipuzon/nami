package main

import (
	"fmt"
	"io"
	"os"
)

const usage = "Nami — local-first codebase navigator\n\nUsage:\n  nami [--help]\n"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprint(stdout, usage)
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n\n%s", args[0], usage)
	return 2
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
