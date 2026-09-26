package config

import (
	"flag"
	"log/slog"
	"os"
	"reflect"
)

// loadLegacy preserves the legacy parser as the reference for migration parity tests.
// Production configuration is assembled by LoadArgs.
func loadLegacy(args []string, getEnv func(string, string) string, loggerArgs ...*slog.Logger) *Config {
	logger := slog.Default()
	if len(loggerArgs) > 0 && loggerArgs[0] != nil {
		logger = loggerArgs[0]
	}
	var conf Config
	loadEnvDefaults(&conf)
	loadEnvOverrides(&conf, getEnv, logger)

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

func registerFlags(fs *flag.FlagSet, conf *Config) {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("cloud") == "true" {
			continue
		}
		registerOneField(fs, v.Field(i), f)
	}
}

func registerOneField(fs *flag.FlagSet, fv reflect.Value, sf reflect.StructField) {
	fname := sf.Tag.Get("flag")
	if fname == "" {
		env := sf.Tag.Get("env")
		if env == "" {
			return
		}
		fname = flagName(env)
	}
	desc := sf.Tag.Get("desc")

	if dep := sf.Tag.Get("deprecated"); dep != "" {
		registerFlagVar(fs, fv, dep, "(deprecated, use --"+fname+")")
	}
	registerFlagVar(fs, fv, fname, desc)
}

func registerCloudFlags(fs *flag.FlagSet, conf *Config) {
	if !includeCloudConfigFields() {
		return
	}
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if f.Tag.Get("cloud") != "true" {
			continue
		}
		registerOneField(fs, v.Field(i), f)
	}
}

func loadEnvDefaults(conf *Config) {
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !shouldLoadEnvField(f) {
			continue
		}
		def := f.Tag.Get("default")
		if def == "" {
			continue
		}
		fv := v.Field(i)
		setDefault(fv, def)
	}
}

func loadEnvOverrides(conf *Config, getEnv func(string, string) string, loggerArgs ...*slog.Logger) {
	logger := slog.Default()
	if len(loggerArgs) > 0 && loggerArgs[0] != nil {
		logger = loggerArgs[0]
	}
	v := reflect.ValueOf(conf).Elem()
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !shouldLoadEnvField(f) {
			continue
		}
		env := f.Tag.Get("env")
		if env == "" {
			continue
		}
		val := getEnv(env, "")
		if val == "" {
			continue
		}
		fv := v.Field(i)
		if !setEnvValue(fv, val) {
			logger.Warn("Invalid value in env var, using default", "key", env)
		}
	}
}

func shouldLoadEnvField(f reflect.StructField) bool {
	if !f.IsExported() {
		return false
	}
	if f.Tag.Get("cloud") == "true" {
		return includeCloudConfigFields()
	}
	return true
}
