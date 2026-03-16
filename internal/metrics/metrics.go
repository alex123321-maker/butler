package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	MetricRequestsTotal          = "butler_requests_total"
	MetricErrorsTotal            = "butler_errors_total"
	MetricRequestDurationSeconds = "butler_request_duration_seconds"
	MetricMemoryJobsTotal        = "butler_memory_jobs_total"
	MetricMemoryWritesTotal      = "butler_memory_writes_total"
	MetricMemoryRetrievalsTotal  = "butler_memory_retrievals_total"
	MetricMemoryQueueDepth       = "butler_memory_queue_depth"
	MetricDoctorChecksTotal      = "butler_doctor_checks_total"
)

type Registry struct {
	registry         *prometheus.Registry
	mu               sync.Mutex
	counters         map[string]*prometheus.CounterVec
	gauges           map[string]*prometheus.GaugeVec
	histograms       map[string]*prometheus.HistogramVec
	metricLabelNames map[string][]string
}

func New() *Registry {
	r := &Registry{
		registry:         prometheus.NewRegistry(),
		counters:         make(map[string]*prometheus.CounterVec),
		gauges:           make(map[string]*prometheus.GaugeVec),
		histograms:       make(map[string]*prometheus.HistogramVec),
		metricLabelNames: make(map[string][]string),
	}

	r.mustRegisterCounter(MetricRequestsTotal, "Total number of Butler requests.", []string{"operation", "service", "status"})
	r.mustRegisterCounter(MetricErrorsTotal, "Total number of Butler errors.", []string{"error_class", "operation", "service"})
	r.mustRegisterHistogram(MetricRequestDurationSeconds, "Latency of Butler request handling in seconds.", []string{"operation", "service"})
	r.mustRegisterCounter(MetricMemoryJobsTotal, "Total number of Butler memory jobs.", []string{"job_type", "service", "status"})
	r.mustRegisterCounter(MetricMemoryWritesTotal, "Total number of Butler memory writes.", []string{"memory_type", "service", "status"})
	r.mustRegisterCounter(MetricMemoryRetrievalsTotal, "Total number of Butler memory retrieval operations.", []string{"memory_type", "service", "status"})
	r.mustRegisterCounter(MetricDoctorChecksTotal, "Total number of Butler doctor checks.", []string{"component", "service", "status"})
	r.mu.Lock()
	queueGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: MetricMemoryQueueDepth, Help: "Depth of Butler memory queues."}, []string{"queue", "service"})
	r.registry.MustRegister(queueGauge)
	r.gauges[MetricMemoryQueueDepth] = queueGauge
	r.metricLabelNames[MetricMemoryQueueDepth] = []string{"queue", "service"}
	r.mu.Unlock()

	return r
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

func (r *Registry) IncrCounter(name string, labels map[string]string) error {
	vec, keys, err := r.counter(name, labels)
	if err != nil {
		return err
	}
	vec.WithLabelValues(labelValues(keys, labels)...).Inc()
	return nil
}

func (r *Registry) ObserveHistogram(name string, value float64, labels map[string]string) error {
	vec, keys, err := r.histogram(name, labels)
	if err != nil {
		return err
	}
	vec.WithLabelValues(labelValues(keys, labels)...).Observe(value)
	return nil
}

func (r *Registry) SetGauge(name string, value float64, labels map[string]string) error {
	vec, keys, err := r.gauge(name, labels)
	if err != nil {
		return err
	}
	vec.WithLabelValues(labelValues(keys, labels)...).Set(value)
	return nil
}

func (r *Registry) counter(name string, labels map[string]string) (*prometheus.CounterVec, []string, error) {
	keys := labelKeys(labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if vec, ok := r.counters[name]; ok {
		if err := r.ensureLabels(name, keys); err != nil {
			return nil, nil, err
		}
		return vec, keys, nil
	}

	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: dynamicHelp(name)}, keys)
	if err := r.registry.Register(vec); err != nil {
		return nil, nil, fmt.Errorf("register counter %s: %w", name, err)
	}
	r.counters[name] = vec
	r.metricLabelNames[name] = append([]string(nil), keys...)
	return vec, keys, nil
}

func (r *Registry) histogram(name string, labels map[string]string) (*prometheus.HistogramVec, []string, error) {
	keys := labelKeys(labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if vec, ok := r.histograms[name]; ok {
		if err := r.ensureLabels(name, keys); err != nil {
			return nil, nil, err
		}
		return vec, keys, nil
	}

	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: dynamicHelp(name), Buckets: prometheus.DefBuckets}, keys)
	if err := r.registry.Register(vec); err != nil {
		return nil, nil, fmt.Errorf("register histogram %s: %w", name, err)
	}
	r.histograms[name] = vec
	r.metricLabelNames[name] = append([]string(nil), keys...)
	return vec, keys, nil
}

func (r *Registry) gauge(name string, labels map[string]string) (*prometheus.GaugeVec, []string, error) {
	keys := labelKeys(labels)
	r.mu.Lock()
	defer r.mu.Unlock()

	if vec, ok := r.gauges[name]; ok {
		if err := r.ensureLabels(name, keys); err != nil {
			return nil, nil, err
		}
		return vec, keys, nil
	}

	vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: dynamicHelp(name)}, keys)
	if err := r.registry.Register(vec); err != nil {
		return nil, nil, fmt.Errorf("register gauge %s: %w", name, err)
	}
	r.gauges[name] = vec
	r.metricLabelNames[name] = append([]string(nil), keys...)
	return vec, keys, nil
}

func (r *Registry) mustRegisterCounter(name, help string, labels []string) {
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	r.registry.MustRegister(vec)
	r.counters[name] = vec
	r.metricLabelNames[name] = append([]string(nil), labels...)
}

func (r *Registry) mustRegisterHistogram(name, help string, labels []string) {
	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help, Buckets: prometheus.DefBuckets}, labels)
	r.registry.MustRegister(vec)
	r.histograms[name] = vec
	r.metricLabelNames[name] = append([]string(nil), labels...)
}

func (r *Registry) ensureLabels(name string, got []string) error {
	want, ok := r.metricLabelNames[name]
	if !ok {
		return fmt.Errorf("metric %s is not registered", name)
	}
	if strings.Join(want, ",") != strings.Join(got, ",") {
		return fmt.Errorf("metric %s already registered with labels [%s], got [%s]", name, strings.Join(want, ","), strings.Join(got, ","))
	}
	return nil
}

func labelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func labelValues(keys []string, labels map[string]string) []string {
	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, labels[key])
	}
	return values
}

func dynamicHelp(name string) string {
	return fmt.Sprintf("Butler metric %s.", name)
}
