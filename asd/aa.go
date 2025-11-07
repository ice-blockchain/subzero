// SPDX-License-Identifier: ice License 1.0

package main

import (
	"context"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/subzero/log"
	opentelemetry "github.com/ice-blockchain/subzero/open-telemetry"
)

func main() {
	opentelemetry.MustInit(context.Background())
	log.Trace(context.Background(), "opentelemetry")
	log.Debug(context.Background(), "opentelemetry")
	log.Info(context.Background(), "opentelemetry")
	log.Warn(context.Background(), "opentelemetry")
	log.Error(context.Background(), errors.New("opentelemetry"))
	log.Fatal(context.Background(), errors.New("opentelemetry"))
	opentelemetry.MustShutdown(context.Background())

}
