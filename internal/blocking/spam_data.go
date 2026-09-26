package blocking

import (
	"embed"
	"fmt"

	"hitkeep/internal/blocking/spamfeed"
)

//go:embed default_spam_filter.json
var embeddedSpamDataFS embed.FS

type SpamFeedData = spamfeed.SpamFeedData
type SpamFeedSourceMetadata = spamfeed.SpamFeedSourceMetadata

func LoadEmbeddedSpamFeedData() (SpamFeedData, error) {
	raw, err := embeddedSpamDataFS.ReadFile("default_spam_filter.json")
	if err != nil {
		return SpamFeedData{}, fmt.Errorf("read embedded spam data: %w", err)
	}
	return spamfeed.Decode(raw)
}

func LoadSpamFeedData(path string) (SpamFeedData, error) {
	return spamfeed.LoadSpamFeedData(path)
}

func SaveSpamFeedData(path string, data SpamFeedData) error {
	return spamfeed.SaveSpamFeedData(path, data)
}

func ValidateEmbeddedSpamFeedData(data SpamFeedData) error {
	return spamfeed.ValidateEmbeddedSpamFeedData(data)
}
