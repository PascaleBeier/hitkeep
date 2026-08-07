package mailer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

// SendStage identifies the part of message delivery that returned an error.
// The values are intentionally stable because they are emitted in operator
// logs and are useful for filtering delivery incidents.
type SendStage string

const (
	SendStageHTMLTemplateParse   SendStage = "html_template_parse"
	SendStageHTMLTemplateExecute SendStage = "html_template_execute"
	SendStageMJMLRender          SendStage = "mjml_render"
	SendStageTextTemplateParse   SendStage = "text_template_parse"
	SendStageTextTemplateExecute SendStage = "text_template_execute"
	SendStageTransport           SendStage = "transport"
	SendStageConfiguration       SendStage = "configuration"
)

// SendError preserves the original error while recording the delivery stage
// that failed. Callers can continue to use errors.Is/errors.As on the wrapped
// cause.
type SendError struct {
	Stage SendStage
	Err   error
}

func (e *SendError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return string(e.Stage)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e *SendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrapSendError(stage SendStage, err error) error {
	if err == nil {
		return nil
	}
	return &SendError{Stage: stage, Err: err}
}

// ErrorDetails is the secret-free diagnostic representation of a mail send
// error. Message is redacted and bounded before it is suitable for logs.
type ErrorDetails struct {
	Stage    string
	Kind     string
	Message  string
	SMTPCode string
}

var (
	mailAddressPattern    = regexp.MustCompile(`(?i)\b[A-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?(?:\.[A-Z0-9](?:[A-Z0-9-]{0,61}[A-Z0-9])?)+\b`)
	mailURLPattern        = regexp.MustCompile(`(?i)\b(?:https?|smtps?)://[^\s<>]+`)
	mailCredentialPattern = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key|authorization|credential)(\s*[:=]\s*|\s+)[^\s,;]+`)
	smtpResponsePattern   = regexp.MustCompile(`\b([245]\d{2})\b`)
)

// DescribeError returns stable, redacted fields for structured mail failure
// logs. It deliberately does not expose the recipient, message body, URLs, or
// arbitrary provider error text without redaction and truncation.
func DescribeError(err error) ErrorDetails {
	if err == nil {
		return ErrorDetails{}
	}

	details := ErrorDetails{Stage: "unknown", Kind: "unknown"}
	if errors.Is(err, ErrMailerDisabled) {
		details.Stage = string(SendStageConfiguration)
		details.Kind = "disabled"
		details.Message = "mail delivery is disabled"
		return details
	}

	var sendErr *SendError
	if errors.As(err, &sendErr) && sendErr != nil && sendErr.Stage != "" {
		details.Stage = string(sendErr.Stage)
	}
	if isRenderStage(details.Stage) {
		// Template and MJML errors can include rendered content or user-provided
		// report data. Keep the stage, but never put that payload in logs.
		details.Message = "mail content rendering failed"
	} else {
		details.Message = safeErrorMessage(err)
	}
	details.SMTPCode = smtpResponseCode(details.Message)

	switch {
	case details.SMTPCode != "":
		if details.SMTPCode[0] == '4' {
			details.Kind = "temporary_rejection"
		} else {
			details.Kind = "permanent_rejection"
		}
	case errors.Is(err, context.Canceled):
		details.Kind = "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		details.Kind = "timeout"
	default:
		var networkErr net.Error
		if errors.As(err, &networkErr) {
			if networkErr.Timeout() {
				details.Kind = "timeout"
			} else {
				details.Kind = "network"
			}
		} else if details.Stage == string(SendStageTransport) {
			details.Kind = "transport"
		} else if isRenderStage(details.Stage) {
			details.Kind = "render"
		}
	}

	return details
}

func isRenderStage(stage string) bool {
	switch SendStage(stage) {
	case SendStageHTMLTemplateParse, SendStageHTMLTemplateExecute, SendStageMJMLRender, SendStageTextTemplateParse, SendStageTextTemplateExecute:
		return true
	default:
		return false
	}
}

func safeErrorMessage(err error) string {
	message := strings.Join(strings.Fields(err.Error()), " ")
	message = mailURLPattern.ReplaceAllString(message, "[redacted-url]")
	message = mailAddressPattern.ReplaceAllString(message, "[redacted-email]")
	message = mailCredentialPattern.ReplaceAllString(message, "$1=[redacted]")
	if message == "" {
		return "mail send failed"
	}

	const maxMessageRunes = 512
	runes := []rune(message)
	if len(runes) > maxMessageRunes {
		return string(runes[:maxMessageRunes]) + "…"
	}
	return message
}

func smtpResponseCode(message string) string {
	match := smtpResponsePattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}
