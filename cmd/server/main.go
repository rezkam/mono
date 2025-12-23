package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to run: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Create main application context that cancels on SIGTERM/SIGINT
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize HTTP server using Wire
	server, cleanup, err := InitializeHTTPServer(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize server: %w", err)
	}
	defer cleanup()

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		fmt.Println("\nReceived shutdown signal")

		// Graceful shutdown with configured timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), server.ShutdownTimeout())
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown error: %w", err)
		}

		fmt.Println("Server stopped gracefully")
		return nil

	case err := <-serverErr:
		return err
	}
}
