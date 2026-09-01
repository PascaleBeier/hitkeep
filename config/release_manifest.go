package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ConfigurationReleaseManifestSchemaVersion = "hitkeep.config-release/v1"
	ConfigurationReleaseManifestFilename      = "hitkeep-configuration-manifest.json"
	ConfigurationCatalogFilename              = "hitkeep-configuration.json"
	ConfigurationExampleFilename              = "hitkeep.example.yaml"
)

// ConfigurationReleaseManifest records the immutable configuration artifacts
// shipped with a release. Its subject order is part of the byte contract.
type ConfigurationReleaseManifest struct {
	SchemaVersion string                                `json:"schema_version"`
	Subjects      []ConfigurationReleaseManifestSubject `json:"subjects"`
}

type ConfigurationReleaseManifestSubject struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// ConfigurationReleaseManifestFor creates the release contract for the exact
// catalog and example bytes that will be distributed.
func ConfigurationReleaseManifestFor(catalog, example []byte) ConfigurationReleaseManifest {
	return ConfigurationReleaseManifest{
		SchemaVersion: ConfigurationReleaseManifestSchemaVersion,
		Subjects: []ConfigurationReleaseManifestSubject{
			{Name: ConfigurationCatalogFilename, SHA256: digestSHA256(catalog)},
			{Name: ConfigurationExampleFilename, SHA256: digestSHA256(example)},
		},
	}
}

// RenderConfigurationReleaseManifest renders the release contract in its
// stable on-disk JSON form.
func RenderConfigurationReleaseManifest(catalog, example []byte) []byte {
	raw, err := json.Marshal(ConfigurationReleaseManifestFor(catalog, example))
	if err != nil {
		panic(err)
	}
	return append(raw, '\n')
}

// ValidateConfigurationReleaseManifest proves that a manifest names exactly
// the catalog and example bytes supplied by the release assembler.
func ValidateConfigurationReleaseManifest(raw, catalog, example []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest ConfigurationReleaseManifest
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode configuration release manifest: %w", err)
	}
	if err := decoder.Decode(new(struct{})); !errors.Is(err, io.EOF) {
		return errors.New("configuration release manifest must contain one JSON value")
	}
	if manifest.SchemaVersion != ConfigurationReleaseManifestSchemaVersion {
		return fmt.Errorf("unsupported configuration release manifest schema %q", manifest.SchemaVersion)
	}
	expected := []ConfigurationReleaseManifestSubject{
		{Name: ConfigurationCatalogFilename, SHA256: digestSHA256(catalog)},
		{Name: ConfigurationExampleFilename, SHA256: digestSHA256(example)},
	}
	if len(manifest.Subjects) != len(expected) {
		return fmt.Errorf("configuration release manifest subjects = %d, want %d", len(manifest.Subjects), len(expected))
	}
	for index, want := range expected {
		got := manifest.Subjects[index]
		if got.Name != want.Name {
			return fmt.Errorf("configuration release manifest subject %d = %q, want %q", index, got.Name, want.Name)
		}
		if got.SHA256 != want.SHA256 {
			return fmt.Errorf("configuration release manifest digest for %s does not match", want.Name)
		}
	}
	return nil
}

func digestSHA256(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}
