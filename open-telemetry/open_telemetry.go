// SPDX-License-Identifier: ice License 1.0

package opentelemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/jellydator/ttlcache/v3"
	"github.com/tidwall/wal"
	otelstdoutlog "go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	otelsdklog "go.opentelemetry.io/otel/sdk/log"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	otelsdktrace "go.opentelemetry.io/otel/sdk/trace"
	otelsemconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"

	"github.com/ice-blockchain/subzero/appcontext"
	"github.com/ice-blockchain/subzero/cfg"
)

var (
	reLogExporter = &redundantLogExporter{
		primaryLifecycleMx:     &sync.RWMutex{},
		fallback:               mustNewFallbackExporter(),
		logWALBackup:           mustNewLogWAL(),
		lastExportedLogRecords: ttlcache.New[string, struct{}](ttlcache.WithTTL[string, struct{}](30 * time.Second)),
	}
	globalTelemetry = &telemetry{
		redundantLogExporter: reLogExporter,
		redundantTraceExporter: &redundantTraceExporter{
			primaryLifecycleMx: &sync.RWMutex{},
		},
		logProvider: otelsdklog.NewLoggerProvider(
			otelsdklog.WithProcessor(&batchProcessorWrapper{
				BatchProcessor: otelsdklog.NewBatchProcessor(reLogExporter,
					otelsdklog.WithExportMaxBatchSize(1),
					otelsdklog.WithExportBufferSize(1),
					otelsdklog.WithExportInterval(10*time.Second),
					otelsdklog.WithExportTimeout(defaultExportTimeout),
				),
				severity: detectLogSeverity("info"),
			}),
		),
	}
)

type (
	config struct {
		LogLevel       string   `yaml:"log-level"`
		Version        string   `yaml:"version"`
		RelayURL       string   `yaml:"relay-url"`
		AuthToken      string   `yaml:"auth-token"`
		ExporterURLs   []string `yaml:"exporter-urls"`
		TracingEnabled bool     `yaml:"tracing-enabled"`
		Debug          bool     `yaml:"debug"`
		tls            *tls.Config
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
	Option func(*telemetry)
)

func MustInit(ctx context.Context, opts ...Option) {
	globalTelemetry.redundantLogExporter.closing.Store(false)
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
	globalTelemetry.redundantTraceExporter.traceWALBackup, err = wal.Open("subzero-tmp-bkp-tracefile", &wal.Options{SegmentCacheSize: 10, NoCopy: true})
	if err != nil {
		globalLogger.Panic(ctx, errors.Wrap(err, "failed to init traceWALBackup"))
	}
	for _, o := range opts {
		o(globalTelemetry)
	}
	if len(globalTelemetry.cfg.ExporterURLs) > 0 {
		exporterConns = make([]*grpc.ClientConn, 0, len(globalTelemetry.cfg.ExporterURLs))
		for _, exporterUrl := range globalTelemetry.cfg.ExporterURLs {
			grpcOpts := []grpc.DialOption{}
			if globalTelemetry.cfg.Debug {
				grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
			} else {
				tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
				if globalTelemetry.cfg.tls != nil {
					tlsCfg = globalTelemetry.cfg.tls
				}
				creds := credentials.NewTLS(tlsCfg)
				grpcOpts = append(grpcOpts,
					grpc.WithTransportCredentials(creds),
				)
			}
			grpcOpts = append(grpcOpts,
				grpc.WithDefaultCallOptions(
					grpc.MaxCallRecvMsgSize(100*1024*1024),
					grpc.UseCompressor(gzip.Name),
				),
				grpc.WithNoProxy(),
				grpc.WithUserAgent(fmt.Sprintf("subzero/%v", globalTelemetry.cfg.Version)),
			)
			exporterConn, grpcErr := grpc.NewClient(exporterUrl, grpcOpts...)
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
	appcontext.GetAppContext(ctx).OnShutdown(func() error {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultExportTimeout)
		defer shutdownCancel()
		MustShutdown(shutdownCtx)
		return nil
	})
}

func MustShutdown(ctx context.Context) {
	if !globalTelemetry.redundantLogExporter.closing.Swap(true) {
		return
	}
	defer func() {
		if globalTelemetry.redundantLogExporter.logWALBackup != nil {
			if err := errors.Join(
				errors.Wrap(globalTelemetry.redundantLogExporter.logWALBackup.Sync(), "failed to sync logWALBackup on shutdown"),
				errors.Wrap(globalTelemetry.redundantLogExporter.logWALBackup.Close(), "failed to logWALBackup close"),
			); err != nil {
				fmt.Printf("%v %+v \n", "failed to logWALBackup sync and close", err)
			}
		}
	}()
	if err := globalTelemetry.traceProvider.ForceFlush(ctx); err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to ForceFlush traceProvider"))
	}
	if err := globalTelemetry.traceProvider.Shutdown(ctx); err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to shutdown traceProvider"))
	}
	if err := globalTelemetry.logProvider.ForceFlush(ctx); err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to ForceFlush logProvider"))
	}
	if err := globalTelemetry.logProvider.Shutdown(ctx); err != nil {
		globalLogger.Error(ctx, errors.Wrap(err, "failed to shutdown logProvider"))
	}
}

func mustNewFallbackExporter() otelsdklog.Exporter {
	std, err := otelstdoutlog.New(otelstdoutlog.WithWriter(os.Stdout))
	if err != nil {
		panic(err)
	}
	return std
}
func mustNewLogWAL() *wal.Log {
	logWAL, err := wal.Open("subzero-tmp-bkp-logfile", &wal.Options{SegmentCacheSize: 10, NoCopy: true})
	if err != nil {
		panic(errors.Wrap(err, "failed to init logWALBackup"))
	}
	return logWAL
}
