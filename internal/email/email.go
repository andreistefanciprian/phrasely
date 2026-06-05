package email

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"

	"github.com/resend/resend-go/v2"
)

//go:embed templates/magic-link.html
var magicLinkTemplate string

// Sender is the interface for sending magic link emails.
// Implementations: ResendSender (production), LogSender (local dev).
type Sender interface {
	SendMagicLink(to, link string) error
}

// ResendSender sends emails via the Resend API.
type ResendSender struct {
	client *resend.Client
	from   string
}

func NewResendSender(apiKey, from string) *ResendSender {
	return &ResendSender{client: resend.NewClient(apiKey), from: from}
}

func (s *ResendSender) SendMagicLink(to, link string) error {
	html, err := renderMagicLink(to, link)
	if err != nil {
		return fmt.Errorf("render magic link template: %w", err)
	}

	_, err = s.client.Emails.Send(&resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: "Your Phrasely sign-in link",
		Html:    html,
	})
	if err != nil {
		return fmt.Errorf("send magic link email: %w", err)
	}
	return nil
}

// LogSender logs the magic link to stdout instead of sending an email.
// Used in local dev when no email provider is configured.
type LogSender struct{}

func (s *LogSender) SendMagicLink(to, link string) error {
	slog.Info("magic link", "email", to, "link", link)
	return nil
}

func renderMagicLink(to, link string) (string, error) {
	tmpl, err := template.New("magic-link").Parse(magicLinkTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ To, Link string }{to, link}); err != nil {
		return "", err
	}
	return buf.String(), nil
}
