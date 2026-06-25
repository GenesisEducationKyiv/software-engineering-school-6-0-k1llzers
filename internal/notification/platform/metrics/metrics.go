package metrics

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	prometheusExporter "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	prometheusclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const meterName = "github-release-notifier-notification-service"

var durationHistogramBucketsSeconds = []float64{
	0.005,
	0.01,
	0.025,
	0.05,
	0.075,
	0.1,
	0.25,
	0.5,
	0.75,
	1,
	2.5,
	5,
	7.5,
	10,
}

type Metrics struct {
	provider *sdkmetric.MeterProvider
	handler  http.Handler

	emailDeliveryRequestsTotal otelmetric.Int64Counter
	emailDeliveryErrorsTotal   otelmetric.Int64Counter
	emailDeliveryDuration      otelmetric.Float64Histogram
}

func New() (*Metrics, error) {
	registry := prometheusclient.NewRegistry()
	exporter, err := prometheusExporter.New(
		prometheusExporter.WithRegisterer(registry),
	)
	if err != nil {
		return nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	meter := provider.Meter(meterName)

	emailDeliveryRequestsTotal, err := meter.Int64Counter(
		"github_release_notifier_notification_email_delivery_requests_total",
		otelmetric.WithDescription("Total number of notification email delivery attempts."),
	)
	if err != nil {
		return nil, err
	}

	emailDeliveryErrorsTotal, err := meter.Int64Counter(
		"github_release_notifier_notification_email_delivery_errors_total",
		otelmetric.WithDescription("Total number of failed notification email delivery attempts."),
	)
	if err != nil {
		return nil, err
	}

	emailDeliveryDuration, err := meter.Float64Histogram(
		"github_release_notifier_notification_email_delivery_duration_seconds",
		otelmetric.WithDescription("Duration of notification email delivery attempts."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(durationHistogramBucketsSeconds...),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		provider:                   provider,
		handler:                    promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		emailDeliveryRequestsTotal: emailDeliveryRequestsTotal,
		emailDeliveryErrorsTotal:   emailDeliveryErrorsTotal,
		emailDeliveryDuration:      emailDeliveryDuration,
	}, nil
}

func (m *Metrics) Shutdown(ctx context.Context) error {
	if m == nil || m.provider == nil {
		return nil
	}

	return m.provider.Shutdown(normalizeContext(ctx))
}

func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return nil
	}

	return m.handler
}

func (m *Metrics) ObserveEmailDelivery(ctx context.Context, notificationType string, success bool, duration time.Duration) {
	if m == nil || m.emailDeliveryRequestsTotal == nil || m.emailDeliveryDuration == nil {
		return
	}

	ctx = normalizeContext(ctx)
	attributes := otelmetric.WithAttributes(attribute.String("notification_type", notificationType))

	m.emailDeliveryRequestsTotal.Add(ctx, 1, attributes)
	m.emailDeliveryDuration.Record(ctx, duration.Seconds(), attributes)

	if !success && m.emailDeliveryErrorsTotal != nil {
		m.emailDeliveryErrorsTotal.Add(ctx, 1, attributes)
	}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}
