// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"time"

	"github.com/ice-blockchain/subzero/cfg"
)

type (
	config struct {
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		MaxPostSize               int           `yaml:"max-post-size"`
	}
)

var (
	globalConfig *config
)

func init() {
	globalConfig = cfg.MustGet[config]()
}
