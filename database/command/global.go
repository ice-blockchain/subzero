// SPDX-License-Identifier: ice License 1.0

package command

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/ice-blockchain/cometbft/config"
	cmtlog "github.com/ice-blockchain/cometbft/libs/log"
	"github.com/ice-blockchain/cometbft/multiplex"
	"github.com/ice-blockchain/cometbft/multiplex/client"
	"github.com/ice-blockchain/cometbft/multiplex/runtime"
	"github.com/ice-blockchain/cometbft/p2p"
	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/database/query"
	"github.com/ice-blockchain/subzero/model"
)

var ErrUserIsNotPresentedOnRelay = errors.Errorf("user is not presented on relay")

var disabled = false

type (
	CallbackFunc func(context.Context, ...*model.Event) error
	QueryFunc    func(context.Context, ...model.Filter) query.EventIterator
)

func RegisterRollbackListener(listen CallbackFunc) {
	rollback = listen
}
func RegisterAcceptListener(listen CallbackFunc) {
	consensusEventListener = listen
}
func RegisterCommitListener(listen CallbackFunc) {
	commitEventListener = listen
}

type Consensus interface {
	AcceptEvents(ctx context.Context, events ...*model.Event) error
}

var (
	globalConsensus struct {
		Consensus *consensus
		Once      sync.Once
	}
	consensusEventListener CallbackFunc
	commitEventListener    CallbackFunc
	rollback               CallbackFunc
)

type Config struct {
	AbsoluteRootPath           string `yaml:"absolute-root-path"`
	AbsoluteNodePrivateKeyPath string `yaml:"absolute-node-private-key-path"`
	ExternalAddress            string `yaml:"external-address"`
	RelayUrl                   string `yaml:"relay-url"`
	DiscoveryPort              uint16 `yaml:"discovery-port"`
	Debug                      bool   `yaml:"debug"`
}

type Option func(*consensus)

func WithConfig(cfg *Config) Option {
	return func(c *consensus) {
		if cfg == nil {
			return
		}
		if cfg.AbsoluteNodePrivateKeyPath != "" {
			c.Config.AbsoluteNodePrivateKeyPath = cfg.AbsoluteNodePrivateKeyPath
		}
		if cfg.AbsoluteRootPath != "" {
			c.Config.AbsoluteRootPath = cfg.AbsoluteRootPath
		}
		if cfg.DiscoveryPort != 0 {
			c.Config.DiscoveryPort = cfg.DiscoveryPort
		}
	}
}

func WithQuery(fn QueryFunc) Option {
	return func(c *consensus) {
		if fn == nil {
			return
		}
		c.Query = fn
	}
}

func WithClient(client client.Client) Option {
	return func(c *consensus) {
		if client != nil {
			c.Client = client
		}
	}
}

func MustInit(ctx context.Context, opts ...Option) {
	conf := cfg.MustGet[Config]()
	if disabled || strings.Contains(conf.RelayUrl, ".testnet.") || (conf.RelayUrl == "" && conf.AbsoluteRootPath == "" && conf.DiscoveryPort == 0) {
		disabled = true
		return
	}
	globalConsensus.Once.Do(func() {
		globalConsensus.Consensus = mustInit(ctx, config.DefaultConfig(), opts...)
	})

}

func mustInit(ctx context.Context, serverCfg *config.Config, opts ...Option) *consensus {
	var c = &consensus{
		ServerConfig: serverCfg,
		Logger:       cmtlog.NewFilter(cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout)), cmtlog.AllowError()),
		Config:       cfg.MustGet[Config](),
		Query: func(ctx context.Context, f ...model.Filter) query.EventIterator {
			return query.GetStoredEvents(ctx, f...)
		},
		ShutdownCh: make(chan chan<- error, 1),
	}

	for _, fn := range opts {
		fn(c)
	}

	serverCfg.SetRoot(c.Config.AbsoluteRootPath)
	serverCfg.NodeKey = c.Config.AbsoluteNodePrivateKeyPath
	serverCfg.MultiplexConfig = config.MultiplexBaseConfig(
		map[string]string{},
		map[string][]string{},
	).MultiplexConfig
	serverCfg.P2P.MaxPacketMsgPayloadSize = 1 * 1024 * 1024
	serverCfg.DBBackend = "goleveldb"
	serverCfg.DiscoveryPort = c.Config.DiscoveryPort
	if c.Config.ExternalAddress != "" {
		serverCfg.P2P.ExternalAddress = c.Config.ExternalAddress
		serverCfg.RPC.ListenAddress = c.Config.ExternalAddress
	}
	if c.Config.Debug {
		serverCfg.P2P.AllowDuplicateIP = true
		serverCfg.P2P.AddrBookStrict = false
		serverCfg.Instrumentation.Prometheus = true
		if c.Config.ExternalAddress != "" {
			serverCfg.Instrumentation.PrometheusListenAddr = c.Config.ExternalAddress
		} else {
			serverCfg.Instrumentation.PrometheusListenAddr = fmt.Sprintf("htp://0.0.0.0:%v", c.Config.DiscoveryPort+3)
		}
		c.Logger = cmtlog.NewTMLogger(cmtlog.NewSyncWriter(os.Stdout))
		c.Logger = c.Logger.With("port", c.Config.DiscoveryPort)
	}
	if err := os.MkdirAll(filepath.Dir(serverCfg.NodeKeyFile()), 0666); err != nil {
		panic(errors.Wrapf(err, "failed to create consensus dir"))
	}
	_, err := p2p.LoadOrGenNodeKey(serverCfg.NodeKeyFile())
	if err != nil {
		panic(errors.Wrapf(err, "failed to generate consensus node key"))
	}
	cometbftServer, err := multiplex.NewServer(ctx, c, serverCfg, c.Logger,
		multiplex.WithRuntimeManagerOptions(
			runtime.RegistryWithConsensusOptions(
				runtime.ConsensusPoolWithAcceptor(c),
			),
		),
	)
	if err != nil {
		panic(errors.Wrapf(err, "failed to start consensus server"))
	}
	if err = cometbftServer.Start(); err != nil {
		panic(errors.Wrapf(err, "failed to start consensus server"))
	}
	c.Server = cometbftServer

	if c.Client == nil {
		c.Client = multiplex.NewClient(multiplex.WithBackend(cometbftServer))
	}
	go c.waitForStop(ctx)

	return c
}

func AcceptEvents(ctx context.Context, events ...*model.Event) error {
	if disabled {
		return nil
	}
	return errors.Wrapf(
		globalConsensus.Consensus.AcceptEvents(ctx, events...),
		"errors occured while broadcasting events on %v",
		globalConsensus.Consensus.Config.RelayUrl)
}

func (c *consensus) AcceptEvents(ctx context.Context, events ...*model.Event) error {
	return c.broadcastUserEvents(ctx, events...)
}

func RootPath() string {
	conf := cfg.MustGet[Config]()
	return conf.AbsoluteRootPath
}
