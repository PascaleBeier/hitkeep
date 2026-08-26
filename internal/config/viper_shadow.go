package config

import (
	"flag"
	"log/slog"
	"os"
	"reflect"

	"github.com/spf13/viper"
)

// loadViperShadow proves Viper parity without changing runtime configuration ownership.
func loadViperShadow(args []string, getEnv func(string, string) string, loggerArgs ...*slog.Logger) *Config {
	logger := slog.Default()
	if len(loggerArgs) > 0 && loggerArgs[0] != nil {
		logger = loggerArgs[0]
	}

	values := viper.New()
	var conf Config
	configValue := reflect.ValueOf(&conf).Elem()
	configType := configValue.Type()
	for index := range configType.NumField() {
		field := configType.Field(index)
		if !shouldLoadEnvField(field) {
			continue
		}
		key := configFileKey(field.Tag.Get("env"), field.Tag.Get("flag"))
		if key == "" {
			continue
		}
		defaultValue := field.Tag.Get("default")
		values.SetDefault(key, defaultValue)
		setDefault(configValue.Field(index), values.GetString(key))

		envKey := field.Tag.Get("env")
		if envKey == "" {
			continue
		}
		envValue := getEnv(envKey, "")
		if envValue == "" {
			continue
		}
		values.Set(key, envValue)
		if !setEnvValue(configValue.Field(index), values.GetString(key)) {
			logger.Warn("Invalid value in env var, using default", "key", envKey)
		}
	}

	fs := flag.NewFlagSet("hitkeep", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	registerFlags(fs, &conf)
	registerCloudFlags(fs, &conf)
	_ = fs.Parse(args)

	if conf.Healthcheck {
		return &conf
	}
	normalizeConfig(&conf, logger)
	return &conf
}
