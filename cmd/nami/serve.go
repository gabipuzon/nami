package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gabipuzon/nami/internal/api"
	"github.com/gabipuzon/nami/internal/storage"
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
	listener, err := net.Listen("tcp", serveAddress)
	if err != nil {
		fmt.Fprintf(stderr, "listen on %s: %v\n", serveAddress, err)
		return 1
	}
	server := &http.Server{Handler: api.NewHandler(snapshot)}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownDone <- server.Shutdown(shutdownCtx)
	}()
	fmt.Fprintf(stdout, "SERVING http://%s scan=%s\n", serveAddress, snapshot.ID)
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
