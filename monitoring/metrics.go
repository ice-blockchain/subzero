// SPDX-License-Identifier: ice License 1.0

package monitoring

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/IBM/pgxpoolprometheus"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	activeWSConnectionsCounter atomic.Int64
	authenticatedUsersCounter  atomic.Int64
	activeSubscriptionsCounter atomic.Int64

	globalPrometheusCollector *PrometheusCollector
	prometheusCollectorOnce   sync.Once

	// === DATABASE QUERY METRICS ===
	dbQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "subzero_db_query_duration_seconds",
			Help:    "Database query execution duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"host", "operation", "success", "query_type"},
	)

	dbQueryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subzero_db_queries_total",
			Help: "Total number of database queries executed",
		},
		[]string{"host", "operation", "query_type", "success"},
	)

	// === CONNECTION POOL METRICS ===
	dbConnectionEstablishDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "subzero_db_connection_establish_duration_seconds",
			Help:    "Time spent establishing new database connection",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"host", "success"},
	)

	// === END-TO-END REQUEST METRICS ===
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "subzero_request_duration_seconds",
			Help:    "End-to-end request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"operation", "success", "protocol"},
	)

	requestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subzero_requests_total",
			Help: "Total number of requests processed",
		},
		[]string{"operation", "success", "protocol"},
	)

	// === WEBSOCKET METRICS ===
	wsOperationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "subzero_ws_operation_duration_seconds",
			Help:    "WebSocket operation duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"operation", "success"},
	)

	wsOperationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subzero_ws_operations_total",
			Help: "Total number of WebSocket operations",
		},
		[]string{"operation", "success"},
	)

	wsActiveConnections = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subzero_ws_active_connections",
			Help: "Number of active WebSocket connections",
		},
		[]string{"type"},
	)

	// === NOSTR-SPECIFIC METRICS ===
	authenticatedUsers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "subzero_authenticated_users",
			Help: "Number of currently authenticated users",
		},
	)

	activeSubscriptions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "subzero_subscriptions_active",
			Help: "Number of active subscriptions",
		},
	)

	eventsValidation = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subzero_events_validation_total",
			Help: "Total number of event validation results",
		},
		[]string{"result", "reason"},
	)

	eventsStored = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "subzero_events_stored_total",
			Help: "Total number of events successfully stored",
		},
	)

	// === DATABASE SIZE METRICS ===
	dbSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subzero_db_size_bytes",
			Help: "Database size in bytes",
		},
		[]string{"database", "host"},
	)

	dbEventsCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subzero_db_events_count",
			Help: "Total number of events in database",
		},
		[]string{"database", "host"},
	)

	// === STORAGE METRICS ===
	storageSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "subzero_storage_size_bytes",
			Help: "Storage size in bytes",
		},
	)

	storageRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subzero_storage_requests_total",
			Help: "Total number of storage requests",
		},
		[]string{"operation", "success"},
	)
)

type (
	PrometheusCollector struct {
		registeredHosts map[string]bool
		mutex           sync.Mutex
	}
	AdvancedDBTracer struct {
		host     string
		poolName string
	}
)

func MustInit() {
	prometheusCollectorOnce.Do(func() {
		globalPrometheusCollector = &PrometheusCollector{
			registeredHosts: make(map[string]bool),
		}
	})
}

func NewAdvancedDBTracer(host, poolName string) *AdvancedDBTracer {
	return &AdvancedDBTracer{
		host:     host,
		poolName: poolName,
	}
}

func (pc *PrometheusCollector) RegisterPgxPool(pool *pgxpool.Pool, dbName, host string) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()

	if pc.registeredHosts[host] {
		return
	}
	collector := pgxpoolprometheus.NewCollector(pool, map[string]string{
		"db_name": dbName,
		"host":    host,
	})
	prometheus.MustRegister(collector)
	pc.registeredHosts[host] = true
}

func RecordDBQuery(host, operation, queryType string, duration time.Duration, success bool) {
	successStr := "true"
	if !success {
		successStr = "false"
	}

	dbQueryDuration.WithLabelValues(host, operation, successStr, queryType).Observe(duration.Seconds())
	dbQueryTotal.WithLabelValues(host, operation, queryType, successStr).Inc()
}

func RecordConnectionEstablish(host string, duration time.Duration, success bool) {
	successStr := "true"
	if !success {
		successStr = "false"
	}
	dbConnectionEstablishDuration.WithLabelValues(host, successStr).Observe(duration.Seconds())
}

func RecordWSOperation(operation string, duration time.Duration, success bool, count int) {
	successStr := "true"
	if !success {
		successStr = "false"
	}

	wsOperationDuration.WithLabelValues(operation, successStr).Observe(duration.Seconds())
	wsOperationTotal.WithLabelValues(operation, successStr).Add(float64(count))
}

func IncreaseActiveConnections(connectionType string) {
	count := activeWSConnectionsCounter.Add(1)
	wsActiveConnections.WithLabelValues(connectionType).Set(float64(count))
}

func DecreaseActiveConnections(connectionType string) {
	count := activeWSConnectionsCounter.Add(-1)
	wsActiveConnections.WithLabelValues(connectionType).Set(float64(count))
}

func IncreaseAuthenticatedUsers() {
	count := authenticatedUsersCounter.Add(1)
	authenticatedUsers.Set(float64(count))
}

func DecreaseAuthenticatedUsers() {
	count := authenticatedUsersCounter.Add(-1)
	authenticatedUsers.Set(float64(count))
}

func IncreaseActiveSubscriptions() {
	count := activeSubscriptionsCounter.Add(1)
	activeSubscriptions.Set(float64(count))
}

func DecreaseActiveSubscriptions(delta int64) {
	count := activeSubscriptionsCounter.Add(-delta)
	activeSubscriptions.Set(float64(count))
}

func RecordRequest(operation, protocol string, duration time.Duration, success bool) {
	successStr := "true"
	if !success {
		successStr = "false"
	}

	requestDuration.WithLabelValues(operation, successStr, protocol).Observe(duration.Seconds())
	requestTotal.WithLabelValues(operation, successStr, protocol).Inc()
}

func (t *AdvancedDBTracer) TraceConnectStart(ctx context.Context, data pgx.TraceConnectStartData) context.Context {
	return context.WithValue(ctx, "connect_start_time", time.Now())
}

func (t *AdvancedDBTracer) TraceConnectEnd(ctx context.Context, data pgx.TraceConnectEndData) {
	if startTime, ok := ctx.Value("connect_start_time").(time.Time); ok {
		duration := time.Since(startTime)
		RecordConnectionEstablish(t.host, duration, data.Err == nil)
	}
}

func (t *AdvancedDBTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = context.WithValue(ctx, "query_start_time", time.Now())

	queryType := getQueryType(data.SQL)
	ctx = context.WithValue(ctx, "query_type", queryType)
	operation := queryType
	ctx = context.WithValue(ctx, "query_operation", operation)

	return ctx
}

func (t *AdvancedDBTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if startTime, ok := ctx.Value("query_start_time").(time.Time); ok {
		duration := time.Since(startTime)
		success := data.Err == nil

		operation := "query"
		if op, ok := ctx.Value("query_operation").(string); ok {
			operation = op
		}

		queryType := "unknown"
		if qt, ok := ctx.Value("query_type").(string); ok {
			queryType = qt
		}

		RecordDBQuery(t.host, operation, queryType, duration, success)
	}
}

func getQueryType(sql string) string {
	sql = strings.ToUpper(strings.TrimSpace(sql))

	switch {
	case strings.HasPrefix(sql, "SELECT"):
		return "SELECT"
	case strings.HasPrefix(sql, "INSERT"):
		return "INSERT"
	case strings.HasPrefix(sql, "UPDATE"):
		return "UPDATE"
	case strings.HasPrefix(sql, "DELETE"):
		return "DELETE"
	default:
		return "OTHER"
	}
}

func RecordEventValidation(result, reason string, count int) {
	eventsValidation.WithLabelValues(result, reason).Add(float64(count))
}

func RecordEventStored(count int) {
	eventsStored.Add(float64(count))
}

func SetDatabaseSize(database, host string, sizeBytes int64) {
	dbSize.WithLabelValues(database, host).Set(float64(sizeBytes))
}

func SetDatabaseEventsCount(database, host string, count uint64) {
	dbEventsCount.WithLabelValues(database, host).Set(float64(count))
}

func SetStorageSize(sizeBytes int64) {
	storageSize.Set(float64(sizeBytes))
}

func RecordStorageRequest(operation string, success bool) {
	successStr := "true"
	if !success {
		successStr = "false"
	}
	storageRequests.WithLabelValues(operation, successStr).Inc()
}

func RegisterPoolFromConnector(pool *pgxpool.Pool, dbName, host string) {
	globalPrometheusCollector.RegisterPgxPool(pool, dbName, host)
}
