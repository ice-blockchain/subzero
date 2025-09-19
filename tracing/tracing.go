// SPDX-License-Identifier: ice License 1.0

// This package's API will be extended as needed
// DO NOT expose to the caller anything from open-telemetry
package tracing

import (
	"context"

	opentelemetry "github.com/ice-blockchain/subzero/open-telemetry"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func Start(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return opentelemetry.DefaultTracer().Start(ctx, spanName, opts...)
}
