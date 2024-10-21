// SPDX-License-Identifier: ice License 1.0

package config

import (
	"crypto/tls"
	"time"
)

type (
	Config struct {
		TLSConfig               *tls.Config
		NIP13MinLeadingZeroBits int           `yaml:"nip13MinLeadingZeroBits"`
		Port                    uint16        `yaml:"port"`
		WriteTimeout            time.Duration `yaml:"writeTimeout"`
		ReadTimeout             time.Duration `yaml:"readTimeout"`
		Debug                   bool          `yaml:"debug"`
	}
)
