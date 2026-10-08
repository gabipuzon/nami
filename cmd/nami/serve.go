package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gabipuzon/nami/internal/session"
	"github.com/gabipuzon/nami/internal/storage"
	"github.com/gabipuzon/nami/internal/webui"
)

const serveAddress = "127.0.0.1:7331"

func serve(args []string, stdout, stderr io.Writer) int {
	if len(args) != 3 {
		fmt.Fprintln(stderr, "usage: nami serve <directory> <scan-id>")
		return 2
	}
	store, err := storage.Open(args[1])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	snapshot, loadErr := store.Load(args[2])
	closeErr := store.Close()
	if loadErr != nil {
		fmt.Fprintln(stderr, loadErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintln(stderr, closeErr)
		return 1
	}
	return serveSnapshot(snapshot, serveAddress, false, stdout, stderr)
}

func serveSnapshot(snapshot storage.Snapshot, address string, open bool, stdout, stderr io.Writer) int {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintf(stderr, "listen on %s: %v\n", address, err)
		return 1
	}
	var handler http.Handler = session.New(snapshot, session.RepositoryScan(snapshot.Root))
	if files := webui.Files(); files != nil {
		handler = webui.Handler(files, handler)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- server.Shutdown(shutdownCtx)
	}()
	fmt.Fprintf(stdout, "SERVING http://%s scan=%s\n", listener.Addr().String(), snapshot.ID)
	if open {
		if err := openBrowser("http://" + listener.Addr().String()); err != nil {
			fmt.Fprintf(stderr, "open browser: %v; use the printed URL\n", err)
		}
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		if shutdownErr := <-shutdownDone; shutdownErr != nil {
			fmt.Fprintf(stderr, "shut down server: %v\n", shutdownErr)
			return 1
		}
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func openBrowser(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{url}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		command, args = "xdg-open", []string{url}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, command, args...).Run()
}
