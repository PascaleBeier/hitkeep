package blocking

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultSpamRefreshInterval = 24 * time.Hour

type SpamDecision struct {
	Blocked bool
	Reason  string
}

type SpamFilter struct {
	path       string
	logger     *slog.Logger
	fetch      func(context.Context) (SpamFeedData, error)
	transition chan struct{}

	mu sync.RWMutex

	data          SpamFeedData
	referrerHosts map[string]struct{}
	networks      []netip.Prefix
}

func NewSpamFilter(path string, logger *slog.Logger) *SpamFilter {
	if logger == nil {
		panic("blocking: logger is required")
	}
	filter := &SpamFilter{
		path:       path,
		logger:     logger,
		transition: make(chan struct{}, 1),
	}
	filter.transition <- struct{}{}
	if embedded, err := LoadEmbeddedSpamFeedData(); err == nil {
		filter.apply(embedded)
	}
	return filter
}

func (f *SpamFilter) acquireTransition(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.transition:
		return nil
	}
}

func (f *SpamFilter) RefreshFromDisk() error {
	return f.refreshFromDisk(context.Background())
}

func (f *SpamFilter) refreshFromDisk(ctx context.Context) error {
	if strings.TrimSpace(f.path) == "" {
		return nil
	}
	if err := f.acquireTransition(ctx); err != nil {
		return err
	}
	defer func() { f.transition <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}

	data, err := LoadSpamFeedData(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	f.apply(data)
	return nil
}

func (f *SpamFilter) StartRefreshLoop(ctx context.Context, autoUpdate bool, interval time.Duration, isLeader func() bool) {
	if !autoUpdate {
		return
	}
	if interval <= 0 {
		interval = defaultSpamRefreshInterval
	}

	go func() {
		if ctx.Err() != nil {
			return
		}
		if isLeader() {
			if err := f.Update(ctx); err != nil && !errors.Is(err, context.Canceled) {
				f.logger.Warn("Failed initial spam filter update", "error", err)
			}
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if isLeader() {
					if err := f.Update(ctx); err != nil && !errors.Is(err, context.Canceled) {
						f.logger.Warn("Failed to refresh spam filter feeds", "error", err)
					}
				} else if err := f.refreshFromDisk(ctx); err != nil && !errors.Is(err, context.Canceled) {
					f.logger.Warn("Failed to reload spam filter cache from disk", "error", err, "path", f.path)
				}
			}
		}
	}()
}

func (f *SpamFilter) Update(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := f.acquireTransition(ctx); err != nil {
		return err
	}
	defer func() { f.transition <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}

	fetch := f.fetch
	if fetch == nil {
		fetch = func(ctx context.Context) (SpamFeedData, error) {
			return FetchSpamFeedData(ctx, nil, f.logger)
		}
	}
	data, err := fetch(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return f.saveAndApply(ctx, data)
}

func (f *SpamFilter) saveAndApply(ctx context.Context, data SpamFeedData) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	data.normalize()
	if strings.TrimSpace(f.path) != "" {
		if err := SaveSpamFeedData(f.path, data); err != nil {
			return err
		}
	}
	f.apply(data)
	return nil
}

func (f *SpamFilter) Evaluate(siteDomain, userIP string, referrer *string) SpamDecision {
	if f.isBlockedIP(userIP) {
		return SpamDecision{Blocked: true, Reason: "spamhaus_drop"}
	}

	referrerHost := normalizeReferrerHost(referrer)
	if referrerHost == "" || isSameSiteHost(referrerHost, siteDomain) {
		return SpamDecision{}
	}

	f.mu.RLock()
	_, blocked := f.referrerHosts[referrerHost]
	f.mu.RUnlock()
	if blocked {
		return SpamDecision{Blocked: true, Reason: "matomo_referrer_spam"}
	}

	return SpamDecision{}
}

func (f *SpamFilter) isBlockedIP(value string) bool {
	addr := parseIP(value)
	if !addr.IsValid() {
		return false
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, prefix := range f.networks {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func (f *SpamFilter) RuleCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.referrerHosts) + len(f.networks)
}

func (f *SpamFilter) LastRefresh() *time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	if f.data.GeneratedAt.IsZero() {
		return nil
	}
	lastRefresh := f.data.GeneratedAt
	return &lastRefresh
}

func (f *SpamFilter) apply(data SpamFeedData) {
	referrerHosts := make(map[string]struct{}, len(data.ReferrerHostDenylist))
	for _, host := range data.ReferrerHostDenylist {
		referrerHosts[host] = struct{}{}
	}

	networks := make([]netip.Prefix, 0, len(data.NetworkDenylist))
	for _, cidr := range data.NetworkDenylist {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			f.logger.Warn("Skipping invalid spam network prefix", "cidr", cidr, "error", err)
			continue
		}
		networks = append(networks, prefix.Masked())
	}

	f.mu.Lock()
	f.data = data
	f.referrerHosts = referrerHosts
	f.networks = networks
	f.mu.Unlock()
}

func normalizeReferrerHost(referrer *string) string {
	if referrer == nil {
		return ""
	}
	value := strings.TrimSpace(strings.ToLower(*referrer))
	if value == "" {
		return ""
	}

	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		u, err := url.Parse(value)
		if err == nil {
			return stripWWW(strings.ToLower(u.Hostname()))
		}
	}

	return stripWWW(strings.Trim(value, "/"))
}

func normalizeHostname(value string) string {
	return stripWWW(strings.ToLower(strings.TrimSpace(value)))
}

func stripWWW(value string) string {
	return strings.TrimPrefix(value, "www.")
}

func isSameSiteHost(referrerHost, siteDomain string) bool {
	normalizedSite := normalizeHostname(siteDomain)
	if normalizedSite == "" || referrerHost == "" {
		return false
	}
	return referrerHost == normalizedSite || strings.HasSuffix(referrerHost, "."+normalizedSite)
}
