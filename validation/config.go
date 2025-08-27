// SPDX-License-Identifier: ice License 1.0

package validation

import (
	"time"
)

type (
	Config struct {
		MaxWrappedEventExpiration time.Duration `yaml:"max-wrapped-event-expiration"`
		MaxContentSizes           map[int]int   `yaml:"max-content-sizes"` // Kind -> size (bytes).
		NIP13MinLeadingZeroBits   int           `yaml:"nip13MinLeadingZeroBits"`
		RelayURL                  string        `yaml:"relay-url" validate:"omitempty,url"`
		IONIdentityURL            string        `yaml:"ion-identity-url" validate:"omitempty,url"`
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
