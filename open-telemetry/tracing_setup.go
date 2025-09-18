// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	"github.com/hashicorp/go-multierror"
	"github.com/tidwall/wal"
	"go.opentelemetry.io/otel"
	otelattribute "go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelstdouttrace "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otelpropagation "go.opentelemetry.io/otel/propagation"
	otelsdkinstrumentation "go.opentelemetry.io/otel/sdk/instrumentation"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

func (t *telemetry) mustInitTracingProvider(ctx context.Context, res *otelsdkresource.Resource, exporterConns []*grpc.ClientConn) {
	if len(exporterConns) > 0 {
		t.redundantTraceExporter.primaries = make([]otelsdktrace.SpanExporter, 0, len(exporterConns))
		for ix, exporterConn := range exporterConns {
			otlpRemoteExporter, err := otlptracegrpc.New(ctx,
				otlptracegrpc.WithGRPCConn(exporterConn),
				otlptracegrpc.WithHeaders(map[string]string{"service": fmt.Sprintf("subzero/%v %v", t.cfg.Version, t.cfg.RelayURL)}),
				otlptracegrpc.WithTimeout(time.Minute),
				otlptracegrpc.WithCompressor("gzip"),
				otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
					Enabled:         true,
					InitialInterval: 100 * time.Millisecond,
					MaxInterval:     5 * time.Second,
					MaxElapsedTime:  time.Minute,
				}),
			)
			if err != nil {
				globalLogger.Panic(ctx, errors.Wrapf(err, "failed to create OTLP remote trace exporter %v", ix))
			}
			t.redundantTraceExporter.primaries = append(t.redundantTraceExporter.primaries, otlpRemoteExporter)
		}
	}
	var err error
	t.redundantTraceExporter.fallback, err = otelstdouttrace.New()
	if err != nil {
		globalLogger.Panic(ctx, errors.Wrap(err, "failed to create stdout trace exporter"))
	}
	//TODO see how this parent based thing works exactly
	//TODO see how to dynamically toggle it on/off in production for specific users
	//TODO see what other sampler impls we can use
	sampler := otelsdktrace.ParentBased(otelsdktrace.AlwaysSample())
	if !t.cfg.TracingEnabled {
		sampler = otelsdktrace.NeverSample()
	}
	t.traceProvider = otelsdktrace.NewTracerProvider(
		otelsdktrace.WithSampler(sampler),
		otelsdktrace.WithResource(res),
		otelsdktrace.WithBatcher(
			t.redundantTraceExporter,
			otelsdktrace.WithBlocking(),
			otelsdktrace.WithExportTimeout(time.Minute),
			otelsdktrace.WithBatchTimeout(10*time.Second),
		),
	)
	otel.SetTracerProvider(t.traceProvider)
	otel.SetTextMapPropagator(otelpropagation.NewCompositeTextMapPropagator(otelpropagation.TraceContext{}, otelpropagation.Baggage{}))
}

type redundantTraceExporter struct {
	fallback           otelsdktrace.SpanExporter
	primaryLifecycleMx *sync.RWMutex

	traceWALBackup      *wal.Log
	primaries           []otelsdktrace.SpanExporter
	currentPrimaryIndex uint64

	primaryExporterEnabled bool
	traceWALBackupEmpty    bool
	closing                atomic.Bool
}

func (re *redundantTraceExporter) ExportSpans(ctx context.Context, spans []otelsdktrace.ReadOnlySpan) error {
	if len(re.primaries) == 0 || (!re.primaryExporterEnabled && re.closing.Load()) {
		return multierror.Append(
			re.writeTraceSpansToWALBackup(spans),
			errors.Wrap(re.fallback.ExportSpans(ctx, spans), "fallback.ExportSpans"),
		).ErrorOrNil()
	}
	nextIndex := atomic.AddUint64(&re.currentPrimaryIndex, 1) % uint64(len(re.primaries))
	if err := re.primaries[nextIndex].ExportSpans(ctx, spans); err != nil {
		var succeeded bool
		for ix, primary := range re.primaries {
			if uint64(ix) == nextIndex {
				continue
			}
			if aggErr := primary.ExportSpans(ctx, spans); aggErr != nil {
				err = multierror.Append(err, errors.Wrapf(aggErr, "primary[%v].ExportSpans", ix))
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
		globalLogger.Error(ctx, errors.Wrap(err, "primary.ExportSpans"), "spans", len(spans))

		if re.primaryExporterEnabled {
			re.primaryLifecycleMx.Lock()
			re.primaryExporterEnabled = false
			re.primaryLifecycleMx.Unlock()
		}

		return multierror.Append(
			re.writeTraceSpansToWALBackup(spans),
			errors.Wrap(re.fallback.ExportSpans(ctx, spans), "fallback.ExportSpans"),
		).ErrorOrNil()
	}

	if !re.primaryExporterEnabled {
		re.primaryLifecycleMx.Lock()
		re.primaryExporterEnabled = true
		re.primaryLifecycleMx.Unlock()
	}

	return nil
}

func (re *redundantTraceExporter) writeTraceSpansToWALBackup(spans []otelsdktrace.ReadOnlySpan) (err error) {
	if re.traceWALBackup == nil {
		return nil
	}
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()
	batch := new(wal.Batch)
	lastIndex, err := re.traceWALBackup.LastIndex()
	if err != nil {
		return errors.Wrap(err, "failed to get last traceWALBackup index")
	}
	for ix, span := range spans {
		bytes, sErr := (&walTraceSpan{actualTraceSpan: span}).MarshallJSON()
		if sErr != nil {
			err = multierror.Append(err, sErr).ErrorOrNil()
		}
		batch.Write(uint64(ix)+1+lastIndex, bytes)
	}

	err = multierror.Append(
		err,
		errors.Wrap(re.traceWALBackup.WriteBatch(batch), "failed to write to traceWALBackup"),
	).ErrorOrNil()

	if err == nil && re.traceWALBackupEmpty {
		re.traceWALBackupEmpty = false
	}

	return err
}

func (re *redundantTraceExporter) Shutdown(ctx context.Context) error {
	re.primaryLifecycleMx.Lock()
	defer re.primaryLifecycleMx.Unlock()

	if len(re.primaries) == 0 {
		var err error
		if re.traceWALBackup != nil {
			err = errors.Wrap(re.traceWALBackup.Close(), "failed to traceWALBackup close")
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
	if re.traceWALBackup != nil {
		err = multierror.Append(err, errors.Wrap(re.traceWALBackup.Close(), "failed to traceWALBackup close")).ErrorOrNil()
	}

	return multierror.Append(
		errors.Wrap(re.fallback.Shutdown(ctx), "fallback.Shutdown"),
		err,
	).ErrorOrNil()
}

type walTraceSpan struct {
	InstrumentationScope otelsdkinstrumentation.Scope `json:"instrumentationScope,omitempty"`
	StartTime            time.Time                    `json:"startTime,omitempty"`
	EndTime              time.Time                    `json:"endTime,omitempty"`
	actualTraceSpan      otelsdktrace.ReadOnlySpan

	Resource *otelsdkresource.Resource `json:"resource,omitempty"`
	Status   otelsdktrace.Status       `json:"status,omitempty"`

	//TODO see if all those fields are serialised properly
	Name              string                   `json:"name,omitempty"`
	Attributes        []otelattribute.KeyValue `json:"attributes,omitempty"`
	Links             []otelsdktrace.Link      `json:"links,omitempty"`
	Events            []otelsdktrace.Event     `json:"events,omitempty"`
	SpanContext       oteltrace.SpanContext    `json:"spanContext,omitempty"`
	Parent            oteltrace.SpanContext    `json:"parent,omitempty"`
	SpanKind          oteltrace.SpanKind       `json:"spanKind,omitempty"`
	DroppedAttributes int                      `json:"droppedAttributes,omitempty"`
	DroppedLinks      int                      `json:"droppedLinks,omitempty"`
	DroppedEvents     int                      `json:"droppedEvents,omitempty"`
	ChildSpanCount    int                      `json:"childSpanCount,omitempty"`
}

func (re *walTraceSpan) MarshallJSON() ([]byte, error) {
	re.Name = re.actualTraceSpan.Name()
	re.SpanContext = re.actualTraceSpan.SpanContext()
	re.Parent = re.actualTraceSpan.Parent()
	re.SpanKind = re.actualTraceSpan.SpanKind()
	re.StartTime = re.actualTraceSpan.StartTime()
	re.EndTime = re.actualTraceSpan.EndTime()
	re.Links = re.actualTraceSpan.Links()
	re.Events = re.actualTraceSpan.Events()
	re.Status = re.actualTraceSpan.Status()
	re.InstrumentationScope = re.actualTraceSpan.InstrumentationScope()
	re.Resource = re.actualTraceSpan.Resource()
	re.DroppedAttributes = re.actualTraceSpan.DroppedAttributes()
	re.DroppedLinks = re.actualTraceSpan.DroppedLinks()
	re.DroppedEvents = re.actualTraceSpan.DroppedEvents()
	re.ChildSpanCount = re.actualTraceSpan.ChildSpanCount()
	re.Attributes = re.actualTraceSpan.Attributes()

	bytes, err := json.Marshal(re)

	return bytes, errors.Wrapf(err, "failed to marshal walTraceSpan %v", re)
}
