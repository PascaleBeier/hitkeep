package drivers

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/wneessen/go-mail"

	"hitkeep/config"
)

type SMTPDriver struct {
	client *mail.Client
	from   string
	name   string
}

func NewSMTPDriver(conf *config.Config) (*SMTPDriver, error) {
	opts := []mail.Option{
		mail.WithPort(conf.MailPort),
		mail.WithTimeout(10 * time.Second),
		mail.WithHELO(heloNameFromPublicURL(conf.PublicURL)),
	}

	if conf.MailUsername != "" {
		opts = append(opts, mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(conf.MailUsername),
			mail.WithPassword(conf.MailPassword),
		)
	}

	switch conf.MailEncryption {
	case "ssl":
		opts = append(opts, mail.WithSSL())
	case "none":
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	case "tls":
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.DefaultTLSPolicy))
	}

	if conf.MailInsecureSkipVerify {
		//nolint:gosec // user asked to
		opts = append(opts, mail.WithTLSConfig(&tls.Config{InsecureSkipVerify: true}))
	}

	client, err := mail.NewClient(conf.MailHost, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create smtp client: %w", err)
	}

	return &SMTPDriver{
		client: client,
		from:   conf.MailFromAddress,
		name:   conf.MailFromName,
	}, nil
}

func heloNameFromPublicURL(publicURL string) string {
	publicURL = strings.TrimSpace(publicURL)
	if publicURL == "" {
		return "localhost"
	}

	if parsed, err := url.Parse(publicURL); err == nil {
		if hostname := parsed.Hostname(); hostname != "" {
			return hostname
		}
	}

	if host, _, err := net.SplitHostPort(publicURL); err == nil && host != "" {
		return strings.Trim(host, "[]")
	}

	if parsed, err := url.Parse("http://" + publicURL); err == nil {
		if hostname := parsed.Hostname(); hostname != "" {
			return hostname
		}
	}

	return publicURL
}

// Message is one rendered email ready for transport.
type Message struct {
	To        []string
	Subject   string
	HTML      string
	Text      string
	MessageID string
	Headers   map[string]string
	// Inline images are embedded in the HTML part and referenced as cid:<CID>.
	Inline []InlineImage
}

// InlineImage is an image embedded in the message instead of fetched remotely.
type InlineImage struct {
	CID         string
	ContentType string
	Data        []byte
}

func (s *SMTPDriver) Send(message Message) error {
	msg, err := s.buildMessage(message)
	if err != nil {
		return err
	}

	return s.client.DialAndSend(msg)
}

func (s *SMTPDriver) buildMessage(message Message) (*mail.Msg, error) {
	msg := mail.NewMsg()
	if err := msg.FromFormat(s.name, s.from); err != nil {
		return nil, err
	}
	if err := msg.To(message.To...); err != nil {
		return nil, err
	}

	msg.Subject(message.Subject)
	if strings.TrimSpace(message.MessageID) != "" {
		msg.SetGenHeader(mail.HeaderMessageID, strings.TrimSpace(message.MessageID))
	}
	for key, value := range message.Headers {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		msg.SetGenHeader(mail.Header(key), value)
	}
	msg.SetBodyString(mail.TypeTextPlain, message.Text)
	msg.AddAlternativeString(mail.TypeTextHTML, message.HTML)
	for _, image := range message.Inline {
		if err := msg.EmbedReader(image.CID, bytes.NewReader(image.Data), mail.WithFileContentID("<"+image.CID+">"), mail.WithFileContentType(mail.ContentType(image.ContentType))); err != nil {
			return nil, err
		}
	}

	return msg, nil
}

func (s *SMTPDriver) Close() error {
	return s.client.Close()
}
