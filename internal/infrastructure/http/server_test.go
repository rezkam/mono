package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxHeaderBytes_EnforcesLimit(t *testing.T) {
	// Go's http.Server adds an internal buffer (~8KB) to MaxHeaderBytes.
	// With MaxHeaderBytes=4KB, the effective limit is ~12KB.
	// We test with headers that are clearly within and clearly outside this limit.
	const maxHeaderBytes = 4 * 1024 // 4KB configured limit

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewUnstartedServer(handler)
	server.Config.MaxHeaderBytes = maxHeaderBytes
	server.Start()
	defer server.Close()

	t.Run("accepts request within limit", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
		require.NoError(t, err)

		// 2KB header - well within the ~12KB effective limit
		req.Header.Set("X-Test", strings.Repeat("A", 2*1024))

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err, "request within limit should succeed")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("rejects request exceeding limit", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
		require.NoError(t, err)

		// 20KB header - clearly exceeds the ~12KB effective limit
		req.Header.Set("X-Test", strings.Repeat("A", 20*1024))

		resp, err := http.DefaultClient.Do(req)

		// Go's http.Server returns 431 Request Header Fields Too Large
		// when headers exceed MaxHeaderBytes + internal buffer
		if err != nil {
			// Connection error also indicates rejection
			t.Logf("server rejected with connection error: %v", err)
			return
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusRequestHeaderFieldsTooLarge, resp.StatusCode,
			"expected 431 when headers exceed limit")
	})
}

func TestServerConfig_ApplyDefaults(t *testing.T) {
	t.Run("applies all defaults for zero config", func(t *testing.T) {
		cfg := ServerConfig{}
		cfg.applyDefaults()

		assert.Equal(t, DefaultPort, cfg.Port)
		assert.Equal(t, DefaultReadTimeout, cfg.ReadTimeout)
		assert.Equal(t, DefaultWriteTimeout, cfg.WriteTimeout)
		assert.Equal(t, DefaultIdleTimeout, cfg.IdleTimeout)
		assert.Equal(t, DefaultReadHeaderTimeout, cfg.ReadHeaderTimeout)
		assert.Equal(t, DefaultMaxHeaderBytes, cfg.MaxHeaderBytes)
		assert.Equal(t, int64(DefaultMaxBodyBytes), cfg.MaxBodyBytes)
	})

	t.Run("preserves non-zero values", func(t *testing.T) {
		cfg := ServerConfig{
			Port:           "9000",
			MaxHeaderBytes: 2048,
			MaxBodyBytes:   4096,
		}
		cfg.applyDefaults()

		assert.Equal(t, "9000", cfg.Port)
		assert.Equal(t, 2048, cfg.MaxHeaderBytes)
		assert.Equal(t, int64(4096), cfg.MaxBodyBytes)
		// Other fields should get defaults
		assert.Equal(t, DefaultReadTimeout, cfg.ReadTimeout)
	})
}
