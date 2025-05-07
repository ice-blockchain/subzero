// SPDX-License-Identifier: ice License 1.0

//go:build test

package command

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"github.com/ice-blockchain/cometbft/config"
	"github.com/ice-blockchain/cometbft/crypto/ed25519"
	"github.com/ice-blockchain/cometbft/multiplex"
	"github.com/ice-blockchain/cometbft/p2p"
)

type TestConsensus interface {
	Consensus
	DiscoveryPort() uint16
	Stop(ctx context.Context, timeout time.Duration) error
	Start(ctx context.Context)
}

func (c *consensus) DiscoveryPort() uint16 {
	return c.Config.DiscoveryPort
}

func (c *consensus) Start(ctx context.Context) {
	if c.Server != nil {
		panic("the server is already running")
	}
	c.ServerConfig.Instrumentation.Namespace = uuid.NewString()
	server, err := multiplex.NewServer(c, c.ServerConfig, c.Logger)
	if err != nil {
		panic(errors.Wrapf(err, "failed to start consensus server"))
	}
	server.MustStart()
	c.Server = server
	go c.waitForStop(ctx)
}

func (c *consensus) Stop(ctx context.Context, timeout time.Duration) error {
	ch := make(chan error, 1)

	select {
	case c.ShutdownCh <- ch:
		c.Logger.Debug("shutdown request was sent")
	default:
		return fmt.Errorf("cannot stop consensus %p, already shutting down", c)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-time.After(timeout):
		c.Logger.Error("timeout waiting for shutdown")
		return context.DeadlineExceeded

	case err := <-ch:
		return err
	}
}

func NewConsensusNode(ctx context.Context, nodeCfg *config.Config, port uint16, opts ...Option) (TestConsensus, func() error) {
	return newConsensusNode(ctx, nodeCfg, port, opts...)
}

func newConsensusNode(ctx context.Context, nodeCfg *config.Config, port uint16, opts ...Option) (*consensus, func() error) {
	if nodeCfg == nil {
		nodeCfg = config.DefaultConfig()
	}

	pattern := "cometbft-" + strconv.Itoa(int(port))
	storage, err := os.MkdirTemp("", pattern+"-storage")
	if err != nil {
		log.Panicf("failed to create temp dir: %v", err)
	}

	keyPath, err := os.CreateTemp("", pattern+"-nodekey")
	if err != nil {
		log.Panicf("failed to create temp file: %v", err)
	}

	nodeKey := &p2p.NodeKey{
		PrivKey: ed25519.GenPrivKey(),
	}

	if err := nodeKey.SaveAs(keyPath.Name()); err != nil {
		log.Panicf("failed to save node key: %v in %s", err, keyPath.Name())
	}

	log.Printf("createating test node: port=%d, storage=%s, keyPath=%s", port, storage, keyPath.Name())

	var options = []Option{
		WithConfig(&Config{
			DiscoveryPort:              port,
			AbsoluteRootPath:           storage,
			AbsoluteNodePrivateKeyPath: keyPath.Name(),
		}),
	}
	if len(opts) > 0 {
		options = append(options, opts...)
	}

	c := mustInit(ctx, nodeCfg, options...)

	return c, func() error {
		return errors.Join(
			os.RemoveAll(storage),
			os.Remove(keyPath.Name()),
		)
	}
}
