package config

import (
	"reflect"
	"testing"
)

func TestConfigurationCatalogClassifiesEveryActiveDescriptor(t *testing.T) {
	for _, setting := range Catalog().Settings {
		if setting.Publication == ConfigurationPublicationUnclassified {
			t.Errorf("%s (%s) has no publication classification", setting.Field, setting.Environment)
		}
	}
}

func TestConfigurationPublicationClassifiesUnknownDescriptorAsUnclassified(t *testing.T) {
	if got := configurationPublication(reflect.StructField{Name: "Unclassified"}); got != ConfigurationPublicationUnclassified {
		t.Errorf("configurationPublication() = %q, want %q", got, ConfigurationPublicationUnclassified)
	}
}
