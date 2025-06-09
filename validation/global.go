// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"context"
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
)

func mustLoadConfig() *Config {
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

func MustInit() {
	globalValidator.Once.Do(func() {
		conf := mustLoadConfig()
		globalConfig = &config{
			MaxWrappedEventExpiration: conf.MaxWrappedEventExpiration,
			MaxContentSizes:           conf.MaxContentSizes,
			NIP13MinLeadingZeroBits:   conf.NIP13MinLeadingZeroBits,
			RelayURL:                  conf.RelayURL,
		}

		globalValidator.Instance = NewEventValidator(conf)
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

func Validate(ctx context.Context, events ...*model.Event) error {
	if globalValidator.Instance == nil {
		MustInit()
	}

	return globalValidator.Instance.Validate(ctx, events...)
}
