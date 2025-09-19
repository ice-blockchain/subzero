// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/goccy/go-json"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type (
	Tracer struct {
		otelTracer             oteltrace.Tracer
		redundantTraceExporter *redundantTraceExporter
	}
)

var globalTracer = new(Tracer)

func DefaultTracer() *Tracer { return globalTracer }

func NewTracer(name string) *Tracer {
	return globalTelemetry.NewTracer(name)
}

func (t *telemetry) NewTracer(name string) *Tracer {
	return &Tracer{
		otelTracer:             t.traceProvider.Tracer(name),
		redundantTraceExporter: t.redundantTraceExporter,
	}
}

// TODO see how to wrap and hide oteltrace.SpanStartOption & oteltrace.Span from the caller
func (t *Tracer) Start(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	t.drainWALTraceSpans(ctx)

	//TODO add sanitize logic for hiding sensitive data that can be added in oteltrace.Span by the caller; see where to best add this logic

	//TODO see if we can add tracing support for postgresql; I.E. with something like otelsql

	return t.otelTracer.Start(ctx, spanName, opts...)
}

func (t *Tracer) drainWALTraceSpans(ctx context.Context) {
	if t.redundantTraceExporter == nil ||
		!t.redundantTraceExporter.primaryExporterEnabled ||
		t.redundantTraceExporter.traceWALBackupEmpty {
		return
	}
	t.redundantTraceExporter.primaryLifecycleMx.Lock()
	defer t.redundantTraceExporter.primaryLifecycleMx.Unlock()

	firstIndex, err := t.redundantTraceExporter.traceWALBackup.FirstIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to fetch first trace wal backup index"))
		return
	}
	lastIndex, err := t.redundantTraceExporter.traceWALBackup.LastIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to fetch last trace wal backup index"))
		return
	}
	if firstIndex == 0 || lastIndex == 0 {
		return
	}
	if lastIndex > 100 {
		lastIndex = 100
	}

	for i := firstIndex; i <= lastIndex; i++ {
		bytes, rErr := t.redundantTraceExporter.traceWALBackup.Read(i)
		if rErr != nil {
			globalLogger.Error(ctx, errors.Wrapf(rErr, "failed to read trace wal backup at index %v", i))
			return
		}
		var traceSpan walTraceSpan
		if rErr = json.Unmarshal(bytes, &traceSpan); rErr != nil {
			globalLogger.Error(ctx, errors.Wrapf(rErr, "failed to Unmarshal traceline %v into %T", string(bytes), traceSpan))
			return
		}
		//TODO figure out how to re-send the spans to the exporter
	}

	if err = t.redundantTraceExporter.traceWALBackup.TruncateFront(lastIndex + 1); err != nil {
		globalLogger.Error(ctx, errors.Wrapf(err, "failed to traceWALBackup.TruncateFront index %v ", lastIndex+1))
		return
	}
	firstIndex, err = t.redundantTraceExporter.traceWALBackup.FirstIndex()
	if err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to get traceWALBackup.FirstIndex"))
		return
	}
	if firstIndex == 0 {
		t.redundantTraceExporter.traceWALBackupEmpty = true
	}
}
