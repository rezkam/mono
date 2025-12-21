package main

import (
	"context"
	"io"
	"log/slog"
)

// shutdowner abstracts the authenticator so tests can verify cleanup behavior
// without constructing real infrastructure dependencies.
type shutdowner interface {
	Shutdown(context.Context) error
}

// newCleanup constructs the shutdown hook that mirrors the previous server
// behavior: drain authenticator state, then close the shared store.
func newCleanup(ctx context.Context, authenticator shutdowner, store io.Closer) func() {
	return func() {
		if authenticator != nil {
			if err := authenticator.Shutdown(ctx); err != nil {
				slog.Error("failed to shut down authenticator", slog.String("error", err.Error()))
			}
		}

		if store != nil {
			if err := store.Close(); err != nil {
				slog.Error("failed to close store", slog.String("error", err.Error()))
			}
		}
	}
}
