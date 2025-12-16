package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// InitTracerProvider initializes an OTLP tracer provider.
func InitTracerProvider(ctx context.Context, serviceName string, collectorAddr string, enabled bool) (*sdktrace.TracerProvider, error) {
	if !enabled {
		// Return a no-op provider or nil?
		// Ideally we return a provider that does nothing but satisfies interface.
		// For simplicity, let's return a noop provider.
		return sdktrace.NewTracerProvider(), nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(collectorAddr),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tracerProvider, nil
}

// InitMeterProvider initializes an OTLP meter provider.
func InitMeterProvider(ctx context.Context, serviceName string, collectorAddr string, enabled bool) (*sdkmetric.MeterProvider, error) {
	if !enabled {
		return sdkmetric.NewMeterProvider(), nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(collectorAddr),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
	)

	otel.SetMeterProvider(meterProvider)

	return meterProvider, nil
}

// InitLogger initializes an OTLP log provider and returns a structured logger.
func InitLogger(ctx context.Context, serviceName string, collectorAddr string, enabled bool) (*log.LoggerProvider, *slog.Logger, error) {
	if !enabled {
		// Return default no-op provider and stdout logger or something simple
		// Slog default matches behavior of just printing to stdout if no handler set?
		// But here we want to respect the calling code expecting an OTel-bridged one.
		// If disabled, just return standard JSON handler on stdout without OTel.
		return log.NewLoggerProvider(), slog.New(slog.NewJSONHandler(os.Stdout, nil)), nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	logExporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(collectorAddr),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewSimpleProcessor(logExporter)),
		log.WithResource(res),
	)

	// Create a bridge logger that uses the OTel provider
	hook := otelslog.NewLogger("todo-service", otelslog.WithLoggerProvider(loggerProvider))

	return loggerProvider, hook, nil
}
