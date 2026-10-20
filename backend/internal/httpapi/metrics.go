package httpapi

import (
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type requestMetric struct {
	Count   int64
	Errors  int64
	TotalMS float64
	MaxMS   float64
}

type metricsStore struct {
	mu        sync.Mutex
	routes    map[string]*requestMetric
	durations []float64
	started   time.Time
}

var metrics = &metricsStore{routes: map[string]*requestMetric{}, started: time.Now()}

var uuidSegment = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func normalizePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if uuidSegment.MatchString(part) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

func (m *metricsStore) record(path string, status int, duration time.Duration) {
	key := normalizePath(path)
	ms := float64(duration.Microseconds()) / 1000
	m.mu.Lock()
	defer m.mu.Unlock()
	metric, ok := m.routes[key]
	if !ok {
		metric = &requestMetric{}
		m.routes[key] = metric
	}
	metric.Count++
	if status >= 500 {
		metric.Errors++
	}
	metric.TotalMS += ms
	if ms > metric.MaxMS {
		metric.MaxMS = ms
	}
	m.durations = append(m.durations, ms)
	if len(m.durations) > 20000 {
		m.durations = m.durations[len(m.durations)-20000:]
	}
}

func (m *metricsStore) snapshot() (p50, p95, p99 float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.durations) == 0 {
		return 0, 0, 0
	}
	values := append([]float64(nil), m.durations...)
	sort.Float64s(values)
	pick := func(q float64) float64 {
		index := int(q * float64(len(values)-1))
		return values[index]
	}
	return pick(0.50), pick(0.95), pick(0.99)
}

func (m *metricsStore) handler(w http.ResponseWriter, _ *http.Request) {
	p50, p95, p99 := m.snapshot()
	m.mu.Lock()
	routes := make([]string, 0, len(m.routes))
	for route := range m.routes {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	var builder strings.Builder
	builder.WriteString("# HELP tabel_http_latency_ms Latency percentiles over recent requests\n")
	builder.WriteString("# TYPE tabel_http_latency_ms gauge\n")
	builder.WriteString("tabel_http_latency_ms{quantile=\"0.5\"} " + strconv.FormatFloat(p50, 'f', 3, 64) + "\n")
	builder.WriteString("tabel_http_latency_ms{quantile=\"0.95\"} " + strconv.FormatFloat(p95, 'f', 3, 64) + "\n")
	builder.WriteString("tabel_http_latency_ms{quantile=\"0.99\"} " + strconv.FormatFloat(p99, 'f', 3, 64) + "\n")
	builder.WriteString("# HELP tabel_http_requests_total Total requests by route\n")
	builder.WriteString("# TYPE tabel_http_requests_total counter\n")
	for _, route := range routes {
		metric := m.routes[route]
		label := strings.ReplaceAll(route, `"`, `\"`)
		builder.WriteString(`tabel_http_requests_total{route="` + label + `"} ` + strconv.FormatInt(metric.Count, 10) + "\n")
		builder.WriteString(`tabel_http_errors_total{route="` + label + `"} ` + strconv.FormatInt(metric.Errors, 10) + "\n")
	}
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(builder.String()))
}

func withMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		metrics.record(r.URL.Path, recorder.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
