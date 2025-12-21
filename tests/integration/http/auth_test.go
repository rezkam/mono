package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rezkam/mono/internal/application/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth_ValidAPIKey_AllowsAccess(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer "+ts.APIKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_ExpiredAPIKeyIsRejected(t *testing.T) {
	ts := SetupTestServer(t)
	defer ts.Cleanup()

	ctx := context.Background()
	expiredAt := time.Now().Add(-1 * time.Hour)
	expiredKey, err := auth.CreateAPIKey(ctx, ts.Store, "sk", "test", "v1", "expired", &expiredAt)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer "+expiredKey)

	w := httptest.NewRecorder()
	ts.Router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
