package webhookdispatcher

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/config"
	"hitkeep/internal/database"
	"hitkeep/internal/webhooks"
)

const (
	HeaderTimestamp  = "X-HitKeep-Timestamp"
	HeaderSignature  = "X-HitKeep-Signature"
	HeaderEventID    = "X-HitKeep-Event-ID"
	HeaderDeliveryID = "X-HitKeep-Delivery-ID"
)

type Dispatcher struct {
	store      *database.Store
	config     config.Config
	httpClient *http.Client
}

func NewDispatcher(store *database.Store, conf config.Config) *Dispatcher {
	timeout := time.Duration(conf.WebhookDeliveryTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Dispatcher{
		store:  store,
		config: conf,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: webhookTransport(conf.WebhookAllowDevelopmentTargets, net.DefaultResolver),
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (d *Dispatcher) Dispatch(ctx context.Context, deliveryID uuid.UUID) error {
	if d == nil || d.store == nil {
		return fmt.Errorf("webhook dispatcher store is not configured")
	}
	startedAt := time.Now().UTC()
	delivery, err := d.store.ClaimWebhookDelivery(ctx, deliveryID, startedAt)
	if err != nil || delivery == nil {
		return err
	}

	result := database.WebhookAttemptResult{StartedAt: startedAt}
	destination, err := webhooks.ValidateDestination(ctx, delivery.DestinationURL, d.config.WebhookAllowDevelopmentTargets, net.DefaultResolver)
	if err != nil {
		result.ErrorCode = "invalid_destination"
		result.ErrorMessage = "destination failed safety validation"
		return d.finishAttempt(ctx, delivery, result)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destination.String(), strings.NewReader(string(delivery.Payload)))
	if err != nil {
		result.ErrorCode = "request_build_failed"
		result.ErrorMessage = "delivery request could not be created"
		return d.finishAttempt(ctx, delivery, result)
	}
	timestamp := strconv.FormatInt(startedAt.Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "HitKeep-Webhook/"+webhooks.MinorAPIVersion(d.config.Version))
	// Keep the documented HitKeep product spelling in the protocol constants.
	req.Header.Set(HeaderTimestamp, timestamp)                                                        //nolint:canonicalheader
	req.Header.Set(HeaderSignature, signPayload(delivery.SigningSecret, timestamp, delivery.Payload)) //nolint:canonicalheader
	req.Header.Set(HeaderEventID, delivery.EventID.String())                                          //nolint:canonicalheader
	req.Header.Set(HeaderDeliveryID, delivery.ID.String())                                            //nolint:canonicalheader

	resp, err := d.httpClient.Do(req)
	if err != nil {
		result.ErrorCode = "request_failed"
		result.ErrorMessage = "delivery request failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			result.ErrorCode = "timeout"
			result.ErrorMessage = "delivery request timed out"
		}
		return d.finishAttempt(ctx, delivery, result)
	}
	_ = resp.Body.Close()
	result.ResponseStatus = resp.StatusCode
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		result.Status = database.WebhookDeliverySucceeded
		result.CompletedAt = time.Now().UTC()
		return d.store.RecordWebhookDeliveryAttempt(ctx, delivery.ID, result)
	}
	result.ErrorCode = "http_status"
	result.ErrorMessage = fmt.Sprintf("receiver returned HTTP %d", resp.StatusCode)
	return d.finishAttempt(ctx, delivery, result)
}

func (d *Dispatcher) finishAttempt(ctx context.Context, delivery *database.WebhookDeliveryRecord, result database.WebhookAttemptResult) error {
	result.CompletedAt = time.Now().UTC()
	maxAttempts := d.config.WebhookMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 6
	}
	nextAttemptNumber := delivery.AttemptCount + 1
	if nextAttemptNumber >= maxAttempts {
		result.Status = database.WebhookDeliveryFailed
		result.NextAttemptAt = nil
	} else {
		result.Status = database.WebhookDeliveryRetrying
		next := result.CompletedAt.Add(d.retryDelay(nextAttemptNumber))
		result.NextAttemptAt = &next
	}
	return d.store.RecordWebhookDeliveryAttempt(ctx, delivery.ID, result)
}

func RetryDelay(attempt int) time.Duration {
	return boundedRetryDelay(attempt, 30*time.Second, 6*time.Hour)
}

func (d *Dispatcher) retryDelay(attempt int) time.Duration {
	base := time.Duration(d.config.WebhookRetryBaseSeconds) * time.Second
	if base <= 0 {
		base = 30 * time.Second
	}
	maximum := time.Duration(d.config.WebhookRetryMaxSeconds) * time.Second
	if maximum <= 0 {
		maximum = 6 * time.Hour
	}
	if maximum < base {
		maximum = base
	}
	return boundedRetryDelay(attempt, base, maximum)
}

func boundedRetryDelay(attempt int, base, maximum time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for i := 1; i < attempt && delay < maximum; i++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func signPayload(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(payload)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookTransport(allowDevelopmentTargets bool, resolver webhooks.Resolver) http.RoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	if allowDevelopmentTargets {
		return base
	}
	base.DialContext = safeDialContext(resolver)
	return base
}

func safeDialContext(resolver webhooks.Resolver) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("parse webhook destination address: %w", err)
		}
		var addresses []netip.Addr
		if literal, err := netip.ParseAddr(host); err == nil {
			addresses = []netip.Addr{literal}
		} else {
			addresses, err = resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve webhook destination: %w", err)
			}
		}
		if len(addresses) == 0 {
			return nil, fmt.Errorf("webhook destination did not resolve to an IP address")
		}
		for _, resolved := range addresses {
			if !webhooks.AddressAllowed(resolved) {
				return nil, fmt.Errorf("webhook destination resolves to a private or reserved address")
			}
		}
		var lastErr error
		for _, resolved := range addresses {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}
