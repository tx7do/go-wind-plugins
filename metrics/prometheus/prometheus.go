// Package prometheus provides a [metrics.Metrics] implementation backed by the
// Prometheus client_golang library.
//
// Metrics are lazily registered with the default Prometheus registry on first
// use. A /metrics HTTP endpoint can be exposed by mounting the standard
// promhttp.Handler().
//
// Example:
//
//	m, _ := prometheus.New(prometheus.WithNamespace("myapp"))
//	m.Counter(ctx, "requests_total", 1, map[string]string{"method": "GET"})
//
//	// Expose /metrics endpoint
//	http.Handle("/metrics", promhttp.Handler())
//	http.ListenAndServe(":9090", nil)
package prometheus

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/tx7do/go-wind-plugins/metrics"
)

var _ metrics.Metrics = (*Provider)(nil)

// Option configures the Prometheus metrics provider.
type Option func(*config)

type config struct {
	namespace   string
	subsystem   string
	registry    prometheus.Registerer
	registrySet bool
}

func defaultConfig() *config {
	return &config{
		registry: prometheus.DefaultRegisterer,
	}
}

// WithNamespace sets the metric namespace prefix.
func WithNamespace(ns string) Option {
	return func(c *config) { c.namespace = ns }
}

// WithSubsystem sets the metric subsystem prefix.
func WithSubsystem(sub string) Option {
	return func(c *config) { c.subsystem = sub }
}

// WithRegistry sets a custom Prometheus registerer (useful for testing).
// Both New and NewWithDefaultRegistry honour an explicitly-set registerer.
func WithRegistry(r prometheus.Registerer) Option {
	return func(c *config) {
		c.registry = r
		c.registrySet = true
	}
}

// Provider implements [metrics.Metrics] using the Prometheus client library.
// Metrics are created lazily and cached so that subsequent calls with the same
// name/label-set reuse the existing instrument.
type Provider struct {
	cfg           *config
	factory       promauto.Factory
	counters      map[string]prometheus.Counter
	counterVecs   map[string]*prometheus.CounterVec
	histograms    map[string]prometheus.Histogram
	histogramVecs map[string]*prometheus.HistogramVec
	gauges        map[string]prometheus.Gauge
	gaugeVecs     map[string]*prometheus.GaugeVec
	labelNames    map[string][]string
}

// New creates a Prometheus-backed metrics provider. Unless a registry is
// supplied via WithRegistry, it installs its own fresh registry so metrics
// never leak into prometheus.DefaultRegisterer.
func New(opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if !cfg.registrySet {
		cfg.registry = prometheus.NewRegistry()
	}

	return &Provider{
		cfg:           cfg,
		factory:       promauto.With(cfg.registry),
		counters:      make(map[string]prometheus.Counter),
		counterVecs:   make(map[string]*prometheus.CounterVec),
		histograms:    make(map[string]prometheus.Histogram),
		histogramVecs: make(map[string]*prometheus.HistogramVec),
		gauges:        make(map[string]prometheus.Gauge),
		gaugeVecs:     make(map[string]*prometheus.GaugeVec),
		labelNames:    make(map[string][]string),
	}, nil
}

// NewWithDefaultRegistry creates a provider using the default Prometheus
// registry (prometheus.DefaultRegisterer). Use this when you want the metrics
// to be exposed via the standard promhttp.Handler().
func NewWithDefaultRegistry(opts ...Option) (*Provider, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return &Provider{
		cfg:           cfg,
		factory:       promauto.With(cfg.registry),
		counters:      make(map[string]prometheus.Counter),
		counterVecs:   make(map[string]*prometheus.CounterVec),
		histograms:    make(map[string]prometheus.Histogram),
		histogramVecs: make(map[string]*prometheus.HistogramVec),
		gauges:        make(map[string]prometheus.Gauge),
		gaugeVecs:     make(map[string]*prometheus.GaugeVec),
		labelNames:    make(map[string][]string),
	}, nil
}

// Registry returns the Prometheus gatherer used by this provider.
// Useful for passing to promhttp.HandlerFor().
func (p *Provider) Registry() prometheus.Gatherer {
	if r, ok := p.cfg.registry.(*prometheus.Registry); ok {
		return r
	}
	return prometheus.DefaultGatherer
}

func (p *Provider) labelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	return keys
}

func (p *Provider) labelValues(labels map[string]string, keys []string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = labels[k]
	}
	return vals
}

func (p *Provider) cachedLabelKeys(name string, labels map[string]string) []string {
	if keys, ok := p.labelNames[name]; ok {
		return keys
	}
	keys := p.labelKeys(labels)
	p.labelNames[name] = keys
	return keys
}

// mustCreate runs the given instrument factory for the named metric. promauto
// panics with a bare "duplicate metrics collector registration attempted"
// when the fully qualified metric name is already registered with different
// label dimensions — e.g. recorded first without labels and later with labels
// (or vice versa). That panic is recovered and re-raised with a message that
// names the conflicting metric.
func mustCreate[T any](name string, create func() T) (t T) {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Sprintf("prometheus: metric %q already registered with different label dimensions (%v)", name, r))
		}
	}()
	return create()
}

// Counter implements [metrics.Metrics].
func (p *Provider) Counter(ctx context.Context, name string, value float64, labels map[string]string) {
	_ = ctx
	if len(labels) == 0 {
		c, ok := p.counters[name]
		if !ok {
			c = mustCreate(name, func() prometheus.Counter {
				return p.factory.NewCounter(prometheus.CounterOpts{
					Namespace: p.cfg.namespace,
					Subsystem: p.cfg.subsystem,
					Name:      name,
				})
			})
			p.counters[name] = c
		}
		c.Add(value)
		return
	}

	keys := p.cachedLabelKeys(name, labels)
	cv, ok := p.counterVecs[name]
	if !ok {
		cv = mustCreate(name, func() *prometheus.CounterVec {
			return p.factory.NewCounterVec(prometheus.CounterOpts{
				Namespace: p.cfg.namespace,
				Subsystem: p.cfg.subsystem,
				Name:      name,
			}, keys)
		})
		p.counterVecs[name] = cv
	}
	cv.WithLabelValues(p.labelValues(labels, keys)...).Add(value)
}

// Histogram implements [metrics.Metrics].
func (p *Provider) Histogram(ctx context.Context, name string, value float64, labels map[string]string) {
	_ = ctx
	if len(labels) == 0 {
		h, ok := p.histograms[name]
		if !ok {
			h = mustCreate(name, func() prometheus.Histogram {
				return p.factory.NewHistogram(prometheus.HistogramOpts{
					Namespace: p.cfg.namespace,
					Subsystem: p.cfg.subsystem,
					Name:      name,
				})
			})
			p.histograms[name] = h
		}
		h.Observe(value)
		return
	}

	keys := p.cachedLabelKeys(name, labels)
	hv, ok := p.histogramVecs[name]
	if !ok {
		hv = mustCreate(name, func() *prometheus.HistogramVec {
			return p.factory.NewHistogramVec(prometheus.HistogramOpts{
				Namespace: p.cfg.namespace,
				Subsystem: p.cfg.subsystem,
				Name:      name,
			}, keys)
		})
		p.histogramVecs[name] = hv
	}
	hv.WithLabelValues(p.labelValues(labels, keys)...).Observe(value)
}

// Gauge implements [metrics.Metrics].
func (p *Provider) Gauge(ctx context.Context, name string, value float64, labels map[string]string) {
	_ = ctx
	if len(labels) == 0 {
		g, ok := p.gauges[name]
		if !ok {
			g = mustCreate(name, func() prometheus.Gauge {
				return p.factory.NewGauge(prometheus.GaugeOpts{
					Namespace: p.cfg.namespace,
					Subsystem: p.cfg.subsystem,
					Name:      name,
				})
			})
			p.gauges[name] = g
		}
		g.Set(value)
		return
	}

	keys := p.cachedLabelKeys(name, labels)
	gv, ok := p.gaugeVecs[name]
	if !ok {
		gv = mustCreate(name, func() *prometheus.GaugeVec {
			return p.factory.NewGaugeVec(prometheus.GaugeOpts{
				Namespace: p.cfg.namespace,
				Subsystem: p.cfg.subsystem,
				Name:      name,
			}, keys)
		})
		p.gaugeVecs[name] = gv
	}
	gv.WithLabelValues(p.labelValues(labels, keys)...).Set(value)
}
