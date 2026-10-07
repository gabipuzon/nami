package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gabipuzon/nami/internal/analysis"
	"github.com/gabipuzon/nami/internal/graph"
	"github.com/gabipuzon/nami/internal/storage"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPArgumentsAndStartupDiagnostics(t *testing.T) {
	for _, args := range [][]string{{"mcp"}, {"mcp", t.TempDir()}, {"mcp", t.TempDir(), "missing", "extra"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage: nami mcp") {
			t.Fatalf("MCP arguments: %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
		}
	}
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"mcp", root, "missing"}, &stdout, &stderr); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), `snapshot "missing" not found`) {
		t.Fatalf("MCP startup: %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("NAMI_MCP_HELPER") != "1" {
		return
	}
	os.Exit(run([]string{"mcp", os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]}, os.Stdout, os.Stderr))
}

func TestMCPStdioProtocolAndShutdown(t *testing.T) {
	for _, shutdown := range []string{"client close", "termination"} {
		t.Run(shutdown, func(t *testing.T) {
			root := t.TempDir()
			store, err := storage.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			result := analysis.Result{Graph: graph.Graph{Nodes: []graph.Node{{ID: "package:.#main", Kind: graph.Package, Name: "main", Path: "."}}}, Coverage: analysis.Coverage{Status: "complete"}}
			summary, saveErr := store.Save(root, result)
			closeErr := store.Close()
			if saveErr != nil || closeErr != nil {
				t.Fatalf("save: %v; close: %v", saveErr, closeErr)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMCPHelperProcess$", "--", root, summary.ID)
			cmd.Env = append(os.Environ(), "NAMI_MCP_HELPER=1")
			var stderr, protocol bytes.Buffer
			cmd.Stderr = &stderr
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "1"}, nil)
			session, err := client.Connect(ctx, &mcp.IOTransport{Reader: io.NopCloser(io.TeeReader(stdout, &protocol)), Writer: stdin}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			tools, err := session.ListTools(ctx, nil)
			if err != nil || len(tools.Tools) != 8 {
				t.Fatalf("stdio discovery = %+v, %v", tools, err)
			}
			info, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "nami_scan_info"})
			if err != nil || info.IsError {
				t.Fatalf("stdio scan info = %+v, %v", info, err)
			}
			if shutdown == "client close" {
				if err := session.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
				_ = session.Wait()
			}
			if err := cmd.Wait(); err != nil {
				t.Fatalf("MCP process did not exit cleanly: %v; stderr %q", err, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected diagnostics: %q", stderr.String())
			}
			scanner := bufio.NewScanner(bytes.NewReader(protocol.Bytes()))
			frames := 0
			for scanner.Scan() {
				var message map[string]any
				if err := json.Unmarshal(scanner.Bytes(), &message); err != nil || message["jsonrpc"] != "2.0" {
					t.Fatalf("stdout contains non-protocol traffic: %q", scanner.Text())
				}
				frames++
			}
			if err := scanner.Err(); err != nil || frames < 3 {
				t.Fatalf("protocol frames = %d, %v", frames, err)
			}
		})
	}
}
