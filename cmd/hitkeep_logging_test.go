package hitkeepcmd

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"hitkeep/config"
)

func TestLogMailerConfigurationErrorDoesNotLogRawConfiguration(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	conf := &config.Config{
		MailDriver:      "smtp-secret-driver",
		MailHost:        "smtp.secret.example",
		MailUsername:    "secret-user",
		MailPassword:    "secret-password",
		MailFromAddress: "secret@example.com",
	}

	logMailerConfigurationError(logger, conf)

	output := logs.String()
	for _, secret := range []string{
		conf.MailDriver,
		conf.MailHost,
		conf.MailUsername,
		conf.MailPassword,
		conf.MailFromAddress,
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("raw mail configuration value %q leaked into logs: %s", secret, output)
		}
	}
	for _, field := range []string{
		"error_kind=configuration",
		"mail.driver_kind=unsupported",
		"mail.host_configured=true",
		"mail.credentials_configured=true",
		"mail.from_address_configured=true",
	} {
		if !strings.Contains(output, field) {
			t.Fatalf("expected %s in mail diagnostics, got: %s", field, output)
		}
	}
}
