package config

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"sort"

	"github.com/spf13/afero"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

func loadViper(
	args []string,
	getEnv func(string, string) string,
	fs afero.Fs,
	configFile string,
	loggerArgs ...*slog.Logger,
) (*Config, error) {
	logger := slog.Default()
	if len(loggerArgs) > 0 && loggerArgs[0] != nil {
		logger = loggerArgs[0]
	}
	if fs == nil {
		fs = afero.NewOsFs()
	}

	values := viper.New()
	values.SetFs(fs)
	var conf Config
	configValue := reflect.ValueOf(&conf).Elem()
	configType := configValue.Type()
	knownKeys := make(map[string]ConfigurationSetting)
	settingsByField := make(map[string]ConfigurationSetting)
	settings := make([]ConfigurationSetting, 0, len(Catalog().Settings))
	for _, setting := range Catalog().Settings {
		if setting.CloudOnly && !includeCloudConfigFields() {
			continue
		}
		field, ok := configType.FieldByName(setting.Field)
		if !ok {
			return nil, fmt.Errorf("configuration catalog field %q is missing", setting.Field)
		}
		knownKeys[setting.ConfigFileKey] = setting
		settingsByField[setting.Field] = setting
		settings = append(settings, setting)
		values.SetDefault(setting.ConfigFileKey, setting.Default)
		setDefault(configValue.FieldByIndex(field.Index), values.GetString(setting.ConfigFileKey))
	}

	if configFile != "" {
		contents, err := afero.ReadFile(fs, configFile)
		if err != nil {
			return nil, fmt.Errorf("read configuration file %q: %w", configFile, err)
		}
		if err := validateRawConfigFileKeys(contents, knownKeys); err != nil {
			return nil, err
		}
		values.SetConfigType("yaml")
		if err := values.ReadConfig(bytes.NewReader(contents)); err != nil {
			return nil, fmt.Errorf("parse configuration file %q: invalid YAML", configFile)
		}
		unknown := make([]string, 0)
		for _, key := range values.AllKeys() {
			if _, ok := knownKeys[key]; !ok {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return nil, fmt.Errorf("unknown configuration key %q", unknown[0])
		}
		for _, setting := range settings {
			key := setting.ConfigFileKey
			if !values.InConfig(key) {
				continue
			}
			field, _ := configType.FieldByName(setting.Field)
			if !setEnvValue(configValue.FieldByIndex(field.Index), values.GetString(key)) {
				return nil, fmt.Errorf("configuration key %q has invalid type", key)
			}
		}
	}

	for index := range configType.NumField() {
		field := configType.Field(index)
		setting, ok := settingsByField[field.Name]
		if !ok || setting.Environment == "" {
			continue
		}
		envValue := getEnv(setting.Environment, "")
		if envValue == "" {
			continue
		}
		values.Set(setting.ConfigFileKey, envValue)
		if !setEnvValue(configValue.Field(index), values.GetString(setting.ConfigFileKey)) {
			logger.Warn("Invalid value in env var, using default", "key", setting.Environment)
		}
	}

	flags := flag.NewFlagSet("hitkeep", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	registerFlags(flags, &conf)
	registerCloudFlags(flags, &conf)
	_ = flags.Parse(args)

	if conf.Healthcheck {
		return &conf, nil
	}
	normalizeConfig(&conf, logger)
	return &conf, nil
}

func validateRawConfigFileKeys(contents []byte, knownKeys map[string]ConfigurationSetting) error {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("parse configuration file: invalid YAML")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return fmt.Errorf("parse configuration file: invalid YAML")
		}
		return fmt.Errorf("configuration file must contain exactly one YAML document")
	}
	if len(document.Content) == 0 {
		return nil
	}
	mapping := document.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("configuration file must contain a top-level mapping")
	}
	seen := make(map[string]struct{}, len(mapping.Content)/2)
	for index := 0; index < len(mapping.Content); index += 2 {
		key := mapping.Content[index]
		value := mapping.Content[index+1]
		if key.Kind == yaml.AliasNode || value.Kind == yaml.AliasNode {
			return fmt.Errorf("configuration file aliases are not supported")
		}
		if key.Tag == "!!merge" || key.Value == "<<" {
			return fmt.Errorf("configuration file merge keys are not supported")
		}
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return fmt.Errorf("unknown configuration key %q", key.Value)
		}
		if _, ok := seen[key.Value]; ok {
			return fmt.Errorf("duplicate configuration key %q", key.Value)
		}
		seen[key.Value] = struct{}{}
		if _, ok := knownKeys[key.Value]; !ok {
			return fmt.Errorf("unknown configuration key %q", key.Value)
		}
		if value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			return fmt.Errorf("configuration value for %q must be a scalar", key.Value)
		}
	}
	return nil
}
