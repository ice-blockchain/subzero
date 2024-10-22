// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"log"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

const (
	defaultYAMLConfigurationFilePath = "/etc/subzero_ion_connect/subzero_ion_connect.yaml"
)

var (
	yamlConfigurationFilePathInitializer = new(sync.Once)
	yamlConfigurationFilePath            string
	globalViper                          = viper.NewWithOptions(viper.KeyDelimiter("/"))
)

func MustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePathInitializer.Do(func() { mustInit(absoluteCfgPaths...) })
}

func mustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePath = ""
	globalViper.SetConfigType("yaml")
	for _, path := range absoluteCfgPaths {
		globalViper.SetConfigFile(path)
		if err := globalViper.ReadInConfig(); err == nil {
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
	typeOf := reflect.TypeOf(t)
	if typeOf.Kind() != reflect.Struct {
		log.Panic("T must be struct")
	}
	key := strings.Replace(typeOf.PkgPath(), "github.com/ice-blockchain/subzero/", "", 1)
	if err := globalViper.UnmarshalKey(key, &t, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.ErrorUnset = true
		decoderConfig.ZeroFields = true
		decoderConfig.WeaklyTypedInput = true
		decoderConfig.Squash = true
		decoderConfig.IgnoreUntaggedFields = true
		decoderConfig.TagName = "yaml"
	}); err != nil {
		log.Panic(errors.Wrapf(err, "could not deserialised `%v` yaml key `%v` into %+v", yamlConfigurationFilePath, key, t))
	}

	return &t
}
