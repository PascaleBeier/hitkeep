package blocking

import (
	"context"
	"log/slog"
	"net/http"

	"hitkeep/internal/blocking/spamfeed"
)

func FetchSpamFeedData(ctx context.Context, client interface {
	Do(*http.Request) (*http.Response, error)
}, logger *slog.Logger) (SpamFeedData, error) {
	return spamfeed.FetchSpamFeedData(ctx, client, logger)
}
