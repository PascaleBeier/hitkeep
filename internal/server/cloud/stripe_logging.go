//go:build billing

package cloud

import (
	"context"
	"errors"
	"strings"

	stripe "github.com/stripe/stripe-go/v86"
)

type stripeErrorLogDetails struct {
	Kind       string
	ErrorType  string
	ErrorCode  string
	HTTPStatus int
}

func describeStripeErrorForLog(err error) stripeErrorLogDetails {
	details := stripeErrorLogDetails{Kind: "unknown"}
	if err == nil {
		return details
	}

	switch {
	case errors.Is(err, context.Canceled):
		details.Kind = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		details.Kind = "timeout"
	}

	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) || stripeErr == nil {
		return details
	}

	if details.Kind == "unknown" {
		details.Kind = "provider"
	}
	details.ErrorType = safeStripeLogValue(string(stripeErr.Type))
	details.ErrorCode = safeStripeLogValue(string(stripeErr.Code))
	details.HTTPStatus = stripeErr.HTTPStatusCode
	return details
}

func stripeErrorLogAttrs(err error) []any {
	details := describeStripeErrorForLog(err)
	attrs := []any{
		"error_code", "stripe_request_failed",
		"error_kind", details.Kind,
	}
	if details.ErrorType != "" {
		attrs = append(attrs, "stripe_error_type", details.ErrorType)
	}
	if details.ErrorCode != "" {
		attrs = append(attrs, "stripe_error_code", details.ErrorCode)
	}
	if details.HTTPStatus > 0 {
		attrs = append(attrs, "stripe_http_status", details.HTTPStatus)
	}
	return attrs
}

func safeStripeLogValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return ""
		}
	}
	return value
}
