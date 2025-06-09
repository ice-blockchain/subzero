// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/ice-blockchain/subzero/cfg"
	"github.com/ice-blockchain/subzero/model"
)

var (
	globalValidator struct {
		Instance Validator
		Once     sync.Once
	}
)

type (
	Config struct {
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		MaxContentSizes           map[int]int   `yaml:"max-content-sizes"`
		NIP13MinLeadingZeroBits   int           `yaml:"nip13MinLeadingZeroBits"`
		RelayURL                  string        `yaml:"relay-url" validate:"omitempty,url"`
	}
	Option func(*Config)
)

func WithConfig(cfg *Config) Option {
	return func(in *Config) {
		if cfg == nil {
			return
		}
		if cfg.MaxWrappedEventExpiration > 0 {
			in.MaxWrappedEventExpiration = cfg.MaxWrappedEventExpiration
		}
		if cfg.MaxContentSizes != nil {
			in.MaxContentSizes = cfg.MaxContentSizes
		}
		if cfg.NIP13MinLeadingZeroBits > 0 {
			in.NIP13MinLeadingZeroBits = cfg.NIP13MinLeadingZeroBits
		}
		if cfg.RelayURL != "" {
			in.RelayURL = cfg.RelayURL
		}
	}
}

func mustLoadConfig(opts ...Option) *Config {
	if len(opts) == 0 {
		if globalConfig != nil {
			return &Config{
				MaxWrappedEventExpiration: globalConfig.MaxWrappedEventExpiration,
				MaxContentSizes:           globalConfig.MaxContentSizes,
				NIP13MinLeadingZeroBits:   globalConfig.NIP13MinLeadingZeroBits,
				RelayURL:                  globalConfig.RelayURL,
			}
		}
		return cfg.MustGet[Config]()
	}

	conf, err := cfg.Get[Config]()
	if err != nil {
		if globalConfig != nil {
			conf = &Config{
				MaxWrappedEventExpiration: globalConfig.MaxWrappedEventExpiration,
				MaxContentSizes:           globalConfig.MaxContentSizes,
				NIP13MinLeadingZeroBits:   globalConfig.NIP13MinLeadingZeroBits,
				RelayURL:                  globalConfig.RelayURL,
			}
		} else {
			conf = &Config{}
		}
	}
	for _, opt := range opts {
		opt(conf)
	}
	if err := cfg.Validate(conf); err != nil {
		log.Panic(err)
	}

	return conf
}

func MustInit() {
	globalValidator.Once.Do(func() {
		conf := mustLoadConfig()
		globalConfig = &config{
			MaxWrappedEventExpiration: conf.MaxWrappedEventExpiration,
			MaxContentSizes:           conf.MaxContentSizes,
			NIP13MinLeadingZeroBits:   conf.NIP13MinLeadingZeroBits,
			RelayURL:                  conf.RelayURL,
		}

		globalValidator.Instance = newEventValidator(globalConfig)
	})
}

func (c *Config) MaxContentSizeOf(kind int) int {
	if c != nil {
		if size, ok := c.MaxContentSizes[kind]; ok {
			return size
		}
	}
	return 0
}

func Validate(ctx context.Context, e *model.Event, events ...*model.Event) error {
	if globalValidator.Instance == nil {
		MustInit()
	}

	return globalValidator.Instance.Validate(ctx, e, events...)
}
