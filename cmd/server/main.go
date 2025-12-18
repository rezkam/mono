package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/auth"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/service"
	sqlstorage "github.com/rezkam/mono/internal/storage/sql"
	"github.com/rezkam/mono/pkg/observability"
)

func main() {
	if err := run(); err != nil {
		// Use standard log here as slog might not be init if config fails,
		// or we can just print to stderr
		fmt.Fprintf(os.Stderr, "failed to run: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load Configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create main application context that cancels on SIGTERM/SIGINT
	// This is the root context for all normal operations
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Init Observability (Logger, Tracer, Meter)
	// Configuration via OTEL_* env vars (endpoint, headers, resource attributes)
	lp, logger, err := observability.InitLogger(ctx, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer func() {
		// Use a timeout to prevent hanging if collector is unreachable
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := lp.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to shutdown logger provider", "error", err)
		}
	}()
	// Set generic logger as default for now
	slog.SetDefault(logger)

	tp, err := observability.InitTracerProvider(ctx, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init tracer provider: %w", err)
	}
	defer func() {
		// Use a timeout to prevent hanging if collector is unreachable
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to shutdown tracer provider", "error", err)
		}
	}()

	mp, err := observability.InitMeterProvider(ctx, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init meter provider: %w", err)
	}
	defer func() {
		// Use a timeout to prevent hanging if collector is unreachable
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := mp.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to shutdown meter provider", "error", err)
		}
	}()

	slog.InfoContext(ctx, "starting mono service", "env", cfg.Env)

	// Init Storage
	poolConfig := sqlstorage.DBConfig{
		MaxOpenConns:    cfg.DBMaxOpenConns,
		MaxIdleConns:    cfg.DBMaxIdleConns,
		ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetime) * time.Second,
		ConnMaxIdleTime: time.Duration(cfg.DBConnMaxIdleTime) * time.Second,
	}

	store, err := sqlstorage.NewPostgresStoreWithConfig(ctx, cfg.PostgresURL, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	defer store.Close()

	slog.InfoContext(ctx, "storage initialized", "url", maskPassword(cfg.PostgresURL))

	// Init Service
	svc := service.NewMonoService(store, cfg.DefaultPageSize, cfg.MaxPageSize)

	// Init Authentication
	authenticator := auth.NewAuthenticator(ctx, store.DB(), store.Queries())
	slog.InfoContext(ctx, "API key authentication enabled")

	// Init gRPC Server
	s, lis, err := createGRPCServer(ctx, cfg, svc, authenticator)
	if err != nil {
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}

	errResult := make(chan error, 1)
	go func() {
		if err := s.Serve(lis); err != nil {
			errResult <- fmt.Errorf("failed to serve gRPC: %w", err)
		}
	}()

	// Init REST Gateway (provides REST/JSON API for clients that don't speak gRPC)
	go startRESTGateway(ctx, cfg)

	// Orchestrate graceful shutdown or handle fatal errors
	// This coordination logic stays in run() to maintain visibility of the shutdown sequence
	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down")

		// Shutdown gRPC server with timeout
		// Note: REST gateway handles its own shutdown in startRESTGateway()
		shutdownCtx, cancel := newShutdownContext(cfg.ShutdownTimeout)
		defer cancel()

		done := make(chan struct{})
		go func() {
			s.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
			slog.InfoContext(shutdownCtx, "gRPC server shutdown complete")
		case <-shutdownCtx.Done():
			slog.WarnContext(shutdownCtx, "gRPC server shutdown timed out, forcing stop")
			s.Stop()
		}

		// Shutdown authenticator worker (drains pending last_used_at updates)
		if err := authenticator.Shutdown(shutdownCtx); err != nil {
			slog.WarnContext(shutdownCtx, "authenticator shutdown timeout", "error", err)
		} else {
			slog.InfoContext(shutdownCtx, "authenticator shutdown complete")
		}

		return nil
	case err := <-errResult:
		return err
	}
}

// createGRPCServer creates and configures the gRPC server with keepalive settings,
// authentication, and observability. Returns the gRPC server, listener, and any error.
func createGRPCServer(ctx context.Context, cfg *config.Config, svc *service.MonoService, authenticator *auth.Authenticator) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Configure gRPC keepalive parameters
	keepaliveParams := keepalive.ServerParameters{
		MaxConnectionIdle:     time.Duration(cfg.GRPCMaxConnectionIdle) * time.Second,
		MaxConnectionAge:      time.Duration(cfg.GRPCMaxConnectionAge) * time.Second,
		MaxConnectionAgeGrace: time.Duration(cfg.GRPCMaxConnectionAgeGrace) * time.Second,
		Time:                  time.Duration(cfg.GRPCKeepaliveTime) * time.Second,
		Timeout:               time.Duration(cfg.GRPCKeepaliveTimeout) * time.Second,
	}

	keepaliveEnforcementPolicy := keepalive.EnforcementPolicy{
		MinTime:             time.Duration(cfg.GRPCKeepaliveEnforcementMinTime) * time.Second,
		PermitWithoutStream: cfg.GRPCKeepaliveEnforcementPermitWithoutStream,
	}

	// Build server options with observability
	// The StatsHandler instruments all incoming gRPC calls with:
	// - Distributed tracing (creates spans for each RPC)
	// - Metrics (request count, duration, status codes)
	// - Automatic trace context extraction from client metadata
	serverOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepaliveParams),
		grpc.KeepaliveEnforcementPolicy(keepaliveEnforcementPolicy),
		grpc.ConnectionTimeout(time.Duration(cfg.GRPCConnectionTimeout) * time.Second),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}

	// Add authentication interceptor if available
	if authenticator != nil {
		serverOpts = append(serverOpts, grpc.UnaryInterceptor(authenticator.UnaryInterceptor))
	}

	s := grpc.NewServer(serverOpts...)
	monov1.RegisterMonoServiceServer(s, svc)

	slog.InfoContext(ctx, "gRPC server listening", "address", lis.Addr())

	return s, lis, nil
}

// startRESTGateway initializes and starts the REST gateway server.
// It provides a REST/JSON API for clients that don't speak gRPC, proxying requests
// to the gRPC server. The REST gateway handles its own graceful shutdown when ctx is cancelled.
func startRESTGateway(ctx context.Context, cfg *config.Config) {
	mux := runtime.NewServeMux()
	// Configure gRPC client options for REST gateway → gRPC server communication
	// The StatsHandler instruments outgoing gRPC calls from the gateway with:
	// - Trace context propagation (injects trace ID/span ID into gRPC metadata)
	// - Distributed tracing (creates client-side spans)
	// - Metrics (request count, duration, status codes)
	//
	// Trace flow: REST client → otelhttp (HTTP span) → gateway → otelgrpc.Client (propagates) → gRPC server → otelgrpc.Server (extracts)
	// Result: Single distributed trace with parent (REST) and child (gRPC) spans
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}

	// Register the REST gateway to communicate with the gRPC server
	// Use main context so REST gateway registration respects graceful shutdown signals
	grpcEndpoint := cfg.GRPCHost + ":" + cfg.GRPCPort
	if err := monov1.RegisterMonoServiceHandlerFromEndpoint(ctx, mux, grpcEndpoint, opts); err != nil {
		slog.ErrorContext(ctx, "failed to register gateway", "error", err)
		return
	}

	// Wrap the gateway mux with HTTP instrumentation
	// This creates spans for incoming REST/JSON requests and propagates trace context
	// The "mono-gateway" name appears as the service name in traces
	handler := otelhttp.NewHandler(mux, "mono-gateway")

	slog.InfoContext(ctx, "REST gateway listening", "port", cfg.RESTPort)
	server := &http.Server{
		Addr:              ":" + cfg.RESTPort,
		Handler:           handler,
		ReadHeaderTimeout: time.Duration(cfg.RESTReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.RESTWriteTimeout) * time.Second,
		IdleTimeout:       time.Duration(cfg.RESTIdleTimeout) * time.Second,
	}

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		// Create fresh shutdown context (main ctx is already cancelled)
		// This gives the REST gateway a timeout window to drain connections gracefully
		shutdownCtx, cancel := newShutdownContext(cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "failed to shutdown REST gateway", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.ErrorContext(ctx, "failed to serve REST gateway", "error", err)
	}
}

// newShutdownContext creates a fresh context with timeout for graceful shutdown operations.
// Uses Background() since the main context is already cancelled at shutdown time,
// but we still need a timeout window to complete cleanup operations.
func newShutdownContext(timeoutSeconds int) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
}

// maskPassword masks the password in a connection string for logging.
func maskPassword(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil {
		// If parsing fails, fall back to full redaction to be safe
		return "[REDACTED]"
	}
	// Check if there is a user info part
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			username := u.User.Username()
			u.User = url.UserPassword(username, "xxxxxx")
		}
	}
	return u.String()
}
