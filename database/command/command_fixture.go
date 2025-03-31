// SPDX-License-Identifier: ice License 1.0

//go:build test

package command

type TestConsensus interface {
	Consensus
	DiscoveryPort() uint16
}

func (c *consensus) DiscoveryPort() uint16 {
	return c.cfg.DiscoveryPort
}

func GetConsensus() TestConsensus {
	return globalConsensus
}
