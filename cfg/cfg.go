// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"log"
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	DefaultYAMLConfigurationFilePath = "/etc/subzero_ion_connect/subzero_ion_connect.yaml"
)

var (
	yamlConfigurationFilePathInitializer = new(sync.Once)
	yamlConfigurationFilePath            string
	globalViper                          = viper.NewWithOptions(viper.KeyDelimiter("/"))
	globalValidator                      = validator.New(validator.WithRequiredStructEnabled())
)

func MustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePathInitializer.Do(func() { mustInit(absoluteCfgPaths...) })
}

func mustInit(absoluteCfgPaths ...string) {
	yamlConfigurationFilePath = ""
	globalViper.SetConfigType("yaml")
	for _, path := range absoluteCfgPaths {
		if path == "" {
			continue
		}
		globalViper.SetConfigFile(path)
		if err := globalViper.ReadInConfig(); err == nil {
			yamlConfigurationFilePath = path
			break
		}
	}
	if yamlConfigurationFilePath == "" {
		if len(absoluteCfgPaths) > 0 && absoluteCfgPaths[0] != "" {
			log.Printf("warn: could not find any of the provided file paths %+v, defaulting to `%v`", absoluteCfgPaths, DefaultYAMLConfigurationFilePath)
		}
		yamlConfigurationFilePath = DefaultYAMLConfigurationFilePath
		globalViper.SetConfigFile(yamlConfigurationFilePath)
		if err := globalViper.ReadInConfig(); err != nil {
			log.Printf("failed to read yaml config file at `%v`", yamlConfigurationFilePath)
		}
	}
}

func MustGet[T any]() *T {
	value, err := Get[T]()
	if err != nil {
		log.Panicf("failed to get config: %v", err)
	}
	return value
}

func Validate[T any](cfg *T) (err error) {
	return globalValidator.Struct(cfg)
}

func Get[T any]() (*T, error) {
	var t T

	typeOf := reflect.TypeOf(t)
	if typeOf.Kind() != reflect.Struct {
		return nil, errors.Errorf("type `%v` is not a struct", typeOf)
	}

	key := strings.Replace(typeOf.PkgPath(), "github.com/ice-blockchain/subzero/", "", 1)
	if err := globalViper.UnmarshalKey(key, &t, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.ZeroFields = true
		decoderConfig.WeaklyTypedInput = true
		decoderConfig.Squash = true
		decoderConfig.IgnoreUntaggedFields = true
		decoderConfig.TagName = "yaml"
	}); err != nil {
		return nil, errors.Wrapf(err, "could not deserialised `%v` yaml key `%v` into %+v", yamlConfigurationFilePath, key, t)
	}

	if err := Validate(&t); err != nil {
		return nil, errors.Wrapf(err, "could not validate `%v` yaml key `%v`: %v", yamlConfigurationFilePath, key, err)
	}

	return &t, nil
}
