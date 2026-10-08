package main

import (
	"fmt"
	"io"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/storage"
	"github.com/gabipuzon/nami/internal/webui"
)

func mapRepository(args []string, stdout, stderr io.Writer) int {
	var root string
	var noServe, noOpen bool
	for _, arg := range args[1:] {
		switch arg {
		case "--no-serve":
			noServe = true
		case "--no-open":
			noOpen = true
		default:
			if root != "" || len(arg) == 0 || arg[0] == '-' {
				fmt.Fprintln(stderr, "usage: nami map [--no-serve] [--no-open] <directory>")
				return 2
			}
			root = arg
		}
	}
	if root == "" {
		fmt.Fprintln(stderr, "usage: nami map [--no-serve] [--no-open] <directory>")
		return 2
	}
	if !noServe && webui.Files() == nil {
		fmt.Fprintln(stderr, "bundled frontend is missing; run make build, or use --no-serve for CLI analysis")
		return 1
	}
	result, err := analysis.Map(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	store, err := storage.Open(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	scan, saveErr := store.Save(root, result)
	closeErr := store.Close()
	if saveErr != nil {
		fmt.Fprintln(stderr, saveErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, closeErr)
		return 1
	}
	printResult(stdout, result)
	fmt.Fprintf(stdout, "SAVED_SCAN id=%s created_at=%s status=%s\n", scan.ID, scan.CreatedAt.Format(time.RFC3339Nano), scan.Status)
	if noServe {
		return 0
	}
	return serveSnapshot(storage.Snapshot{ID: scan.ID, Root: scan.Root, CreatedAt: scan.CreatedAt, Status: scan.Status, Result: result}, "127.0.0.1:0", !noOpen, stdout, stderr)
}
