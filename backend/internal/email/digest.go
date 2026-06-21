package email

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"strings"

	"github.com/resend/resend-go/v2"
)

//go:embed templates/phrase-digest.html
var phraseDigestTemplateStr string

var phraseDigestTmpl = template.Must(template.New("phrase-digest").Parse(phraseDigestTemplateStr))

// DigestPhrase is the per-phrase data rendered into the digest email.
type DigestPhrase struct {
	Headword string
	Phrase   string
}

func (s *ResendSender) SendPhraseDigest(to string, phrases []DigestPhrase) error {
	html, err := renderPhraseDigest(phrases)
	if err != nil {
		return fmt.Errorf("render phrase digest template: %w", err)
	}

	subject := "Your Phrase Digest"
	if len(phrases) == 1 {
		subject = "Your phrase for today: " + phrases[0].Headword
	}

	_, err = s.client.Emails.Send(&resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
	})
	if err != nil {
		return fmt.Errorf("send phrase digest email: %w", err)
	}
	return nil
}

func (s *LogSender) SendPhraseDigest(to string, phrases []DigestPhrase) error {
	headwords := make([]string, len(phrases))
	for i, p := range phrases {
		headwords[i] = p.Headword
	}
	slog.Info("phrase digest", "email", to, "headwords", strings.Join(headwords, ", "))
	return nil
}

func renderPhraseDigest(phrases []DigestPhrase) (string, error) {
	var buf bytes.Buffer
	if err := phraseDigestTmpl.Execute(&buf, phrases); err != nil {
		return "", err
	}
	return buf.String(), nil
}
