// SPDX-License-Identifier: ice License 1.0

package cfg

import (
	"reflect"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/go-playground/validator/v10"
	"github.com/go-viper/mapstructure/v2"
	"github.com/rs/zerolog/log"
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
			log.Warn().Strs("provided_paths", absoluteCfgPaths).Str("default_path", DefaultYAMLConfigurationFilePath).Msg("could not find any of the provided file paths, using default value")
		}
		yamlConfigurationFilePath = DefaultYAMLConfigurationFilePath
		globalViper.SetConfigFile(yamlConfigurationFilePath)
		if err := globalViper.ReadInConfig(); err != nil {
			log.Panic().
				Err(err).
				Str("file_path", yamlConfigurationFilePath).
				Msg("failed to read yaml config file")
		}
	}
}

func MustGet[T any]() *T {
	value, err := Get[T]()
	if err != nil {
		log.Panic().Err(err).Msg("failed to get config")
	}
	return value
}

func Validate[T any](cfg *T) (err error) {
	return globalValidator.Struct(cfg)
}

func Load[T any]() (*T, error) {
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

	return &t, nil
}

func Get[T any]() (*T, error) {
	t, err := Load[T]()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load config of type `%v`", reflect.TypeOf(t))
	}

	if err = Validate(t); err != nil {
		return nil, errors.Wrapf(err, "validation failed for config of type `%v` with value %+v", reflect.TypeOf(t), t)
	}

	return t, nil
}
