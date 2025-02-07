// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"time"

	"github.com/ice-blockchain/subzero/cfg"
)

type (
	config struct {
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		MaxPostSizes              map[int]int   `yaml:"max-post-sizes"` // Kind -> size (bytes).
	}
)

var (
	globalConfig *config
)

func init() {
	globalConfig = cfg.MustGet[config]()
}

func (c *config) MaxPostSizeOf(kind int) int {
	if c != nil {
		if size, ok := c.MaxPostSizes[kind]; ok {
			return size
		}
	}
	return 0
}
