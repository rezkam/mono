package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	monov1 "github.com/rezkam/mono/api/proto/mono/v1"
	"github.com/rezkam/mono/internal/config"
	"github.com/rezkam/mono/internal/core"
	"github.com/rezkam/mono/internal/service"
	"github.com/rezkam/mono/internal/storage/fs"
	"github.com/rezkam/mono/internal/storage/gcs"
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
	// 1. Load Configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 2. Init Observability (Logger, Tracer, Meter)
	// We init logger first to use it for startup logs
	lp, logger, err := observability.InitLogger(ctx, "mono-service", cfg.OTelCollector, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer func() {
		if err := lp.Shutdown(context.Background()); err != nil {
			fmt.Printf("failed to shutdown logger provider: %v\n", err)
		}
	}()
	// Set generic logger as default for now
	slog.SetDefault(logger)

	tp, err := observability.InitTracerProvider(ctx, "mono-service", cfg.OTelCollector, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init tracer provider: %w", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown tracer provider", "error", err)
		}
	}()

	mp, err := observability.InitMeterProvider(ctx, "mono-service", cfg.OTelCollector, cfg.OTelEnabled)
	if err != nil {
		return fmt.Errorf("failed to init meter provider: %w", err)
	}
	defer func() {
		if err := mp.Shutdown(context.Background()); err != nil {
			slog.Error("failed to shutdown meter provider", "error", err)
		}
	}()

	slog.Info("starting mono service", "env", cfg.Env, "storage", cfg.StorageType)

	// 3. Init Storage
	var store core.Storage
	switch cfg.StorageType {
	case "gcs":
		if cfg.GCSBucket == "" {
			return errors.New("bucket is required for gcs storage")
		}
		store, err = gcs.NewStore(ctx, cfg.GCSBucket)
		if err != nil {
			return fmt.Errorf("failed to create gcs store: %w", err)
		}
	case "fs":
		store, err = fs.NewStore(cfg.FSDir)
		if err != nil {
			return fmt.Errorf("failed to create fs store: %w", err)
		}
	case "postgres":
		poolConfig := sqlstorage.DBConfig{
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetime) * time.Second,
			ConnMaxIdleTime: time.Duration(cfg.DBConnMaxIdleTime) * time.Second,
		}
		store, err = sqlstorage.NewPostgresStoreWithConfig(ctx, cfg.PostgresURL, poolConfig)
		if err != nil {
			return fmt.Errorf("failed to create postgres store: %w", err)
		}
		slog.Info("using postgres storage", "url", maskPassword(cfg.PostgresURL))
	case "sqlite":
		poolConfig := sqlstorage.DBConfig{
			MaxOpenConns:    cfg.DBMaxOpenConns,
			MaxIdleConns:    cfg.DBMaxIdleConns,
			ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetime) * time.Second,
			ConnMaxIdleTime: time.Duration(cfg.DBConnMaxIdleTime) * time.Second,
		}
		store, err = sqlstorage.NewSQLiteStoreWithConfig(ctx, cfg.SQLitePath, poolConfig)
		if err != nil {
			return fmt.Errorf("failed to create sqlite store: %w", err)
		}
		slog.Info("using sqlite storage", "path", cfg.SQLitePath)
	default:
		return fmt.Errorf("unknown storage type: %s", cfg.StorageType)
	}

	// 4. Init Service
	svc := service.NewMonoService(store)

	// 5. Init gRPC Server
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	monov1.RegisterMonoServiceServer(s, svc)

	slog.Info("gRPC server listening", "address", lis.Addr())

	errResult := make(chan error, 1)
	go func() {
		if err := s.Serve(lis); err != nil {
			errResult <- fmt.Errorf("failed to serve gRPC: %w", err)
		}
	}()

	// 6. Init HTTP Gateway
	go func() {
		mux := runtime.NewServeMux()
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}

		// Register the gateway to talk to the gRPC server
		gwCtx := context.Background()
		// Gateway talks to localhost:GRPC_PORT
		if err := monov1.RegisterMonoServiceHandlerFromEndpoint(gwCtx, mux, "localhost:"+cfg.GRPCPort, opts); err != nil {
			slog.Error("failed to register gateway", "error", err)
			return
		}

		handler := otelhttp.NewHandler(mux, "mono-gateway")

		slog.Info("HTTP gateway listening", "port", cfg.HTTPPort)
		server := &http.Server{
			Addr:              ":" + cfg.HTTPPort,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second, // Production readiness: set timeouts
		}

		go func() {
			<-ctx.Done()
			server.Shutdown(context.Background())
		}()

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to serve HTTP gateway", "error", err)
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		s.GracefulStop()
		return nil
	case err := <-errResult:
		return err
	}
}

// maskPassword masks the password in a connection string for logging.
func maskPassword(connStr string) string {
	// Simple masking for PostgreSQL connection strings
	// Format: postgres://user:password@host:port/dbname
	// We'll just return a generic message to avoid logging sensitive info
	return "[REDACTED]"
}
