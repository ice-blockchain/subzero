// SPDX-License-Identifier: ice License 1.0

//go:build test

package command

import "context"

type TestConsensus interface {
	Consensus
	DiscoveryPort() uint16
}

func (c *consensus) DiscoveryPort() uint16 {
	return c.cfg.DiscoveryPort
}

func GetConsensus(ctx context.Context, opts ...Option) TestConsensus {
	return mustInit(ctx, opts...).(*consensus)
}
