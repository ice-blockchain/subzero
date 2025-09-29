// SPDX-License-Identifier: ice License 1.0

package log

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	opentelemetry "github.com/ice-blockchain/subzero/open-telemetry"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/ice-blockchain/subzero/cfg"
)

var (
	globalInitializer sync.Once
)

type (
	Config struct {
		Level string `yaml:"level" validate:"omitempty,oneof=trace debug info warn error fatal panic"`
	}
	Option func(*Config)
)

func MustInit(opts ...Option) {
	config, err := cfg.Get[Config]()
	if config == nil || err != nil {
		fmt.Println("no log configuration found, using default values", err)
		config = &Config{}
	}

	for _, opt := range opts {
		opt(config)
	}

	if config.Level == "" {
		config.Level = "info"
	}

	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		panic(fmt.Sprintf("invalid log level: %v", err))
	}
	zerolog.SetGlobalLevel(level)

	globalInitializer.Do(func() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:          os.Stdout,
			TimeFormat:   time.RFC3339Nano,
			TimeLocation: time.UTC,
			NoColor:      true,
		})
	})
}

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

func Panic(ctx context.Context, err error, keysAndValues ...any) {
	opentelemetry.DefaultLogger().Panic(ctx, err, keysAndValues...)
}
