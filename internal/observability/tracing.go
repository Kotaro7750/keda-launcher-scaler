package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/config"
)

func NewTracerProvider(ctx context.Context, cfg config.Config) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			attribute.String("service.name", cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, nil, err
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.OTLPEndpoint != "" {
		exporterOptions := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		}
		if cfg.OTLPInsecure {
			exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
		}

		exporter, err := otlptracegrpc.New(ctx, exporterOptions...)
		if err != nil {
			return nil, nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(options...)
	return provider, provider.Shutdown, nil
}
