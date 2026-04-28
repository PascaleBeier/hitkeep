//go:build billing

package config

import (
	"flag"
	"reflect"
)

func registerCloudFlags(fs *flag.FlagSet, conf *Config) {
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
