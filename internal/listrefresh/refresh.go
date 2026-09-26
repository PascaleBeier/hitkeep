package listrefresh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"time"

	"hitkeep/internal/aianalytics"
	"hitkeep/internal/blocking/spamfeed"
)

func RunAI(ctx context.Context, outputPath string, logger *slog.Logger) (aianalytics.AIAgentData, bool, error) {
	return run(ctx, outputPath,
		func(ctx context.Context) (aianalytics.AIAgentData, error) {
			return aianalytics.FetchAIAgentData(ctx, nil, logger)
		},
		aianalytics.ValidateEmbeddedAIAgentData, aianalytics.LoadAIAgentData, aianalytics.SaveAIAgentData, sameAI,
		"could not fetch AI agent lists", "refusing to write incomplete embedded AI agent data", "could not write AI agent data")
}

func RunSpam(ctx context.Context, outputPath string, logger *slog.Logger) (spamfeed.SpamFeedData, bool, error) {
	return run(ctx, outputPath,
		func(ctx context.Context) (spamfeed.SpamFeedData, error) {
			return spamfeed.FetchSpamFeedData(ctx, nil, logger)
		},
		spamfeed.ValidateEmbeddedSpamFeedData, spamfeed.LoadSpamFeedData, spamfeed.SaveSpamFeedData, sameSpam,
		"could not fetch spam feeds", "refusing to write incomplete embedded spam data", "could not write spam cache")
}

func run[T any](ctx context.Context, outputPath string,
	fetch func(context.Context) (T, error), validate func(T) error,
	load func(string) (T, error), save func(string, T) error, same func(T, T) bool,
	fetchError, validationError, saveError string,
) (T, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	data, err := fetch(ctx)
	if err != nil {
		return data, false, fmt.Errorf("%s: %w", fetchError, err)
	}
	if err := ctx.Err(); err != nil {
		return data, false, err
	}
	if err := validate(data); err != nil {
		return data, false, fmt.Errorf("%s: %w", validationError, err)
	}

	old, err := load(outputPath)
	if err == nil {
		if same(old, data) {
			if err := ctx.Err(); err != nil {
				return data, false, err
			}
			return data, false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return data, false, fmt.Errorf("could not read existing list %s: %w", outputPath, err)
	} else if _, statErr := os.Lstat(outputPath); statErr == nil {
		return data, false, fmt.Errorf("could not read existing list %s: %w", outputPath, err)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return data, false, fmt.Errorf("could not inspect existing list %s: %w", outputPath, statErr)
	}
	if err := ctx.Err(); err != nil {
		return data, false, err
	}
	if err := save(outputPath, data); err != nil {
		return data, false, fmt.Errorf("%s: %w", saveError, err)
	}
	return data, true, nil
}

func sameAI(a, b aianalytics.AIAgentData) bool {
	a.GeneratedAt, b.GeneratedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

func sameSpam(a, b spamfeed.SpamFeedData) bool {
	a.GeneratedAt, b.GeneratedAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}
