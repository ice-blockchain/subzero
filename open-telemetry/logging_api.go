// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	otellog "go.opentelemetry.io/otel/log"
)

type (
	Logger struct {
		otelLogger           otellog.Logger
		redundantLogExporter *redundantLogExporter
	}
)

var (
	reUser        = regexp.MustCompile("user=[^\\s`]+")
	reDatabase    = regexp.MustCompile("database=[^\\s`]+")
	reDbname      = regexp.MustCompile("dbname=[^\\s`]+")
	rePassword    = regexp.MustCompile("password=[^\\s`]+")
	reURIUserInfo = regexp.MustCompile(`://[^/]+@`) // postgres://user:pass@host
)

var globalLogger = globalTelemetry.NewLogger("subzero")

const defaultExportTimeout = 60 * time.Second

func DefaultLogger() *Logger { return globalLogger }

func NewLogger(name string) *Logger {
	return globalTelemetry.NewLogger(name)
}

func (t *telemetry) NewLogger(name string) *Logger {
	return &Logger{
		otelLogger:           t.logProvider.Logger(name),
		redundantLogExporter: t.redundantLogExporter,
	}
}

func (l *Logger) Trace(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityTrace, msg, keysAndValues...)
}

func (l *Logger) Debug(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityDebug, msg, keysAndValues...)
}

func (l *Logger) Info(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityInfo, msg, keysAndValues...)
}

func (l *Logger) Warn(ctx context.Context, msg string, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityWarn, msg, keysAndValues...)
}

func (l *Logger) Error(ctx context.Context, err error, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityError, fmt.Sprintf("%v", err), keysAndValues...)
}

func (l *Logger) Fatal(ctx context.Context, err error, keysAndValues ...any) {
	l.log(ctx, otellog.SeverityFatal, strings.ReplaceAll(fmt.Sprintf("%+v", err), "\n", " "), keysAndValues...)
}

func (l *Logger) Panic(ctx context.Context, err error, keysAndValues ...any) {
	l.Fatal(ctx, err, keysAndValues...)
	panic(err)
}

func (l *Logger) log(ctx context.Context, severity otellog.Severity, msg string, keysAndValues ...any) {
	if len(keysAndValues)%2 != 0 {
		panic("use pairs: fieldName1, fieldValue1, fieldName2, fieldValue2, ...")
	}
	if !l.otelLogger.Enabled(ctx, otellog.EnabledParameters{Severity: severity}) {
		return
	}

	l.drainWALLogRecords(ctx)

	var record otellog.Record
	record.SetTimestamp(time.Now())
	record.SetBody(otellog.StringValue(sanitize(msg)))
	record.SetSeverity(severity)
	record.SetSeverityText(severity.String())
	uniq, _ := uuid.NewV7()
	record.AddAttributes(otellog.KeyValue{logRecordID, otellog.StringValue(uniq.String())})
	for i := 0; i < len(keysAndValues)-1; i += 2 {
		key := keysAndValues[i].(string)
		value := keysAndValues[i+1]
		record.AddAttributes(extractKeyValue(key, value))
	}
	go l.otelLogger.Emit(ctx, record)
}

func (l *Logger) drainWALLogRecords(ctx context.Context) {
	if l.redundantLogExporter == nil ||
		!l.redundantLogExporter.primaryExporterEnabled ||
		l.redundantLogExporter.logWALBackupEmpty {
		return
	}
	l.redundantLogExporter.primaryLifecycleMx.Lock()
	defer l.redundantLogExporter.primaryLifecycleMx.Unlock()

	firstIndex, err := l.redundantLogExporter.logWALBackup.FirstIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to fetch first log wal backup index"))
		return
	}
	lastIndex, err := l.redundantLogExporter.logWALBackup.LastIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to fetch last log wal backup index"))
		return
	}
	if firstIndex == 0 && lastIndex == 0 {
		return
	}
	if lastIndex > firstIndex+100 {
		lastIndex = firstIndex + 100
	}

	for i := firstIndex; i <= lastIndex; i++ {
		bytes, rErr := l.redundantLogExporter.logWALBackup.Read(i)
		if rErr != nil {
			globalLogger.Error(ctx, errors.Wrapf(rErr, "failed to read log wal backup at index %v", i))
			return
		}
		var logRecord walLogRecord
		if rErr = json.Unmarshal(bytes, &logRecord); rErr != nil {
			globalLogger.Error(ctx, errors.Wrapf(rErr, "failed to Unmarshal logline %v into %T", string(bytes), logRecord))
			return
		}
		var record otellog.Record
		record.SetTimestamp(logRecord.Timestamp)
		record.SetBody(otellog.StringValue(logRecord.Body))
		record.SetSeverity(logRecord.Severity)
		record.SetSeverityText(logRecord.SeverityText)
		record.AddAttributes(otellog.KeyValue{logRecordID, otellog.StringValue(logRecord.LogRecordID)})
		for key, value := range logRecord.Attributes {
			//TODO value is deserialized; so its not exactly like the original one, so it needs adaption
			record.AddAttributes(extractKeyValue(key, value))
		}

		go l.otelLogger.Emit(ctx, record)
	}

	if err = l.redundantLogExporter.logWALBackup.TruncateFront(lastIndex); err != nil {
		globalLogger.Error(ctx, errors.Wrapf(err, "failed to logWALBackup.TruncateFront index %v ", lastIndex+1))
		return
	}
	firstIndex, err = l.redundantLogExporter.logWALBackup.FirstIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to get logWALBackup.FirstIndex"))
		return
	}
	if firstIndex == 0 || firstIndex == lastIndex {
		l.redundantLogExporter.logWALBackupEmpty = true
	}
}

func extractKeyValue(key string, val any) otellog.KeyValue {
	switch v := val.(type) {
	case *string:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.String(key, fmt.Sprintf("%v", *v))
	case string:
		return otellog.String(key, fmt.Sprintf("%v", v))
	case *int:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int(key, *v)
	case *int64:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, *v)
	case *int32:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case *int16:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case *int8:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case *uint:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		if *v > math.MaxInt64 {
			return otellog.String(key, fmt.Sprintf("%v", *v))
		} else {
			return otellog.Int64(key, int64(*v))
		}
	case *uint64:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		if *v > math.MaxInt64 {
			return otellog.String(key, fmt.Sprintf("%v", *v))
		} else {
			return otellog.Int64(key, int64(*v))
		}
	case *uint32:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case *uint16:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case *uint8:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Int64(key, int64(*v))
	case int:
		return otellog.Int(key, v)
	case int64:
		return otellog.Int64(key, v)
	case int32:
		return otellog.Int64(key, int64(v))
	case int16:
		return otellog.Int64(key, int64(v))
	case int8:
		return otellog.Int64(key, int64(v))
	case uint:
		if v > math.MaxInt64 {
			return otellog.String(key, fmt.Sprintf("%v", v))
		} else {
			return otellog.Int64(key, int64(v))
		}
	case uint64:
		if v > math.MaxInt64 {
			return otellog.String(key, fmt.Sprintf("%v", v))
		} else {
			return otellog.Int64(key, int64(v))
		}
	case uint32:
		return otellog.Int64(key, int64(v))
	case uint16:
		return otellog.Int64(key, int64(v))
	case uint8:
		return otellog.Int64(key, int64(v))
	case *bool:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Bool(key, *v)
	case bool:
		return otellog.Bool(key, v)
	case float32:
		return otellog.Float64(key, float64(v))
	case *float32:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Float64(key, float64(*v))
	case float64:
		return otellog.Float64(key, v)
	case *float64:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Float64(key, *v)
	case *[]byte:
		if v == nil {
			return otellog.String(key, "<nil>")
		}
		return otellog.Bytes(key, *v)
	case []byte:
		return otellog.Bytes(key, v)
	case error:
		return otellog.String(key, strings.ReplaceAll(fmt.Sprintf("%+v", v), "\n", " "))
	default:
		t := reflect.TypeOf(val)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() == reflect.Map {
			vv := reflect.ValueOf(val)
			entries := make([]otellog.KeyValue, vv.Len())
			iter := vv.MapRange()
			for iter.Next() {
				entries = append(entries, extractKeyValue(fmt.Sprintf("%+v", iter.Key().Interface()), iter.Value().Interface()))
			}

			return otellog.Map(key, entries...)
		}
		if t.Kind() == reflect.Slice {
			vv := reflect.ValueOf(val)
			entries := make([]otellog.Value, vv.Len())
			for i := 0; i < vv.Len(); i++ {
				entries = append(entries, extractKeyValue("_", vv.Index(i).Interface()).Value)
			}

			return otellog.Slice(key, entries...)
		}

		return otellog.String(key, fmt.Sprintf("%+v", v))
	}
}

func sanitize(s string) string {
	if s == "" {
		return s
	}
	s = reUser.ReplaceAllString(s, "user=***")
	s = reDatabase.ReplaceAllString(s, "database=***")
	s = reDbname.ReplaceAllString(s, "dbname=***")
	s = rePassword.ReplaceAllString(s, "password=***")
	s = reURIUserInfo.ReplaceAllString(s, "://***@")

	return s
}
