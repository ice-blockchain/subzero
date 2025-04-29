// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"os"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/cometbft/config"
	cmtlog "github.com/ice-blockchain/cometbft/libs/log"
	"github.com/ice-blockchain/cometbft/multiplex"
	"github.com/ice-blockchain/cometbft/p2p"
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

type Consensus interface {
	AcceptEvents(ctx context.Context, events ...*model.Event) error
}

var globalConsensus *consensus
var once sync.Once
var globalCfg *Config

var consensusEventListener func(context.Context, ...*model.Event) error
var rollback func(context.Context, ...*model.Event) error

type Config struct {
	AbsoluteRootPath string `yaml:"absolute-root-path"`
	NodePrivKey      string `yaml:"absolute-node-private-key-path"`
	DiscoveryPort    uint16 `yaml:"discovery-port"`
	ExternalAddress  string `yaml:"external-address"`
	Debug            bool   `yaml:"debug"`
	RelayUrl         string `yaml:"relay-url"`
}

type Option func(cfg *Config)

func WithConfig(cfg *Config) Option {
	return func(in *Config) {
		if cfg == nil {
			return
		}
		if cfg.NodePrivKey != "" {
			in.NodePrivKey = cfg.NodePrivKey
		}
		if cfg.AbsoluteRootPath != "" {
			in.AbsoluteRootPath = cfg.AbsoluteRootPath
		}
		if cfg.DiscoveryPort != 0 {
			in.DiscoveryPort = cfg.DiscoveryPort
		}
	}
}

func MustInit(ctx context.Context, opts ...Option) {
	once.Do(func() {
		globalConsensus = mustInit(ctx, config.DefaultConfig(), opts...).(*consensus)
	})

}

func mustInit(ctx context.Context, serverCfg *config.Config, opts ...Option) Consensus {
	globalCfg = cfg.MustGet[Config]()
	for _, o := range opts {
		o(globalCfg)
	}
	c := &consensus{
		cfg:        globalCfg,
		shutdownCh: make(chan struct{}, 1),
	}
	serverCfg.SetRoot(globalCfg.AbsoluteRootPath)
	serverCfg.NodeKey = globalCfg.NodePrivKey
	serverCfg.MultiplexConfig = config.MultiplexBaseConfig(
		map[string]string{},
		map[string][]string{},
	).MultiplexConfig
	serverCfg.P2P.MaxPacketMsgPayloadSize = 100 * 1024 * 1024
	serverCfg.DBBackend = "goleveldb"
	serverCfg.DiscoveryPort = globalCfg.DiscoveryPort
	if globalCfg.ExternalAddress != "" {
		serverCfg.P2P.ExternalAddress = globalCfg.ExternalAddress
		serverCfg.RPC.ListenAddress = globalCfg.ExternalAddress
	}
	logger := cmtlog.NewFilter(cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout)), cmtlog.AllowError())

	if globalCfg.Debug {
		serverCfg.P2P.AllowDuplicateIP = true
		serverCfg.P2P.AddrBookStrict = false
		logger = cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
		logger = logger.With("port", globalCfg.DiscoveryPort)
	}
	_, err := p2p.LoadOrGenNodeKey(serverCfg.NodeKeyFile())
	if err != nil {
		panic(errors.Wrapf(err, "failed to generate consensus node key"))
	}
	cometbftServer, err := multiplex.NewServer(c, serverCfg, logger)
	if err != nil {
		panic(err)
	}
	c.server = cometbftServer
	c.server.MustStart()
	c.client = multiplex.NewClient(multiplex.WithBackend(cometbftServer))
	go func() {
		select {
		case <-ctx.Done():
		case <-c.shutdownCh:
			break
		}
		c.server.Close()
	}()
	return c
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return errors.Wrapf(globalConsensus.AcceptEvents(ctx, events...), "errors occured while broadcasting events")
}

func (c *consensus) AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return c.broadcastUserEvents(ctx, events...)
}
