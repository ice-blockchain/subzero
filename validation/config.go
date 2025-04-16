// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"time"

	"github.com/ice-blockchain/subzero/cfg"
)

type (
	config struct {
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		MaxContentSizes           map[int]int   `yaml:"max-content-sizes"`
		RelayURL                  string        `yaml:"relay-url"`
	}
)

var (
	globalConfig *config
)

func init() {
	globalConfig = cfg.MustGet[config]()
}

func (c *config) MaxContentSizeOf(kind int) int {
	if c != nil {
		if size, ok := c.MaxContentSizes[kind]; ok {
			return size
		}
	}
	return 0
}
