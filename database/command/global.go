// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/cometbft/config"
	cmtlog "github.com/ice-blockchain/cometbft/libs/log"
	"github.com/ice-blockchain/cometbft/multiplex"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var ErrUserIsNotPresentedOnRelay = errors.Errorf("user is not presented on relay")

func RegisterRollbackListener(listen func(context.Context, ...*model.Event) error) {
	rollback = listen
}
func RegisterAcceptListener(listen func(context.Context, ...*model.Event) error) {
	consensusEventListener = listen
}

var globalConsensus *consensus
var globalCfg *commandConfig
var consensusEventListener func(context.Context, ...*model.Event) error
var rollback func(context.Context, ...*model.Event) error

type commandConfig struct {
	AbsoluteRootPath        string `yaml:"absolute-root-path"`
	NodePrivKey             string `yaml:"absolute-node-private-key-path"`
	DiscoveryPort           uint16 `yaml:"discovery-port"`
	Debug                   bool   `yaml:"debug"`
	NIP13MinLeadingZeroBits int    `yaml:"nip13MinLeadingZeroBits"`
}

func MustInit(ctx context.Context) {
	globalConsensus = mustInit(ctx).(*consensus)
}

func mustInit(ctx context.Context) Consensus {
	globalCfg = cfg.MustGet[commandConfig]()
	c := &consensus{}
	serverCfg := config.DefaultConfig()
	serverCfg.SetRoot(globalCfg.AbsoluteRootPath)
	serverCfg.NodeKey = globalCfg.NodePrivKey
	serverCfg.MultiplexConfig = config.MultiplexBaseConfig(
		map[string]string{},
		//map[string][]string{"4DB7A5F1CB00DF46BACDE404EBAFAD3E36484A39": []string{"mx-chain-4DB7A5F1CB00DF46BACDE404EBAFAD3E36484A39-01DB91D06032CC64"}},
		map[string][]string{},
	).MultiplexConfig
	serverCfg.DiscoveryPort = globalCfg.DiscoveryPort
	logger := cmtlog.NewFilter(cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout)), cmtlog.AllowError())
	if globalCfg.Debug {
		serverCfg.P2P.AllowDuplicateIP = true
		serverCfg.P2P.AddrBookStrict = false
		logger = cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
	}
	cometbftServer, err := multiplex.NewServer(c, serverCfg, logger)
	if err != nil {
		panic(err)
	}
	c.server = cometbftServer
	c.server.MustStart()
	c.client = multiplex.NewClient(multiplex.WithBackend(cometbftServer))
	go func() {
		<-ctx.Done()
		c.server.Close()
	}()
	return c
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return errors.Wrapf(globalConsensus.broadcastUserEvents(ctx, events...), "errors occured while broadcasting events")
}
