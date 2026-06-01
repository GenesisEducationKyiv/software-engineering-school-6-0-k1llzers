package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	prometheusExporter "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	prometheusclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const meterName = "github-release-notifier"

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

	httpRequestsTotal         otelmetric.Int64Counter
	httpRequestErrorsTotal    otelmetric.Int64Counter
	httpRequestDuration       otelmetric.Float64Histogram
	outboxDispatchesTotal     otelmetric.Int64Counter
	outboxDispatchErrorsTotal otelmetric.Int64Counter
	outboxDispatchDuration    otelmetric.Float64Histogram
	releaseChecksTotal        otelmetric.Int64Counter
	releaseCheckErrorsTotal   otelmetric.Int64Counter
	releaseCheckDuration      otelmetric.Float64Histogram
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

	httpRequestsTotal, err := meter.Int64Counter(
		"github_release_notifier_http_requests_total",
		otelmetric.WithDescription("Total number of handled HTTP requests."),
	)
	if err != nil {
		return nil, err
	}

	httpRequestErrorsTotal, err := meter.Int64Counter(
		"github_release_notifier_http_request_errors_total",
		otelmetric.WithDescription("Total number of handled HTTP requests that returned a 4xx or 5xx status."),
	)
	if err != nil {
		return nil, err
	}

	httpRequestDuration, err := meter.Float64Histogram(
		"github_release_notifier_http_request_duration_seconds",
		otelmetric.WithDescription("Duration of handled HTTP requests."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(durationHistogramBucketsSeconds...),
	)
	if err != nil {
		return nil, err
	}

	outboxDispatchesTotal, err := meter.Int64Counter(
		"github_release_notifier_outbox_dispatches_total",
		otelmetric.WithDescription("Total number of processed outbox dispatch attempts."),
	)
	if err != nil {
		return nil, err
	}

	outboxDispatchErrorsTotal, err := meter.Int64Counter(
		"github_release_notifier_outbox_dispatch_errors_total",
		otelmetric.WithDescription("Total number of failed outbox dispatch attempts."),
	)
	if err != nil {
		return nil, err
	}

	outboxDispatchDuration, err := meter.Float64Histogram(
		"github_release_notifier_outbox_dispatch_duration_seconds",
		otelmetric.WithDescription("Duration of processed outbox dispatch attempts."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(durationHistogramBucketsSeconds...),
	)
	if err != nil {
		return nil, err
	}

	releaseChecksTotal, err := meter.Int64Counter(
		"github_release_notifier_release_monitor_checks_total",
		otelmetric.WithDescription("Total number of release monitor checks."),
	)
	if err != nil {
		return nil, err
	}

	releaseCheckErrorsTotal, err := meter.Int64Counter(
		"github_release_notifier_release_monitor_check_errors_total",
		otelmetric.WithDescription("Total number of failed release monitor checks."),
	)
	if err != nil {
		return nil, err
	}

	releaseCheckDuration, err := meter.Float64Histogram(
		"github_release_notifier_release_monitor_check_duration_seconds",
		otelmetric.WithDescription("Duration of release monitor checks."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(durationHistogramBucketsSeconds...),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		provider:                  provider,
		handler:                   promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
		httpRequestsTotal:         httpRequestsTotal,
		httpRequestErrorsTotal:    httpRequestErrorsTotal,
		httpRequestDuration:       httpRequestDuration,
		outboxDispatchesTotal:     outboxDispatchesTotal,
		outboxDispatchErrorsTotal: outboxDispatchErrorsTotal,
		outboxDispatchDuration:    outboxDispatchDuration,
		releaseChecksTotal:        releaseChecksTotal,
		releaseCheckErrorsTotal:   releaseCheckErrorsTotal,
		releaseCheckDuration:      releaseCheckDuration,
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

func (m *Metrics) ObserveHTTPRequest(ctx context.Context, method string, route string, status int, duration time.Duration) {
	if m == nil || m.httpRequestsTotal == nil || m.httpRequestDuration == nil {
		return
	}

	ctx = normalizeContext(ctx)
	statusCode := strconv.Itoa(status)
	statusClass := httpStatusClass(status)
	requestAttributes := otelmetric.WithAttributes(
		attribute.String("method", method),
		attribute.String("route", route),
		attribute.String("status_code", statusCode),
		attribute.String("status_class", statusClass),
	)
	durationAttributes := otelmetric.WithAttributes(
		attribute.String("method", method),
		attribute.String("route", route),
		attribute.String("status_class", statusClass),
	)

	m.httpRequestsTotal.Add(ctx, 1, requestAttributes)
	m.httpRequestDuration.Record(ctx, duration.Seconds(), durationAttributes)

	if status >= 400 && m.httpRequestErrorsTotal != nil {
		m.httpRequestErrorsTotal.Add(ctx, 1, requestAttributes)
	}
}

func (m *Metrics) ObserveOutboxDispatch(ctx context.Context, result string, duration time.Duration) {
	if m == nil || m.outboxDispatchesTotal == nil || m.outboxDispatchDuration == nil {
		return
	}

	ctx = normalizeContext(ctx)
	attributes := otelmetric.WithAttributes(attribute.String("result", result))

	m.outboxDispatchesTotal.Add(ctx, 1, attributes)
	m.outboxDispatchDuration.Record(ctx, duration.Seconds(), attributes)

	if result != "success" && m.outboxDispatchErrorsTotal != nil {
		m.outboxDispatchErrorsTotal.Add(ctx, 1, attributes)
	}
}

func (m *Metrics) ObserveReleaseMonitorCheck(ctx context.Context, result string, duration time.Duration) {
	if m == nil || m.releaseChecksTotal == nil || m.releaseCheckDuration == nil {
		return
	}

	ctx = normalizeContext(ctx)
	attributes := otelmetric.WithAttributes(attribute.String("result", result))

	m.releaseChecksTotal.Add(ctx, 1, attributes)
	m.releaseCheckDuration.Record(ctx, duration.Seconds(), attributes)

	if result != "success" && m.releaseCheckErrorsTotal != nil {
		m.releaseCheckErrorsTotal.Add(ctx, 1, attributes)
	}
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return ctx
}

func httpStatusClass(status int) string {
	return strconv.Itoa(status/100) + "xx"
}
