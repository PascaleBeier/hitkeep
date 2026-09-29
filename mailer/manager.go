package mailer

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	htmltpl "html/template"
	"io/fs"
	"math"
	"path"
	"strings"
	texttpl "text/template"
	"time"
	"unicode/utf8"

	"github.com/Boostport/mjml-go"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"hitkeep/config"
	"hitkeep/mailer/drivers"
)

//go:embed templates/*.mjml templates/*.txt
var templateFS embed.FS

//go:embed assets/brand-icon-light.png
var brandIconLight []byte

//go:embed assets/brand-icon-dark.png
var brandIconDark []byte

// brandImages are embedded in every email so the header icon never loads
// from a remote server (no open tracking, works offline).
var brandImages = []InlineImage{
	{CID: "hitkeep-icon-light", ContentType: "image/png", Data: brandIconLight},
	{CID: "hitkeep-icon-dark", ContentType: "image/png", Data: brandIconDark},
}

// Mailer acts as the Manager.
type Mailer struct {
	driver Driver
	conf   *config.Config
}

var ErrMailerDisabled = errors.New("mailer not configured")

// templateFuncs contains helpers available to all email templates.
func templateFuncsForLocale(locale string) htmltpl.FuncMap {
	printer := message.NewPrinter(language.Make(NormalizeLocale(locale)))
	return htmltpl.FuncMap{
		// percentChange returns a formatted change label like "+12%" or "−3%" or "—".
		"percentChange": func(current, prev int) string {
			if prev == 0 {
				if current == 0 {
					return "—"
				}
				return "+100%"
			}
			pct := (float64(current-prev) / float64(prev)) * 100
			if math.Abs(pct) < 0.05 {
				return "—"
			}
			if pct > 0 {
				return fmt.Sprintf("+%.0f%%", pct)
			}
			return fmt.Sprintf("−%.0f%%", math.Abs(pct))
		},
		// formatDuration formats seconds into "Xm Ys" or "Xs".
		"formatDuration": func(seconds float64) string {
			s := int(seconds)
			if s >= 60 {
				return Translatef(locale, "reports.duration_minutes_seconds", printer.Sprintf("%d", s/60), printer.Sprintf("%d", s%60))
			}
			return Translatef(locale, "reports.duration_seconds", printer.Sprintf("%d", s))
		},
		"formatInt":     func(value int) string { return printer.Sprintf("%d", value) },
		"formatPercent": func(value float64) string { return printer.Sprintf("%.1f%%", value) },
		// mod2 returns i % 2 for alternating row shading.
		"mod2": func(i int) int { return i % 2 },
		"trendBarWidth": func(values []int, index int) int {
			if index < 0 || index >= len(values) {
				return 0
			}
			maxValue := 0
			for _, value := range values {
				if value > maxValue {
					maxValue = value
				}
			}
			if maxValue <= 0 {
				return 0
			}
			return max(2, int(math.Round(float64(values[index])/float64(maxValue)*100)))
		},
		"t": func(key string) string {
			return Translate(locale, key)
		},
		"tf": func(key string, args ...any) string {
			return Translatef(locale, key, args...)
		},
		"roleLabel": func(role string) string {
			return RoleLabel(locale, role)
		},
	}
}

func textTemplateFuncsForLocale(locale string) texttpl.FuncMap {
	htmlFuncs := templateFuncsForLocale(locale)
	return texttpl.FuncMap{
		"percentChange":  htmlFuncs["percentChange"],
		"formatDuration": htmlFuncs["formatDuration"],
		"formatInt":      htmlFuncs["formatInt"],
		"formatPercent":  htmlFuncs["formatPercent"],
		"mod2":           htmlFuncs["mod2"],
		"t":              htmlFuncs["t"],
		"tf":             htmlFuncs["tf"],
		"roleLabel":      htmlFuncs["roleLabel"],
		// rule underlines a plain-text heading to its visible length in any locale.
		"rule": func(char, heading string) string {
			return strings.Repeat(char, utf8.RuneCountInString(heading))
		},
	}
}

type templateContext struct {
	Meta struct {
		Subject string
		Year    int
		Locale  string
	}
	Data any
}

// New creates the mailer and resolves the driver based on config.
func New(conf *config.Config) (*Mailer, error) {
	var driver Driver
	var err error

	switch conf.MailDriver {
	case "smtp":
		driver, err = drivers.NewSMTPDriver(conf)
	default:
		err = fmt.Errorf("mail driver '%s' is not implemented. Available drivers: smtp", conf.MailDriver)
	}

	if err != nil {
		return nil, err
	}

	return &Mailer{
		driver: driver,
		conf:   conf,
	}, nil
}

// NewWithDriver creates a Mailer with the specified driver. This is primarily
// useful for testing where a no-op or mock driver is desired.
func NewWithDriver(driver Driver, conf *config.Config) *Mailer {
	return &Mailer{driver: driver, conf: conf}
}

// Send processes a Mailable (renders MJML) and dispatches via the driver.
// Usage: mailer.Send(user.Email, mailables.NewWelcomeEmail(user))
func (m *Mailer) Send(to string, email Mailable) error {
	return m.SendWithOptions(to, email, SendOptions{})
}

// SendWithOptions renders and sends a mailable with a stable message ID and
// optional RFC headers. Drivers without header support retain legacy behavior.
func (m *Mailer) SendWithOptions(to string, email Mailable, options SendOptions) error {
	if m == nil || m.driver == nil {
		return ErrMailerDisabled
	}

	rendered, err := Render(email, RenderOptions{})
	if err != nil {
		return err
	}

	return wrapSendError(SendStageTransport, m.driver.Send(Message{
		To:        []string{to},
		Subject:   rendered.Subject,
		HTML:      rendered.HTML,
		Text:      rendered.Text,
		MessageID: options.MessageID,
		Headers:   options.Headers,
		Inline:    rendered.Inline,
	}))
}

// RenderOptions controls rendering for previews and tests. The zero value
// matches production sends.
type RenderOptions struct {
	// Now stamps template metadata such as the footer year; zero uses time.Now.
	Now time.Time
	// Validation sets the MJML validation level; empty keeps the MJML default.
	Validation mjml.ValidationLevel
	// Beautify emits readable HTML instead of minified HTML.
	Beautify bool
}

// Rendered is a fully rendered email, ready for a transport.
type Rendered struct {
	Subject string
	// Preheader is the inbox preview text shown after the subject.
	Preheader string
	Locale    string
	// HTML references inline images as cid:<CID>; see PreviewHTML.
	HTML string
	Text string
	// Inline are the images the HTML embeds, sent as related MIME parts.
	Inline []InlineImage
}

// PreviewHTML inlines embedded images as data URIs so the HTML renders on its
// own in browsers and previews, where cid: references cannot resolve.
func (r Rendered) PreviewHTML() string {
	html := r.HTML
	for _, image := range r.Inline {
		html = strings.ReplaceAll(html, "cid:"+image.CID, "data:"+image.ContentType+";base64,"+base64.StdEncoding.EncodeToString(image.Data))
	}
	return html
}

// Render executes a mailable's MJML and plain-text templates without sending.
func Render(email Mailable, opts RenderOptions) (Rendered, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	ctx := templateContext{
		Data: email.Data(),
	}
	ctx.Meta.Subject = email.Subject()
	ctx.Meta.Year = now.Year()
	locale := defaultMailLocale
	if localized, ok := email.(LocalizedMailable); ok {
		locale = NormalizeLocale(localized.Locale())
	}
	ctx.Meta.Locale = locale

	// Render MJML → HTML
	htmlTmpl, err := htmltpl.New("layout.mjml").Funcs(templateFuncsForLocale(locale)).ParseFS(templateFS, "templates/layout.mjml", "templates/"+email.Template())
	if err != nil {
		return Rendered{}, wrapSendError(SendStageHTMLTemplateParse, fmt.Errorf("failed to parse html templates: %w", err))
	}

	var mjmlBuffer bytes.Buffer
	if err := htmlTmpl.Execute(&mjmlBuffer, ctx); err != nil {
		return Rendered{}, wrapSendError(SendStageHTMLTemplateExecute, fmt.Errorf("failed to execute html template: %w", err))
	}

	// The layout's preview block is plain inbox text; expose it for previews.
	var preheader bytes.Buffer
	if err := htmlTmpl.ExecuteTemplate(&preheader, "preheader", ctx.Data); err != nil {
		return Rendered{}, wrapSendError(SendStageHTMLTemplateExecute, fmt.Errorf("failed to execute preheader template: %w", err))
	}

	mjmlOptions := []mjml.ToHTMLOption{mjml.WithMinify(!opts.Beautify), mjml.WithBeautify(opts.Beautify)}
	if opts.Validation != "" {
		mjmlOptions = append(mjmlOptions, mjml.WithValidationLevel(opts.Validation))
	}
	htmlContent, err := mjml.ToHTML(context.Background(), mjmlBuffer.String(), mjmlOptions...)
	if err != nil {
		return Rendered{}, wrapSendError(SendStageMJMLRender, fmt.Errorf("mjml render error: %w", err))
	}

	// Render plain-text
	textTemplateName := strings.TrimSuffix(email.Template(), ".mjml") + ".txt"
	textTmpl, err := texttpl.New("layout.txt").Funcs(textTemplateFuncsForLocale(locale)).ParseFS(templateFS, "templates/layout.txt", "templates/"+textTemplateName)
	if err != nil {
		return Rendered{}, wrapSendError(SendStageTextTemplateParse, fmt.Errorf("failed to parse text templates: %w", err))
	}

	var textBuffer bytes.Buffer
	if err := textTmpl.Execute(&textBuffer, ctx); err != nil {
		return Rendered{}, wrapSendError(SendStageTextTemplateExecute, fmt.Errorf("failed to execute text template: %w", err))
	}

	return Rendered{
		Subject:   ctx.Meta.Subject,
		Preheader: strings.Join(strings.Fields(html.UnescapeString(preheader.String())), " "),
		Locale:    locale,
		HTML:      htmlContent,
		Text:      textBuffer.String(),
		Inline:    brandImages,
	}, nil
}

// TemplateNames lists the embedded mailable templates, excluding the layout.
func TemplateNames() []string {
	matches, _ := fs.Glob(templateFS, "templates/*.mjml")
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if name := path.Base(match); name != "layout.mjml" {
			names = append(names, name)
		}
	}
	return names
}
