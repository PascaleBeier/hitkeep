package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// LoadArgs assembles runtime configuration from explicit arguments, the process
// environment, and an optional configuration file read through fsys. It never
// reads os.Args.
//
// It applies the 2.x precedence: changed flag, environment, explicit file,
// catalog default. Viper owns defaults, file, and environment; pflag owns flags
// and writes typed values last so the legacy last-occurrence rule holds.
func LoadArgs(fsys afero.Fs, args []string, configFile string, logger *slog.Logger) (*Config, error) {
	if logger == nil {
		logger = slog.Default()
	}
	var conf Config
	fields := reflect.ValueOf(&conf).Elem()
	settings := activeSettings()
	values := viper.New()
	for _, setting := range settings {
		values.SetDefault(setting.ConfigFileKey, setting.Default)
	}

	if configFile != "" {
		contents, err := afero.ReadFile(fsys, configFile)
		if err != nil {
			return nil, fmt.Errorf("read configuration file %q: %w", configFile, err)
		}
		if err := validateRawConfigFileKeys(contents, settings); err != nil {
			return nil, err
		}
		values.SetConfigType("yaml")
		if err := values.ReadConfig(bytes.NewReader(contents)); err != nil {
			return nil, fmt.Errorf("parse configuration file %q: invalid YAML", configFile)
		}
		for _, setting := range settings {
			if values.InConfig(setting.ConfigFileKey) && !validValue(fields.FieldByName(setting.Field), values.GetString(setting.ConfigFileKey)) {
				return nil, fmt.Errorf("configuration key %q has invalid type", setting.ConfigFileKey)
			}
		}
	}

	for _, setting := range settings {
		// The first non-empty name wins; an invalid value keeps the file or default.
		for _, name := range append([]string{setting.Environment}, setting.DeprecatedEnvironments...) {
			value := os.Getenv(name)
			if name == "" || value == "" {
				continue
			}
			if validValue(fields.FieldByName(setting.Field), value) {
				_ = values.BindEnv(setting.ConfigFileKey, name)
			} else {
				logger.Warn("Invalid value in env var, using default", "key", setting.Environment)
			}
			break
		}
	}

	for _, setting := range settings {
		setField(fields.FieldByName(setting.Field), values.GetString(setting.ConfigFileKey))
	}

	flags := newFlagSet(fields, settings)
	if err := flags.Parse(legacyFlagArgs(flags, args)); err != nil && !errors.Is(err, pflag.ErrHelp) {
		// The 2.x parser reported a bad flag, kept earlier flags, and continued.
		fmt.Fprintln(flags.Output(), err)
		fmt.Fprintf(flags.Output(), "Usage of %s:\n", flags.Name())
		flags.PrintDefaults()
	}

	if conf.Healthcheck {
		return &conf, nil
	}
	normalizeConfig(&conf, logger)
	return &conf, nil
}

func activeSettings() []ConfigurationSetting {
	settings := Catalog().Settings
	if includeCloudConfigFields() {
		return settings
	}
	return slices.DeleteFunc(settings, func(setting ConfigurationSetting) bool { return setting.CloudOnly })
}

// newFlagSet binds every setting's flag to its typed field. Deprecated names are
// normalized onto the canonical flag, so the last occurrence of either wins.
func newFlagSet(fields reflect.Value, settings []ConfigurationSetting) *pflag.FlagSet {
	flags := pflag.NewFlagSet("hitkeep", pflag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.SetInterspersed(false)
	aliases := make(map[string]string)
	for _, setting := range settings {
		field := fields.FieldByName(setting.Field)
		switch pointer := field.Addr().Interface().(type) {
		case *string:
			flags.StringVar(pointer, setting.Flag, *pointer, setting.Description)
		case *int:
			flags.IntVar(pointer, setting.Flag, *pointer, setting.Description)
		case *bool:
			flags.BoolVar(pointer, setting.Flag, *pointer, setting.Description)
		case *float64:
			flags.Float64Var(pointer, setting.Flag, *pointer, setting.Description)
		}
		for _, alias := range setting.DeprecatedFlags {
			aliases[alias] = setting.Flag
		}
	}
	flags.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if canonical, ok := aliases[name]; ok {
			return pflag.NormalizedName(canonical)
		}
		return pflag.NormalizedName(name)
	})
	return flags
}

// legacyFlagArgs rewrites the 2.x single-dash long form (-db x, -db=x) to the
// pflag form. Like the 2.x parser it stops at the first non-flag or unknown flag.
func legacyFlagArgs(flags *pflag.FlagSet, args []string) []string {
	rewritten := append([]string(nil), args...)
	for index := 0; index < len(rewritten); index++ {
		arg := rewritten[index]
		if len(arg) < 2 || arg[0] != '-' || arg == "--" || strings.HasPrefix(arg, "---") {
			break
		}
		name, _, hasValue := strings.Cut(strings.TrimPrefix(arg[1:], "-"), "=")
		flag := flags.Lookup(name)
		if flag == nil {
			break
		}
		if arg[1] != '-' {
			rewritten[index] = "-" + arg
		}
		if !hasValue && flag.Value.Type() != "bool" {
			index++
		}
	}
	return rewritten
}

// setField stores a configuration string in a supported scalar field. Integers
// keep the 2.x base-10 environment grammar.
func setField(field reflect.Value, value string) bool {
	switch field.Kind() { //nolint:exhaustive // Config supports only these scalar kinds
	case reflect.String:
		field.SetString(value)
	case reflect.Int:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return false
		}
		field.SetInt(int64(parsed))
	case reflect.Bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false
		}
		field.SetBool(parsed)
	case reflect.Float64:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return false
		}
		field.SetFloat(parsed)
	default:
		return false
	}
	return true
}

func validValue(field reflect.Value, value string) bool {
	return setField(reflect.New(field.Type()).Elem(), value)
}

func validateRawConfigFileKeys(contents []byte, settings []ConfigurationSetting) error {
	known := make(map[string]struct{}, len(settings))
	for _, setting := range settings {
		known[setting.ConfigFileKey] = struct{}{}
	}
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
		if _, ok := known[key.Value]; !ok {
			return fmt.Errorf("unknown configuration key %q", key.Value)
		}
		if value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			return fmt.Errorf("configuration value for %q must be a scalar", key.Value)
		}
	}
	return nil
}
