// SPDX-License-Identifier: ice License 1.0

//go:build test

package command

import (
	"context"

	"github.com/google/uuid"

	"github.com/ice-blockchain/cometbft/config"
)

type TestConsensus interface {
	Consensus
	DiscoveryPort() uint16
	Stop()
}

func (c *consensus) DiscoveryPort() uint16 {
	return c.cfg.DiscoveryPort
}

func (c *consensus) Stop() {
	close(c.shutdownCh)
}

func GetConsensus(ctx context.Context, opts ...Option) TestConsensus {
	return mustInit(ctx, config.DefaultConfig(), opts...).(*consensus)
}
func GetConsensusWithMetricsOverride(ctx context.Context, opts ...Option) TestConsensus {
	cfg := config.DefaultConfig()
	cfg.Instrumentation.Namespace = uuid.NewString()
	return mustInit(ctx, cfg, opts...).(*consensus)
}
