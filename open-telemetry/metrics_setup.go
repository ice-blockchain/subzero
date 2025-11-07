// SPDX-License-Identifier: ice License 1.0

package opentelemetry

//
//import (
//	"context"
//	"fmt"
//
//	"go.opentelemetry.io/otel"
//	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
//	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
//	"go.opentelemetry.io/otel/sdk/resource"
//	"google.golang.org/grpc"
//)
//
//// Initializes an OTLP exporter, and configures the corresponding meter provider.
//func initMeterProvider(ctx context.Context, res *resource.Resource, conn *grpc.ClientConn) (func(context.Context) error, error) {
//	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
//	if err != nil {
//		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
//	}
//
//	meterProvider := sdkmetric.NewMeterProvider(
//		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
//		sdkmetric.WithResource(res),
//	)
//	otel.SetMeterProvider(meterProvider)
//
//	meter := otel.Meter("bla")
//	println(meter)
//
//	return meterProvider.Shutdown, nil
//}
