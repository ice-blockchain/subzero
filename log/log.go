// SPDX-License-Identifier: ice License 1.0

package log

import (
	"context"
	"time"

	opentelemetry "github.com/ice-blockchain/subzero/open-telemetry"
)

func Trace(ctx context.Context, msg string, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Trace(ctx, msg, keysAndValues...)
}

func Debug(ctx context.Context, msg string, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Debug(ctx, msg, keysAndValues...)
}

func Info(ctx context.Context, msg string, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Info(ctx, msg, keysAndValues...)
}

func Warn(ctx context.Context, msg string, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Warn(ctx, msg, keysAndValues...)
}

func Error(ctx context.Context, err error, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Error(ctx, err, keysAndValues...)
}

func Fatal(ctx context.Context, err error, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Fatal(ctx, err, keysAndValues...)
}

func PanicCtx(ctx context.Context, err error, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Panic(ctx, err, keysAndValues...)
}
func Panic(err error, keysAndValues ...any) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opentelemetry.DefaultLogger().Panic(ctx, err, keysAndValues...)
}
