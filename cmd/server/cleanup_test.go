package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewCleanup_ShutsDownAuthenticatorBeforeClosingStore(t *testing.T) {
	t.Helper()

	ctx := context.WithValue(context.Background(), ctxKey("test"), "marker")
	var callOrder []string

	authenticator := &fakeAuthenticator{calls: &callOrder}
	store := &fakeStore{calls: &callOrder}

	cleanup := newCleanup(ctx, authenticator, store)

	cleanup()

	require.Equal(t, []string{"authShutdown", "storeClose"}, callOrder)
	require.Equal(t, "marker", authenticator.receivedCtx.Value(ctxKey("test")))
}

type ctxKey string

type fakeAuthenticator struct {
	calls       *[]string
	receivedCtx context.Context
}

func (f *fakeAuthenticator) Shutdown(ctx context.Context) error {
	f.receivedCtx = ctx
	*f.calls = append(*f.calls, "authShutdown")
	return nil
}

type fakeStore struct {
	calls *[]string
}

func (s *fakeStore) Close() error {
	*s.calls = append(*s.calls, "storeClose")
	return nil
}
