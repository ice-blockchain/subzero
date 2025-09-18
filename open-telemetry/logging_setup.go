// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"fmt"
	"io"
	stdliblog "log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-logr/logr"
	"github.com/goccy/go-json"
	"github.com/jellydator/ttlcache/v3"
	zerolog "github.com/rs/zerolog/log"
	"github.com/tidwall/wal"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otelstdoutlog "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	otellog "go.opentelemetry.io/otel/log"
	globalotellog "go.opentelemetry.io/otel/log/global"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
)

const logRecordID = "logRecordID"

func LogWriter() io.Writer {
	var verbosity int
	switch strings.ToLower(globalTelemetry.cfg.LogLevel) {
	case "trace":
		verbosity = 12
	case "debug":
		verbosity = 8
	case "info":
		verbosity = 4
	case "warn", "warning":
		verbosity = 1
	}
	return &otelLogWriter{level: verbosity}
}

func (t *telemetry) mustInitLogProvider(ctx context.Context, res *otelsdkresource.Resource, exporterConns []*grpc.ClientConn) {
	var otlpRemoteExporters []otelsdklog.Exporter
	if len(exporterConns) > 0 {
		otlpRemoteExporters = make([]otelsdklog.Exporter, 0, len(exporterConns))
		for ix, exporterConn := range exporterConns {
			otlpRemoteExporter, err := otlploggrpc.New(ctx,
				otlploggrpc.WithCompressor(gzip.Name),
				otlploggrpc.WithRetry(otlploggrpc.RetryConfig{
					Enabled:         true,
					InitialInterval: 500 * time.Millisecond,
					MaxInterval:     5 * time.Second,
					MaxElapsedTime:  defaultExportTimeout,
				}),
				otlploggrpc.WithGRPCConn(exporterConn),
				otlploggrpc.WithHeaders(map[string]string{
					"service":       fmt.Sprintf("subzero/%v %v", t.cfg.Version, t.cfg.RelayURL),
					"Authorization": fmt.Sprintf("Basic %v", t.cfg.AuthToken),
					"stream-name":   "subzero",
					"organization":  "default", // TODO: cfg?
				}),
				otlploggrpc.WithTimeout(defaultExportTimeout),
			)
			if err != nil {
				globalLogger.Panic(ctx, errors.Wrapf(err, "failed to create OTLP remote log exporter %v", ix))
			}
			otlpRemoteExporters = append(otlpRemoteExporters, otlpRemoteExporter)
		}
	}
	var stdoutExporter otelsdklog.Exporter
	var err error
	stdoutExporter, err = otelstdoutlog.New(otelstdoutlog.WithWriter(os.Stdout))
	if err != nil {
		globalLogger.Panic(ctx, errors.Wrap(err, "failed to create stdout log exporter"))
	}
	options := []otelsdklog.BatchProcessorOption{
		otelsdklog.WithExportInterval(10 * time.Second),
		otelsdklog.WithExportTimeout(defaultExportTimeout),
		otelsdklog.WithExportBufferSize(1000),
	}
	t.redundantLogExporter.primaries = otlpRemoteExporters
	if globalTelemetry.cfg.Debug {
		options = append(options,
			otelsdklog.WithExportBufferSize(1),
			otelsdklog.WithExportMaxBatchSize(1),
			otelsdklog.WithExportTimeout(100*time.Millisecond),
		)
		stdoutExporter = newNonStructuredExporter(os.Stdout)
		t.redundantLogExporter.primaries = []otelsdklog.Exporter{stdoutExporter}
	}
	t.redundantLogExporter.fallback = stdoutExporter
	t.logProvider = otelsdklog.NewLoggerProvider(
		otelsdklog.WithProcessor(&batchProcessorWrapper{
			BatchProcessor: otelsdklog.NewBatchProcessor(t.redundantLogExporter,
				options...,
			),
			severity: detectLogSeverity(t.cfg.LogLevel),
		}),
		otelsdklog.WithResource(res),
	)
	var verbosity int
	switch strings.ToLower(t.cfg.LogLevel) {
	case "trace":
		verbosity = 12
	case "debug":
		verbosity = 8
	case "info":
		verbosity = 4
	case "warn", "warning":
		verbosity = 1
	}

	ow := &otelLogWriter{level: verbosity}
	otel.SetLogger(logr.New(ow).V(verbosity))
	globalotellog.SetLoggerProvider(t.logProvider)
	otel.SetErrorHandler(&errHandler{})

	slogHandler := otelslog.NewHandler("stdlib",
		otelslog.WithLoggerProvider(t.logProvider),
		otelslog.WithSource(true),
		otelslog.WithAttributes(otelattribute.String("relay", t.cfg.RelayURL)),
		otelslog.WithAttributes(otelattribute.String("service", "subzero")),
		otelslog.WithVersion(t.cfg.Version),
	)
	slog.SetDefault(slog.New(slogHandler))
	stdliblog.SetOutput(ow)
	zerolog.Logger = zerolog.Output(ow)

}

func detectLogSeverity(logLevel string) otellog.Severity {
	switch strings.ToLower(logLevel) {
	case "trace":
		return otellog.SeverityTrace
	case "debug":
		return otellog.SeverityDebug
	case "info":
		return otellog.SeverityInfo
	case "warn", "warning":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	default:
		return otellog.SeverityFatal
	}
}

type otelLogWriter struct {
	level int
}

func (ow *otelLogWriter) Init(_ logr.RuntimeInfo) {}

func (ow *otelLogWriter) Enabled(level int) bool {
	return level >= ow.level
}

func (ow *otelLogWriter) Info(level int, msg string, keysAndValues ...any) {
	if level >= 12 {
		globalLogger.Trace(context.Background(), msg, keysAndValues...)
	} else if level >= 8 {
		globalLogger.Debug(context.Background(), msg, keysAndValues...)
	} else if level >= 4 {
		globalLogger.Info(context.Background(), msg, keysAndValues...)
	} else if level >= 1 {
		globalLogger.Warn(context.Background(), msg, keysAndValues...)
	}
}

func (ow *otelLogWriter) Error(err error, msg string, keysAndValues ...any) {
	globalLogger.Error(context.Background(), errors.Wrap(err, msg), keysAndValues...)
}

func (ow *otelLogWriter) WithValues(_ ...any) logr.LogSink {
	return ow
}

func (ow *otelLogWriter) WithName(_ string) logr.LogSink {
	return ow
}

func (ow *otelLogWriter) Write(p []byte) (int, error) {
	var logLevel, body string
	if len(p) > 1 && p[0] == '{' && (p[len(p)-1] == '}' || p[len(p)-2] == '}') { // zerolog
		type zeroLogLogLevel struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		var l zeroLogLogLevel
		if err := json.Unmarshal(p, &l); err != nil {
			return 0, errors.Wrapf(err, "malformed")
		}
		logLevel = strings.ToLower(l.Level)
		body = l.Message
	} else {
		body = string(p)
		if idx := strings.Index(body, " DBG "); idx >= 0 {
			logLevel = "debug"
			body = body[idx+5:]
		} else if idx = strings.Index(body, " TRC "); idx >= 0 {
			logLevel = "trace"
			body = body[idx+5:]
		} else if idx = strings.Index(body, " WRN "); idx >= 0 {
			logLevel = "warn"
			body = body[idx+5:]
		} else if idx = strings.Index(body, " ERR "); idx >= 0 {
			logLevel = "error"
			body = body[idx+5:]
		} else if idx = strings.Index(body, " PNC "); idx >= 0 {
			logLevel = "panic"
			body = body[idx+5:]
		}
	}
	if body == "" {
		body = string(p)
	}
	switch logLevel {
	case "trace", "trc":
		globalLogger.Trace(context.Background(), body)
	case "debug", "dbg":
		globalLogger.Debug(context.Background(), body)
	case "info", "inf":
		globalLogger.Info(context.Background(), body)
	case "warn", "wrn":
		globalLogger.Warn(context.Background(), body)
	case "error", "err":
		globalLogger.Error(context.Background(), errors.New(body))
	case "panic", "pnc", "fatal":
		globalLogger.Panic(context.Background(), errors.New(body))
	default:
		globalLogger.Info(context.Background(), body)
	}
	return len(p), nil
}

type errHandler struct{}

func (eh *errHandler) Handle(err error) {
	globalLogger.Error(context.Background(), err, "source", "opentelemetry-error-handler")
}

type batchProcessorWrapper struct {
	*otelsdklog.BatchProcessor

	severity otellog.Severity
}

func (bp *batchProcessorWrapper) Enabled(_ context.Context, param otelsdklog.EnabledParameters) bool {
	return param.Severity >= bp.severity
}

type redundantLogExporter struct {
	fallback           otelsdklog.Exporter
	primaryLifecycleMx *sync.RWMutex

	logWALBackup        *wal.Log
	primaries           []otelsdklog.Exporter
	currentPrimaryIndex uint64

	primaryExporterEnabled bool
	logWALBackupEmpty      bool
	closing                atomic.Bool
	lastExportedLogRecords *ttlcache.Cache[string, struct{}]
}

type nonStructuredExporter struct {
	writer    io.Writer
	formatLog func(r otelsdklog.Record) string
}

func (re *redundantLogExporter) Export(ctx context.Context, records []otelsdklog.Record) error {
	var ids []string
	records, ids = re.deduplRecords(records)
	if len(records) == 0 {
		return nil
	}
	defer func() {
		for i := range records {
			re.lastExportedLogRecords.Set(ids[i], struct{}{}, 30*time.Second)
		}
	}()
	if len(re.primaries) == 0 || (!re.primaryExporterEnabled && re.closing.Load()) {
		return errors.Join(
			re.writeLogRecordsToWALBackup(records, ids),
			errors.Wrap(re.fallback.Export(ctx, records), "fallback.Export"),
		)
	}
	nextIndex := atomic.AddUint64(&re.currentPrimaryIndex, 1) % uint64(len(re.primaries))
	if err := re.primaries[nextIndex].Export(ctx, records); err != nil {
		var succeeded bool
		for ix, primary := range re.primaries {
			if uint64(ix) == nextIndex {
				continue
			}
			exportCtx, cancel := context.WithTimeout(context.Background(), defaultExportTimeout)
			if aggErr := primary.Export(exportCtx, records); aggErr != nil {
				cancel()
				err = errors.Join(err, errors.Wrapf(aggErr, "primary[%v].Export", ix))
			} else {
				succeeded = true
				cancel()
				break
			}
		}
		if succeeded {
			if !re.primaryExporterEnabled {
				re.primaryLifecycleMx.Lock()
				re.primaryExporterEnabled = true
				re.primaryLifecycleMx.Unlock()
			}

			return nil
		}
		globalLogger.Error(ctx, errors.Wrap(err, "primary.Export"), "records", len(records))

		if re.primaryExporterEnabled {
			re.primaryLifecycleMx.Lock()
			re.primaryExporterEnabled = false
			re.primaryLifecycleMx.Unlock()
		}
		exportCtx, cancel := context.WithTimeout(context.Background(), defaultExportTimeout)
		defer cancel()
		return errors.Join(
			errors.Wrap(re.fallback.Export(exportCtx, records), "fallback.Export"),
			re.writeLogRecordsToWALBackup(records, ids),
		)
	}
	if !re.primaryExporterEnabled {
		re.primaryLifecycleMx.Lock()
		re.primaryExporterEnabled = true
		re.primaryLifecycleMx.Unlock()
	}

	return nil
}

func (re *redundantLogExporter) deduplRecords(records []otelsdklog.Record) ([]otelsdklog.Record, []string) {
	filtered := make([]otelsdklog.Record, 0, len(records))
	uniques := make([]string, 0, len(records))
	for _, record := range records {
		if record.AttributesLen() == 0 {
			continue
		}
		var uniq string
		filteredAttrs := make([]otellog.KeyValue, 0, record.AttributesLen()-1)
		record.WalkAttributes(func(kv otellog.KeyValue) bool {
			if kv.Key == logRecordID {
				uniq = kv.Value.String()
				return true
			}
			filteredAttrs = append(filteredAttrs, kv)
			return true
		})
		record.SetAttributes(filteredAttrs...)
		if !re.lastExportedLogRecords.Has(uniq) {
			filtered = append(filtered, record)
			uniques = append(uniques, uniq)
		}
	}
	return filtered, uniques
}

func (re *redundantLogExporter) writeLogRecordsToWALBackup(records []otelsdklog.Record, ids []string) (err error) {
	if re.logWALBackup == nil {
		return nil
	}
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()
	batch := new(wal.Batch)
	lastIndex, err := re.logWALBackup.LastIndex()
	if err != nil {
		return errors.Wrap(err, "failed to get last logWALBackup index")
	}
	for ix, record := range records {
		bytes, sErr := (&walLogRecord{actualLogRecord: record, LogRecordID: ids[ix]}).MarshallJSON()
		if sErr != nil {
			err = errors.Join(err, sErr)
		}
		batch.Write(uint64(ix)+1+lastIndex, bytes)
	}

	err = errors.Join(
		err,
		errors.Wrap(re.logWALBackup.WriteBatch(batch), "failed to write to logWALBackup"),
	)
	if err == nil && re.logWALBackupEmpty {
		re.logWALBackupEmpty = false
	}

	return err
}

func (re *redundantLogExporter) Shutdown(ctx context.Context) error {
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()

	if len(re.primaries) == 0 {
		var err error

		err = errors.Join(err,
			errors.Wrap(re.fallback.Shutdown(ctx), "fallback.Shutdown"),
		)
	}

	var errs []error
	for ix, primary := range re.primaries {
		errs = append(errs, errors.Wrapf(primary.Shutdown(ctx), "primary[%v].Shutdown", ix))
	}

	err := errors.Wrap(errors.Join(errs...), "primary.Shutdown")

	return errors.Join(
		errors.Wrap(re.fallback.Shutdown(ctx), "fallback.Shutdown"),
		err,
	)
}

func (re *redundantLogExporter) ForceFlush(ctx context.Context) error {
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()
	if len(re.primaries) == 0 || !re.primaryExporterEnabled {
		var err error
		if re.logWALBackup != nil {
			err = errors.Wrap(re.logWALBackup.Sync(), "failed to logWALBackup sync")
		}

		return errors.Join(err, errors.Wrap(re.fallback.ForceFlush(ctx), "fallback.ForceFlush"))
	}
	var errs []error
	for ix, primary := range re.primaries {
		errs = append(errs, errors.Wrapf(primary.ForceFlush(ctx), "primary[%v].ForceFlush", ix))
	}
	err := errors.Wrap(errors.Join(errs...), "primary.ForceFlush")
	if re.logWALBackup != nil {
		err = errors.Join(err, errors.Wrap(re.logWALBackup.Sync(), "failed to logWALBackup sync"))
	}

	return errors.Join(
		errors.Wrap(re.fallback.ForceFlush(ctx), "fallback.ForceFlush"),
		err,
	)
}

type walLogRecord struct {
	Timestamp time.Time `json:"timestamp"`

	Attributes   map[string]any `json:"attributes"`
	SeverityText string         `json:"severityText"`
	Body         string         `json:"body"`

	actualLogRecord otelsdklog.Record

	Severity    otellog.Severity `json:"severity"`
	LogRecordID string           `json:"logRecordID"`
}

func (re *walLogRecord) MarshallJSON() ([]byte, error) {
	re.Timestamp = re.actualLogRecord.Timestamp()
	re.Severity = re.actualLogRecord.Severity()
	re.SeverityText = re.actualLogRecord.SeverityText()
	re.Body = re.actualLogRecord.Body().AsString()
	re.Attributes = make(map[string]any, re.actualLogRecord.AttributesLen())
	re.actualLogRecord.WalkAttributes(func(kv otellog.KeyValue) bool {
		re.Attributes[kv.Key] = kv.Value

		return true
	})

	bytes, err := json.Marshal(re)

	return bytes, errors.Wrapf(err, "failed to marshal walLogRecord %v", re)
}

func newNonStructuredExporter(writer io.Writer) otelsdklog.Exporter {
	exporter := &nonStructuredExporter{writer: writer}
	exporter.formatLog = func(r otelsdklog.Record) string {
		line := r.Timestamp().Format(time.RFC3339Nano) + " " + r.SeverityText() + " " + r.Body().AsString()
		r.WalkAttributes(func(kv otellog.KeyValue) bool {
			line += fmt.Sprintf(" %v=%q", kv.Key, kv.Value)
			return true
		})
		return line
	}

	return exporter
}

func (n *nonStructuredExporter) Export(ctx context.Context, records []otelsdklog.Record) (err error) {
	for _, r := range records {
		logLine := n.formatLog(r)
		_, wErr := n.writer.Write([]byte(logLine + "\n"))
		err = errors.Join(err, errors.Wrapf(wErr, "failed to write log line: %v", logLine))
	}
	return err
}

func (n *nonStructuredExporter) Shutdown(ctx context.Context) error {
	return nil
}

func (n *nonStructuredExporter) ForceFlush(ctx context.Context) error {
	return nil
}
