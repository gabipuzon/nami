//go:build webui

package main

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBundledMapHelper(t *testing.T) {
	if root := os.Getenv("NAMI_BUNDLED_TEST_ROOT"); root != "" {
		os.Exit(run([]string{"map", "--no-open", root}, os.Stdout, os.Stderr))
	}
}
func TestBundledMapServesOutsideCheckoutAndShutsDown(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/sample\n\ngo 1.25\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package sample\nvar Value = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestBundledMapHelper$")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "NAMI_BUNDLED_TEST_ROOT="+root)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
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
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "SERVING ") {
				ready <- strings.Fields(line)[1]
				return
			}
		}
		ready <- ""
	}()
	var address string
	select {
	case address = <-ready:
	case <-ctx.Done():
		t.Fatal("map did not start", ctx.Err())
	}
	if address == "" {
		t.Fatal("map exited before serving")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, route := range []string{"/", "/api/v1/scan", "/api/v1/overview"} {
		response, err := client.Get(address + route)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			t.Fatal(route, response.StatusCode, err)
		}
		if route == "/" && !strings.Contains(string(body), "/_next/") {
			t.Fatal("real frontend assets missing")
		}
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal("shutdown failed", err)
	}
}
