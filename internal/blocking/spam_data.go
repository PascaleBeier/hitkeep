package blocking

import (
	"embed"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	json "hitkeep/jsonapi"
)

//go:embed default_spam_filter.json
var embeddedSpamDataFS embed.FS

const (
	matomoReferrerSpamSource = "matomo_referrer_spam_list"
	spamhausDropSource       = "spamhaus_drop"
	spamhausDropV6Source     = "spamhaus_dropv6"
)

type SpamFeedSourceMetadata struct {
	Timestamp int64  `json:"timestamp,omitempty"`
	Copyright string `json:"copyright,omitempty"`
	Terms     string `json:"terms,omitempty"`
}

type SpamFeedData struct {
	GeneratedAt          time.Time                         `json:"generated_at"`
	Sources              map[string]string                 `json:"sources"`
	SourceMetadata       map[string]SpamFeedSourceMetadata `json:"source_metadata,omitempty"`
	ReferrerHostDenylist []string                          `json:"referrer_host_denylist"`
	NetworkDenylist      []string                          `json:"network_denylist"`
}

func LoadEmbeddedSpamFeedData() (SpamFeedData, error) {
	raw, err := embeddedSpamDataFS.ReadFile("default_spam_filter.json")
	if err != nil {
		return SpamFeedData{}, fmt.Errorf("read embedded spam data: %w", err)
	}
	return decodeSpamFeedData(raw)
}

func LoadSpamFeedData(path string) (SpamFeedData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SpamFeedData{}, err
	}
	return decodeSpamFeedData(raw)
}

func SaveSpamFeedData(path string, data SpamFeedData) error {
	data.normalize()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create spam filter cache dir: %w", err)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal spam feed data: %w", err)
	}
	raw = append(raw, '\n')

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write spam feed data: %w", err)
	}
	return nil
}

func decodeSpamFeedData(raw []byte) (SpamFeedData, error) {
	var data SpamFeedData
	if err := json.Unmarshal(raw, &data); err != nil {
		return SpamFeedData{}, fmt.Errorf("decode spam feed data: %w", err)
	}
	data.normalize()
	return data, nil
}

// ValidateEmbeddedSpamFeedData verifies the stricter completeness contract for
// the generated fallback bundled with HitKeep. Runtime refreshes intentionally
// remain tolerant of individual feed failures.
func ValidateEmbeddedSpamFeedData(data SpamFeedData) error {
	if data.GeneratedAt.IsZero() {
		return fmt.Errorf("embedded spam data is missing its generation timestamp")
	}
	for _, source := range []string{matomoReferrerSpamSource, spamhausDropSource, spamhausDropV6Source} {
		if strings.TrimSpace(data.Sources[source]) == "" {
			return fmt.Errorf("embedded spam data is missing source %q", source)
		}
	}
	if len(data.ReferrerHostDenylist) == 0 {
		return fmt.Errorf("embedded spam data has no referrer hosts")
	}

	hasIPv4 := false
	hasIPv6 := false
	for _, cidr := range data.NetworkDenylist {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return fmt.Errorf("embedded spam data contains invalid network %q: %w", cidr, err)
		}
		if prefix.Addr().Is4() {
			hasIPv4 = true
		} else {
			hasIPv6 = true
		}
	}
	if !hasIPv4 {
		return fmt.Errorf("embedded spam data has no IPv4 networks")
	}
	if !hasIPv6 {
		return fmt.Errorf("embedded spam data has no IPv6 networks")
	}

	for _, source := range []string{spamhausDropSource, spamhausDropV6Source} {
		metadata, ok := data.SourceMetadata[source]
		if !ok {
			return fmt.Errorf("embedded spam data is missing metadata for source %q", source)
		}
		if metadata.Timestamp <= 0 {
			return fmt.Errorf("embedded spam data source %q has no timestamp", source)
		}
		if strings.TrimSpace(metadata.Copyright) == "" {
			return fmt.Errorf("embedded spam data source %q has no copyright", source)
		}
		if strings.TrimSpace(metadata.Terms) == "" {
			return fmt.Errorf("embedded spam data source %q has no terms", source)
		}
	}
	return nil
}

func (d *SpamFeedData) normalize() {
	if d.Sources == nil {
		d.Sources = make(map[string]string)
	}
	d.ReferrerHostDenylist = normalizeStringList(d.ReferrerHostDenylist)
	d.NetworkDenylist = normalizeStringList(d.NetworkDenylist)
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(strings.ToLower(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	slices.Sort(out)
	return out
}
