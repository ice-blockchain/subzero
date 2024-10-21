// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"log"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/spf13/viper"
)

const (
	defaultYAMLConfigurationFilePath = "/etc/subzero_ion_connect/subzero_ion_connect.yaml"
)

var (
	yamlConfigurationFilePathInitializer = new(sync.Once)
	yamlConfigurationFilePath            string
)

func MustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePathInitializer.Do(func() { mustInit(absoluteCfgPaths...) })
}

func mustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePath = ""
	for _, path := range absoluteCfgPaths {
		viper.SetConfigFile(path)
		if err := viper.ReadInConfig(); err == nil {
			yamlConfigurationFilePath = path
			break
		}
	}
	if yamlConfigurationFilePath == "" {
		if len(absoluteCfgPaths) > 0 {
			log.Printf("warn: could not find any of the provided file paths %+v, defaulting to `%v`", absoluteCfgPaths, defaultYAMLConfigurationFilePath)
		}
		yamlConfigurationFilePath = defaultYAMLConfigurationFilePath
	}
}

func MustGet[T any]() *T {
	var t T
	key := strings.Replace(reflect.TypeOf(t).PkgPath(), "github.com/ice-blockchain/subzero/", "", 1)
	if err := viper.UnmarshalKey(key, &t); err != nil {
		log.Panic(errors.Wrapf(err, "could not deserialised `%v` yaml key `%v` into %+v", yamlConfigurationFilePath, key, t))
	}

	return &t
}
