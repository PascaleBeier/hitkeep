package config

import (
	"strconv"
	"strings"
)

// RenderExampleYAML returns the checked-in self-hosted configuration example.
func RenderExampleYAML() []byte {
	return renderExampleYAML(Catalog())
}

func renderExampleYAML(catalog ConfigurationCatalog) []byte {
	var output strings.Builder
	output.WriteString("# HitKeep self-hosted configuration.\n")
	output.WriteString("# Generated from ")
	output.WriteString(catalog.SchemaVersion)
	output.WriteString("; environment variables and flags remain supported.\n")

	category := ""
	for _, setting := range catalog.Settings {
		if setting.CloudOnly {
			continue
		}
		if setting.Category != category {
			category = setting.Category
			output.WriteString("\n# ")
			output.WriteString(category)
			output.WriteString(" settings\n")
		}

		inactive := setting.Sensitive != "" || setting.DisplayDefault != ""
		defaultValue := setting.Default
		if setting.Sensitive != "" {
			defaultValue = "unset"
		} else if setting.DisplayDefault != "" {
			defaultValue = setting.DisplayDefault
		}
		output.WriteString("# ")
		output.WriteString(strings.ReplaceAll(setting.Description, "\n", " "))
		output.WriteString(" (type: ")
		output.WriteString(setting.Type)
		output.WriteString("; default: ")
		output.WriteString(strconv.Quote(defaultValue))
		if setting.Sensitive != "" {
			output.WriteString("; sensitive")
		}
		if setting.DisplayDefault != "" {
			output.WriteString("; derived")
		}
		output.WriteString(")\n")
		if inactive {
			output.WriteString("# ")
		}
		output.WriteString(setting.ConfigFileKey)
		output.WriteString(":")
		if !inactive {
			output.WriteString(" ")
			output.WriteString(yamlDefault(setting))
		}
		output.WriteString("\n")
	}
	return []byte(output.String())
}

func yamlDefault(setting ConfigurationSetting) string {
	if setting.Type == "string" {
		return strconv.Quote(setting.Default)
	}
	return setting.Default
}
