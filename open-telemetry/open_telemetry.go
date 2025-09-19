// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"fmt"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/tidwall/wal"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/ice-blockchain/subzero/cfg"
)

var (
	globalTelemetry = &telemetry{
		redundantLogExporter: &redundantLogExporter{
			primaryLifecycleMx: &sync.RWMutex{},
		},
		redundantTraceExporter: &redundantTraceExporter{
			primaryLifecycleMx: &sync.RWMutex{},
		},
	}
)

type (
	config struct {
		LogLevel       string   `yaml:"log-level"`
		Version        string   `yaml:"version"`
		RelayURL       string   `yaml:"relay-url"`
		ExporterURLs   []string `yaml:"exporter-urls"`
		TracingEnabled bool     `yaml:"tracing-enabled"`
	}
	telemetry struct {
		cfg *config
		//Logging

		logProvider          *otelsdklog.LoggerProvider
		redundantLogExporter *redundantLogExporter

		//Metrics

		//Tracing
		traceProvider          *otelsdktrace.TracerProvider
		redundantTraceExporter *redundantTraceExporter
	}
)

func MustInit(ctx context.Context) {
	globalTelemetry.cfg = cfg.MustGet[config]()
	res, err := otelsdkresource.New(ctx,
		otelsdkresource.WithAttributes(
			otelsemconv.ServiceName("subzero"),
			otelsemconv.ServiceInstanceID(globalTelemetry.cfg.RelayURL),
			otelsemconv.ServiceVersion(globalTelemetry.cfg.Version),
		),
	)
	if err != nil {
		globalLogger.Panic(ctx, errors.Wrap(err, "failed to init resource"))
	}
	var exporterConns []*grpc.ClientConn
	if len(globalTelemetry.cfg.ExporterURLs) > 0 {
		globalTelemetry.redundantLogExporter.logWALBackup, err = wal.Open("subzero-tmp-bkp-logfile", &wal.Options{SegmentCacheSize: 10, NoCopy: true})
		if err != nil {
			globalLogger.Panic(ctx, errors.Wrap(err, "failed to init logWALBackup"))
		}
		globalTelemetry.redundantTraceExporter.traceWALBackup, err = wal.Open("subzero-tmp-bkp-tracefile", &wal.Options{SegmentCacheSize: 10, NoCopy: true})
		if err != nil {
			globalLogger.Panic(ctx, errors.Wrap(err, "failed to init traceWALBackup"))
		}
		exporterConns = make([]*grpc.ClientConn, len(globalTelemetry.cfg.ExporterURLs))
		for _, exporterUrl := range globalTelemetry.cfg.ExporterURLs {
			exporterConn, grpcErr := grpc.NewClient(exporterUrl,
				// TODO setup proper tls
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				// TODO setup optimal grpc dial opts
			)
			if grpcErr != nil {
				globalLogger.Panic(ctx, errors.Wrap(grpcErr, "failed to init exporterConn"))
			}
			exporterConns = append(exporterConns, exporterConn)
		}
	}

	globalTelemetry.mustInitLogProvider(ctx, res, exporterConns)
	*globalLogger = *NewLogger("subzero")
	globalTelemetry.mustInitTracingProvider(ctx, res, exporterConns)
	*globalTracer = *NewTracer("subzero")
}

// TODO: see how u can not lose those error logs in this func
func MustShutdown(ctx context.Context) {
	if err := globalTelemetry.traceProvider.ForceFlush(ctx); err != nil {
		fmt.Printf("%v %+v \n", "failed to ForceFlush traceProvider", err)
	}
	if err := globalTelemetry.traceProvider.Shutdown(ctx); err != nil {
		fmt.Printf("%v %+v \n", "failed to shutdown traceProvider", err)
	}
	if err := globalTelemetry.logProvider.ForceFlush(ctx); err != nil {
		fmt.Printf("%v %+v \n", "failed to ForceFlush logProvider", err)
	}
	if err := globalTelemetry.logProvider.Shutdown(ctx); err != nil {
		fmt.Printf("%v %+v \n", "failed to shutdown logProvider", err)
	}
}
