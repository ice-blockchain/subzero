package config

import stdlibtime "time"

type (
	Config struct {
		CertPath     string              `yaml:"certPath"`
		KeyPath      string              `yaml:"keyPath"`
		Port         uint16              `yaml:"port"`
		WriteTimeout stdlibtime.Duration `yaml:"writeTimeout"`
		ReadTimeout  stdlibtime.Duration `yaml:"readTimeout"`
	}
)
