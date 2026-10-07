package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gabipuzon/nami/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func runMCP(args []string, stdin io.ReadCloser, stdout, stderr io.Writer) int {
	if len(args) != 3 {
		fmt.Fprintln(stderr, "usage: nami mcp <directory> <scan-id>")
		return 2
	}
	server, err := mcpserver.Load(args[1], args[2])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// stdout carries only SDK protocol messages. Closing a session must not
	// close the process's output stream or a caller-owned diagnostic buffer.
	err = server.Run(ctx, &mcp.IOTransport{Reader: stdin, Writer: mcpOutput{stdout}})
	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

type mcpOutput struct{ io.Writer }

func (mcpOutput) Close() error { return nil }
