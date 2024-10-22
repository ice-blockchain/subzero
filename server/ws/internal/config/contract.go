// SPDX-License-Identifier: ice License 1.0

package config

import stdlibtime "time"

type (
	Config struct {
		CertPath                string              `yaml:"certPath"`
		KeyPath                 string              `yaml:"keyPath"`
		NIP13MinLeadingZeroBits int                 `yaml:"nip13MinLeadingZeroBits"`
		Port                    uint16              `yaml:"port"`
		WriteTimeout            stdlibtime.Duration `yaml:"writeTimeout"`
		ReadTimeout             stdlibtime.Duration `yaml:"readTimeout"`
		Debug                   bool                `yaml:"debug"`
	}
)
