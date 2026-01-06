// SPDX-License-Identifier: ice License 1.0

package config

import (
	"crypto/tls"
	"time"
)

type (
	Config struct {
		TLSConfig    *tls.Config
		WriteTimeout time.Duration `yaml:"writeTimeout"`
		ReadTimeout  time.Duration `yaml:"readTimeout"`
		BindingPorts []uint16      `yaml:"binding-ports"`
		Debug        bool          `yaml:"debug"`
	}
)
