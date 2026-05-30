package tracing

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "live-auction"

type ShutdownFunc func(context.Context) error

func Init(ctx context.Context, log *slog.Logger) ShutdownFunc {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if !envBool("OTEL_TRACES_ENABLED", false) {
		return func(context.Context) error { return nil }
	}
	normalizeOTLPDurationEnv("OTEL_EXPORTER_OTLP_TIMEOUT")
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(envEndpoint("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")),
		otlptracehttp.WithInsecure(),
		otlptracehttp.WithTimeout(envDuration("OTEL_EXPORTER_OTLP_TIMEOUT", 3*time.Second)),
	)
	if err != nil {
		if log != nil {
			log.Error("init otel trace exporter", slog.String("error", err.Error()))
		}
		return func(context.Context) error { return nil }
	}
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(envFloat("OTEL_TRACES_SAMPLER_RATIO", 1.0)))
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(500*time.Millisecond),
			sdktrace.WithExportTimeout(envDuration("OTEL_EXPORTER_OTLP_TIMEOUT", 3*time.Second)),
		),
		sdktrace.WithSampler(sampler),
		sdktrace.WithResource(resource.NewWithAttributes("",
			attribute.String("service.name", envString("OTEL_SERVICE_NAME", "live-auction-backend")),
			attribute.String("deployment.environment", envString("APP_ENV", "local")),
		)),
	)
	otel.SetTracerProvider(provider)
	if log != nil {
		log.Info("otel tracing enabled",
			slog.String("endpoint", envEndpoint("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4318")),
			slog.Float64("sample_ratio", envFloat("OTEL_TRACES_SAMPLER_RATIO", 1.0)),
		)
	}
	return provider.Shutdown
}

func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

func StartServer(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
	)
}

func ExtractHTTP(ctx context.Context, header map[string][]string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(header))
}

func InjectHTTP(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}

func SpanTraceID(ctx context.Context) string {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if !spanCtx.HasTraceID() {
		return ""
	}
	return spanCtx.TraceID().String()
}

func End(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

func envString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envEndpoint(key string, fallback string) string {
	return strings.TrimPrefix(strings.TrimPrefix(envString(key, fallback), "http://"), "https://")
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if ms, err := strconv.Atoi(value); err == nil {
		return time.Duration(ms) * time.Millisecond
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func normalizeOTLPDurationEnv(key string) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return
	}
	if _, err := strconv.Atoi(value); err == nil {
		return
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return
	}
	_ = os.Setenv(key, strconv.FormatInt(parsed.Milliseconds(), 10))
}
