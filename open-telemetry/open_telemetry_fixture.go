// SPDX-License-Identifier: ice License 1.0

//go:build test

package opentelemetry

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cockroachdb/errors"
	"github.com/imroc/req/v3"
	"github.com/rs/zerolog/log"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"

	servercert "github.com/ice-blockchain/subzero/server/cert"
)

type (
	OpenTelemetry interface {
		Stop(ctx context.Context, down bool, services ...string) (err error)
		IsRunning(ctx context.Context, service string) (bool, error)
		Start(ctx context.Context, services ...string) (err error)
		GetLogs(ctx context.Context, query Query) ([]string, error)
	}
	openObserveClient struct {
		containers *compose.DockerCompose
		clients    []*req.Client
		clientsIdx uint64
		authToken  string
	}
	Query struct {
		Sql       string `json:"sql"`
		StartTime int64  `json:"start_time"`
		EndTime   int64  `json:"end_time"`
		From      int    `json:"from"`
		Size      int    `json:"size"`
	}
	hits struct {
		Took int `json:"took"`
		Hits []struct {
			Timestamp       int64  `json:"_timestamp"`
			Body            string `json:"body"`
			ServiceInstance string `json:"service_instance_id"`
			ServiceName     string `json:"service_name"`
			Severity        string `json:"severity"`
		} `json:"hits"`
		Total         int      `json:"total"`
		From          int      `json:"from"`
		Size          int      `json:"size"`
		ScanSize      int      `json:"scan_size"`
		FunctionError []string `json:"function_error"`
	}
)

func NewOTServer(ctx context.Context, clientApiToken string) (OpenTelemetry, error) {
	c := &openObserveClient{
		authToken: clientApiToken,
	}
	dockerCompose, err := compose.NewDockerComposeWith(
		compose.WithLogger(c),
		compose.WithStackFiles(locateDockerCompose()...),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to init docker compose %+v", locateDockerCompose())
	}
	if err = dockerCompose.Up(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start docker compose")
	}
	dockerCompose.WaitForService("openobserve-3", wait.ForLog("Starting HTTP server at"))
	c.containers = dockerCompose
	for _, service := range otServices {
		hostedPort, err := c.getContainerPort(ctx, service)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get host port")
		}
		c.clients = append(c.clients, req.SetBaseURL("http://localhost:"+hostedPort+"/"))
	}
	return c, nil
}

func (o *openObserveClient) Printf(format string, v ...any) {
	fmt.Printf(format+"\n", v...)
}

func (o *openObserveClient) req(ctx context.Context) *req.Request {
	idx := atomic.AddUint64(&o.clientsIdx, 1) % uint64(len(o.clients))
	return o.clients[idx].R().
		SetContext(ctx).
		SetHeader("Authorization", fmt.Sprintf("Basic %v", o.authToken))
}

func (o *openObserveClient) IsRunning(ctx context.Context, service string) (bool, error) {
	container, err := o.containers.ServiceContainer(ctx, service)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get container for service %v", service)
	}
	return container.IsRunning(), nil
}

func (o *openObserveClient) Stop(ctx context.Context, down bool, services ...string) (err error) {
	if down {
		return o.containers.Down(ctx)
	}
	if len(services) == 0 {
		services = otServices
	}
	for _, s := range services {
		container, cerr := o.containers.ServiceContainer(ctx, s)
		if cerr != nil {
			return errors.Wrapf(cerr, "failed to get container for service %v", s)
		}
		stopTimeout := 5 * time.Second
		err = errors.Join(err, errors.Wrapf(container.Stop(ctx, &stopTimeout), "failed to stop container for service %v = %v", s, container.ID))
	}
	return errors.Wrapf(err, "failed to stop open telemetry server")
}

func (o *openObserveClient) GetLogs(ctx context.Context, query Query) (logs []string, err error) {
	err = backoff.RetryNotify(
		func() error {
			logs, err = o.getLogs(ctx, query)
			return err
		},
		backoff.WithContext(&backoff.ExponentialBackOff{
			InitialInterval:     1 * time.Second,
			RandomizationFactor: 0.5,
			Multiplier:          2.5,
			MaxInterval:         10 * time.Second,
			MaxElapsedTime:      30 * time.Second,
			Stop:                backoff.Stop,
			Clock:               backoff.SystemClock,
		}, ctx),
		func(e error, next time.Duration) {
			fmt.Printf("retrying in %v: %v\n", next, err.Error())
		})
	return logs, err
}
func (o *openObserveClient) getLogs(ctx context.Context, query Query) ([]string, error) {
	resp, err := o.req(ctx).SetBody(struct {
		Query      Query  `json:"query"`
		SearchType string `json:"search_type"`
		Timeout    int    `json:"timeout"`
	}{
		Query:      query,
		SearchType: "ui",
		Timeout:    0,
	}).Post("/api/default/_search")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get logs from otel server")
	}
	if resp.GetStatusCode() != http.StatusOK {
		return nil, errors.Errorf("search responded with status: %d", resp.GetStatusCode())
	}
	var hit hits
	err = resp.UnmarshalJson(&hit)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal json")
	}
	if len(hit.FunctionError) != 0 {
		return nil, errors.Errorf("search responded with error on %v: %s", resp.Request.URL.String(), strings.Join(hit.FunctionError, " "))
	}
	var logs []string
	for _, h := range hit.Hits {
		logs = append(logs, h.Body)
	}
	return logs, nil
}

func (o *openObserveClient) getContainerPort(ctx context.Context, service string) (string, error) {
	container, err := o.containers.ServiceContainer(ctx, service)
	if err != nil {
		return "", errors.Wrapf(err, "failed to get container for service %v", service)
	}
	inspect, err := container.Inspect(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "failed to inspect container for service %v", service)
	}
	ports := inspect.NetworkSettings.Ports["5080/tcp"]
	if len(ports) == 0 {
		return "", errors.Errorf("failed to get port for service %v", service)
	}
	return ports[0].HostPort, nil
}

func (o *openObserveClient) Start(ctx context.Context, services ...string) (err error) {
	waitForService := "openobserve-3"
	if len(services) == 0 {
		services = otServices
	} else {
		waitForService = services[0]
	}
	for _, s := range services {
		container, cerr := o.containers.ServiceContainer(ctx, s)
		if cerr != nil {
			return errors.Wrapf(cerr, "failed to get container for service %v", s)
		}
		err = errors.Join(err, errors.Wrapf(container.Start(ctx), "failed to start container for service %v = %v", s, container.ID))
	}
	if err != nil {
		return errors.Wrapf(err, "failed to start open telemetry server")
	}
	o.containers.WaitForService(waitForService, wait.ForLog("Starting HTTP server at"))
	return nil
}

func locateDockerCompose() []string {
	var files []string
	var hints []string

	if p, err := os.Getwd(); err == nil {
		hints = append(hints, p)
	}
	if p, err := os.Executable(); err == nil {
		hints = append(hints, path.Dir(filepath.Join(p, "..")))
	}

	for _, dir := range hints {
		pattern := filepath.Join(dir, ".testdata", "docker-compose.yaml")
		if f, err := filepath.Glob(pattern); err != nil {
			log.Error().Err(err).Str("pattern", pattern).Msg("glob failed")
		} else {
			files = append(files, f...)
		}
	}

	return files
}

func WithTLS(tlsConf *tls.Config) Option {
	return func(t *telemetry) {
		t.cfg.tls = tlsConf
	}
}

func ClientTLS() *tls.Config {
	tlscfg := servercert.MustGenerateTLSConfigSelfSigned("localhost")
	cert := tlscfg.Certificates[0]
	var dir string
	for _, p := range locateDockerCompose() {
		if _, pErr := os.Stat(p); !os.IsNotExist(pErr) {
			dir = filepath.Dir(p)
		}
	}
	certPEM := new(bytes.Buffer)
	err := pem.Encode(
		certPEM,
		&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]},
	)
	if err != nil {
		log.Panic().Err(err).Msg("failed to encode certificate to PEM")
	}
	if err = os.WriteFile(filepath.Join(dir, "server.crt"), certPEM.Bytes(), 0644); err != nil {
		log.Panic().Err(errors.New("failed to write cert file"))
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(certPEM.Bytes()); !ok {
		log.Panic().Err(errors.New("failed to append localhost tls to cert pool"))
	}

	keyPEM := new(bytes.Buffer)
	ecBytes, err := x509.MarshalECPrivateKey(cert.PrivateKey.(*ecdsa.PrivateKey))
	if err != nil {
		log.Panic().Err(err).Msg("failed to marshal ECDSA private key")
	}
	err = pem.Encode(
		keyPEM,
		&pem.Block{
			Type:  "EC PRIVATE KEY",
			Bytes: ecBytes,
		},
	)
	if err != nil {
		log.Panic().Err(err).Msg("failed to encode private key to PEM")
	}
	if err = os.WriteFile(filepath.Join(dir, "server.key"), keyPEM.Bytes(), 0644); err != nil {
		log.Panic().Err(errors.New("failed to write key file"))
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}
}
