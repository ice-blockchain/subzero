// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"time"

	"github.com/ice-blockchain/subzero/model"
)

type (
	Config struct {
		MaxContentSizes           map[int]int   `yaml:"max-content-sizes"` // Kind -> size (bytes).
		RelayURL                  string        `yaml:"relay-url" validate:"omitempty,url"`
		IONIdentityBaseURL        string        `yaml:"ion-identity-base-url" validate:"omitempty,url"`
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		NIP13MinLeadingZeroBits   int           `yaml:"nip13MinLeadingZeroBits"`
	}
)

func (c *Config) MaxContentSizeOf(kind int) int {
	if c != nil {
		if size, ok := c.MaxContentSizes[kind]; ok {
			return size
		}
	}
	return 0
}

func (c *Config) EqualRelayURL(relayURL string) bool {
	if c == nil || c.RelayURL == "" {
		return true
	}

	return model.CompareRelaysURLs(c.RelayURL, relayURL)
}
