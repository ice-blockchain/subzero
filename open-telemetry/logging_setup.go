// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"fmt"
	stdliblog "log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-logr/logr"
	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
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
)

func (t *telemetry) mustInitLogProvider(ctx context.Context, res *otelsdkresource.Resource, exporterConns []*grpc.ClientConn) {
	var otlpRemoteExporters []otelsdklog.Exporter
	if len(exporterConns) > 0 {
		otlpRemoteExporters = make([]otelsdklog.Exporter, 0, len(exporterConns))
		for ix, exporterConn := range exporterConns {
			otlpRemoteExporter, err := otlploggrpc.New(ctx,
				//TODO: proper setup
				otlploggrpc.WithGRPCConn(exporterConn),
				otlploggrpc.WithHeaders(map[string]string{"service": fmt.Sprintf("subzero/%v %v", t.cfg.Version, t.cfg.RelayURL)}),
				otlploggrpc.WithTimeout(time.Minute),
			)
			if err != nil {
				globalLogger.Panic(ctx, errors.Wrapf(err, "failed to create OTLP remote log exporter %v", ix))
			}
			otlpRemoteExporters = append(otlpRemoteExporters, otlpRemoteExporter)
		}
	}
	stdoutExporter, err := otelstdoutlog.New()
	if err != nil {
		globalLogger.Panic(ctx, errors.Wrap(err, "failed to create stdout log exporter"))
	}
	t.redundantLogExporter.primaries = otlpRemoteExporters
	t.redundantLogExporter.fallback = stdoutExporter

	t.logProvider = otelsdklog.NewLoggerProvider(
		otelsdklog.WithProcessor(&batchProcessorWrapper{
			BatchProcessor: otelsdklog.NewBatchProcessor(t.redundantLogExporter,
				otelsdklog.WithExportBufferSize(10),
				otelsdklog.WithExportInterval(10*time.Second),
				otelsdklog.WithExportTimeout(time.Minute),
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
	//TODO parse the log line
	globalLogger.Info(context.Background(), string(p))
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
}

func (re *redundantLogExporter) Export(ctx context.Context, records []otelsdklog.Record) error {
	if len(re.primaries) == 0 {
		return multierror.Append(
			re.writeLogRecordsToWALBackup(records),
			errors.Wrap(re.fallback.Export(ctx, records), "fallback.Export"),
		).ErrorOrNil()
	}
	nextIndex := atomic.AddUint64(&re.currentPrimaryIndex, 1) % uint64(len(re.primaries))
	if err := re.primaries[nextIndex].Export(ctx, records); err != nil {
		var succeeded bool
		for ix, primary := range re.primaries {
			if uint64(ix) == nextIndex {
				continue
			}
			if aggErr := primary.Export(ctx, records); aggErr != nil {
				err = multierror.Append(err, errors.Wrapf(aggErr, "primary[%v].Export", ix))
			} else {
				succeeded = true
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

		return multierror.Append(
			re.writeLogRecordsToWALBackup(records),
			errors.Wrap(re.fallback.Export(ctx, records), "fallback.Export"),
		).ErrorOrNil()
	}

	if !re.primaryExporterEnabled {
		re.primaryLifecycleMx.Lock()
		re.primaryExporterEnabled = true
		re.primaryLifecycleMx.Unlock()
	}

	return nil
}

func (re *redundantLogExporter) writeLogRecordsToWALBackup(records []otelsdklog.Record) (err error) {
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
		bytes, sErr := (&walLogRecord{actualLogRecord: record}).MarshallJSON()
		if sErr != nil {
			err = multierror.Append(err, sErr).ErrorOrNil()
		}
		batch.Write(uint64(ix)+1+lastIndex, bytes)
	}

	err = multierror.Append(
		err,
		errors.Wrap(re.logWALBackup.WriteBatch(batch), "failed to write to logWALBackup"),
	).ErrorOrNil()

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
		if re.logWALBackup != nil {
			err = errors.Wrap(re.logWALBackup.Close(), "failed to logWALBackup close")
		}

		return multierror.Append(err,
			errors.Wrap(re.fallback.Shutdown(ctx), "fallback.Shutdown"),
		).ErrorOrNil()
	}

	var errs []error
	for ix, primary := range re.primaries {
		errs = append(errs, errors.Wrapf(primary.Shutdown(ctx), "primary[%v].Shutdown", ix))
	}

	err := errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "primary.Shutdown")
	if re.logWALBackup != nil {
		err = multierror.Append(err, errors.Wrap(re.logWALBackup.Close(), "failed to logWALBackup close")).ErrorOrNil()
	}

	return multierror.Append(
		errors.Wrap(re.fallback.Shutdown(ctx), "fallback.Shutdown"),
		err,
	).ErrorOrNil()
}

func (re *redundantLogExporter) ForceFlush(ctx context.Context) error {
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()

	if len(re.primaries) == 0 {
		var err error
		if re.logWALBackup != nil {
			err = errors.Wrap(re.logWALBackup.Sync(), "failed to logWALBackup sync")
		}

		return multierror.Append(err, errors.Wrap(re.fallback.ForceFlush(ctx), "fallback.ForceFlush")).ErrorOrNil()
	}
	var errs []error
	for ix, primary := range re.primaries {
		errs = append(errs, errors.Wrapf(primary.ForceFlush(ctx), "primary[%v].ForceFlush", ix))
	}
	err := errors.Wrap(multierror.Append(nil, errs...).ErrorOrNil(), "primary.ForceFlush")
	if re.logWALBackup != nil {
		err = multierror.Append(err, errors.Wrap(re.logWALBackup.Sync(), "failed to logWALBackup sync")).ErrorOrNil()
	}

	return multierror.Append(
		errors.Wrap(re.fallback.ForceFlush(ctx), "fallback.ForceFlush"),
		err,
	).ErrorOrNil()
}

type walLogRecord struct {
	Timestamp time.Time `json:"timestamp"`

	Attributes   map[string]any `json:"attributes"`
	SeverityText string         `json:"severityText"`
	Body         string         `json:"body"`

	actualLogRecord otelsdklog.Record

	Severity otellog.Severity `json:"severity"`
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
