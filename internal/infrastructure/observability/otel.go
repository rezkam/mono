package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	// DefaultServiceName is the default service name for observability when OTEL_SERVICE_NAME is not set.
	DefaultServiceName = "mono"
)

// Config holds observability configuration.
type Config struct {
	Enabled     bool   // Whether OpenTelemetry is enabled
	ServiceName string // Service name for logging (defaults to DefaultServiceName if empty)
}

// newResource creates a resource with service metadata merged with defaults.
// Uses resource.Merge to combine default SDK attributes with custom service attributes.
// Handles partial resource errors gracefully as they are non-fatal.
//
// All attributes are set via OTEL_RESOURCE_ATTRIBUTES env var:
//
//	export OTEL_RESOURCE_ATTRIBUTES="service.name=mono,service.version=1.0.0,service.namespace=my-namespace,deployment.environment=production"
func newResource(ctx context.Context) (*resource.Resource, error) {
	// Create custom resource with service attributes
	// WithFromEnv() reads OTEL_RESOURCE_ATTRIBUTES (service.name, service.version, etc.) and OTEL_SERVICE_NAME
	serviceResource, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithSchemaURL(semconv.SchemaURL),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create service resource: %w", err)
	}

	// Merge with default resource (includes telemetry.sdk.* attributes)
	res, err := resource.Merge(
		resource.Default(),
		serviceResource,
	)
	if err != nil {
		// Handle partial resource or schema URL conflicts gracefully
		if errors.Is(err, resource.ErrPartialResource) || errors.Is(err, resource.ErrSchemaURLConflict) {
			// Non-fatal: resource is still usable
			return res, nil
		}
		return nil, fmt.Errorf("failed to merge resources: %w", err)
	}

	return res, nil
}

// InitTracerProvider initializes an OTLP tracer provider following OpenTelemetry best practices.
// Uses HTTP transport for compatibility with Grafana Cloud and other OTLP backends.
//
// Configuration via environment variables (standard OTEL env vars):
//   - OTEL_EXPORTER_OTLP_ENDPOINT: Full URL (e.g., https://otlp-gateway-prod-eu-north-0.grafana.net/otlp)
//   - OTEL_EXPORTER_OTLP_HEADERS: Auth headers (e.g., Authorization=Basic <base64-token>)
//   - OTEL_RESOURCE_ATTRIBUTES: Resource attributes (e.g., service.name=mono,service.version=1.0.0)
func InitTracerProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	if !cfg.Enabled {
		// Return a no-op provider that satisfies the interface
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return tp, nil
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, err
	}

	// Use context.Background() for exporter creation to avoid hanging on shutdown.
	// Headers are read from OTEL_EXPORTER_OTLP_HEADERS env var by the SDK.
	traceExporter, err := otlptracehttp.New(context.Background(),
		otlptracehttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Use WithBatcher (recommended) instead of manually creating BatchSpanProcessor.
	// Configure batch timeout for reasonable flush intervals.
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
	)

	// Set global tracer provider so instrumentation libraries can access it
	otel.SetTracerProvider(tracerProvider)

	// Set up W3C Trace Context and Baggage propagation for distributed tracing
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tracerProvider, nil
}

// InitMeterProvider initializes an OTLP meter provider following OpenTelemetry best practices.
// Uses HTTP transport for compatibility with Grafana Cloud and other OTLP backends.
//
// Configuration via environment variables (standard OTEL env vars):
//   - OTEL_EXPORTER_OTLP_ENDPOINT: Full URL
//   - OTEL_EXPORTER_OTLP_HEADERS: Auth headers
//   - OTEL_RESOURCE_ATTRIBUTES: Resource attributes (e.g., service.name=mono,service.version=1.0.0)
func InitMeterProvider(ctx context.Context, cfg Config) (*sdkmetric.MeterProvider, error) {
	if !cfg.Enabled {
		mp := sdkmetric.NewMeterProvider()
		otel.SetMeterProvider(mp)
		return mp, nil
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, err
	}

	// Use context.Background() for exporter creation to avoid hanging on shutdown.
	// Headers are read from OTEL_EXPORTER_OTLP_HEADERS env var by the SDK.
	metricExporter, err := otlpmetrichttp.New(context.Background(),
		otlpmetrichttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	// Configure PeriodicReader with reasonable collection interval
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(15*time.Second),
		)),
	)

	// Set global meter provider so instrumentation libraries can access it
	otel.SetMeterProvider(meterProvider)

	return meterProvider, nil
}

// InitLogger initializes an OTLP log provider and returns a structured logger.
// Uses HTTP transport for compatibility with Grafana Cloud and other OTLP backends.
//
// Configuration via environment variables (standard OTEL env vars):
//   - OTEL_EXPORTER_OTLP_ENDPOINT: Full URL
//   - OTEL_EXPORTER_OTLP_HEADERS: Auth headers
//   - OTEL_RESOURCE_ATTRIBUTES: Resource attributes (e.g., service.name=mono,service.version=1.0.0)
func InitLogger(ctx context.Context, cfg Config) (*log.LoggerProvider, *slog.Logger, error) {
	if !cfg.Enabled {
		// Return a no-op provider and stdout JSON logger when disabled
		return log.NewLoggerProvider(), slog.New(slog.NewJSONHandler(os.Stdout, nil)), nil
	}

	res, err := newResource(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Use context.Background() for exporter creation to avoid hanging on shutdown.
	// Headers are read from OTEL_EXPORTER_OTLP_HEADERS env var by the SDK.
	logExporter, err := otlploghttp.New(context.Background(),
		otlploghttp.WithTimeout(10*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	// Use BatchProcessor for production (more efficient than SimpleProcessor)
	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter,
			log.WithExportTimeout(5*time.Second),
		)),
		log.WithResource(res),
	)

	// Create a bridge logger that sends logs to OTel
	logger := otelslog.NewLogger(cfg.ServiceName, otelslog.WithLoggerProvider(loggerProvider))

	return loggerProvider, logger, nil
}
