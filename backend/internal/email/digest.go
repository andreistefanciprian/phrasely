package email

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"github.com/resend/resend-go/v2"
)

//go:embed templates/phrase-digest.html
var phraseDigestTemplateStr string

var phraseDigestTmpl = template.Must(template.New("phrase-digest").Parse(phraseDigestTemplateStr))

// DigestPhrase is the per-phrase data rendered into the digest email.
type DigestPhrase struct {
	ID        string
	Headwords []string
	Phrase    string
}

func (s *ResendSender) SendPhraseDigest(to string, phrases []DigestPhrase) error {
	html, err := renderPhraseDigest(phrases)
	if err != nil {
		return fmt.Errorf("render phrase digest template: %w", err)
	}

	subject := "Your Phrase Digest"
	if len(phrases) == 1 {
		subject = "Your phrase for today: " + strings.Join(phrases[0].Headwords, " • ")
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
	var headwords []string
	for _, p := range phrases {
		headwords = append(headwords, p.Headwords...)
	}
	slog.Info("phrase digest", "email", to, "headwords", strings.Join(headwords, ", "))
	return nil
}

// phraseDigestView is the data passed to the phrase-digest template. OpenURL
// deep-links to the digest's phrase on the shuffle page (always phrases[0] —
// SendDue currently always sends exactly one phrase per digest).
type phraseDigestView struct {
	Phrases []digestPhraseView
	OpenURL string
}

type digestPhraseView struct {
	Headwords []digestHeadwordView
	Phrase    template.HTML
	Single    bool
}

type digestHeadwordView struct {
	Text    string
	Meaning string
	First   bool
}

type phraseMatch struct {
	start         int
	end           int
	headwordStart int
	headwordEnd   int
	meaning       string
}

func renderPhraseDigest(phrases []DigestPhrase) (string, error) {
	openURL := "https://getphrasely.com"
	if len(phrases) > 0 {
		openURL = "https://getphrasely.com/shuffle?id=" + phrases[0].ID
	}

	viewPhrases := make([]digestPhraseView, len(phrases))
	for i, phrase := range phrases {
		viewPhrases[i] = prepareDigestPhrase(phrase)
	}

	var buf bytes.Buffer
	if err := phraseDigestTmpl.Execute(&buf, phraseDigestView{Phrases: viewPhrases, OpenURL: openURL}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func prepareDigestPhrase(phrase DigestPhrase) digestPhraseView {
	headwords := make([]digestHeadwordView, len(phrase.Headwords))
	matches := make([]phraseMatch, 0, len(phrase.Headwords))

	for i, headword := range phrase.Headwords {
		meaning := ""
		if strings.TrimSpace(headword) != "" {
			pattern := `(?i)(\b` + regexp.QuoteMeta(headword) + `\b)[\s\x{00A0}]*(?:\*|_)?\(([^()]*)\)(?:\*|_)?`
			re := regexp.MustCompile(pattern)
			match := re.FindStringSubmatchIndex(phrase.Phrase)
			if match != nil {
				meaning = strings.TrimSpace(phrase.Phrase[match[4]:match[5]])
				matches = append(matches, phraseMatch{
					start:         match[0],
					end:           match[1],
					headwordStart: match[2],
					headwordEnd:   match[3],
					meaning:       meaning,
				})
			} else {
				headwordMatch := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(headword) + `\b`).FindStringIndex(phrase.Phrase)
				if headwordMatch != nil {
					matches = append(matches, phraseMatch{
						start:         headwordMatch[0],
						end:           headwordMatch[1],
						headwordStart: headwordMatch[0],
						headwordEnd:   headwordMatch[1],
					})
				}
			}
		}

		headwords[i] = digestHeadwordView{
			Text:    headword,
			Meaning: meaning,
			First:   i == 0,
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].start < matches[j].start })

	var formatted strings.Builder
	position := 0
	for _, match := range matches {
		if match.start < position {
			continue
		}
		formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[position:match.start]))
		formatted.WriteString(`<strong style="font-weight:700;">`)
		formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[match.headwordStart:match.headwordEnd]))
		formatted.WriteString(`</strong>`)
		if match.meaning != "" {
			formatted.WriteString(` <span style="color:#625CD9;font-size:0.88em;font-style:normal;">(`)
			formatted.WriteString(template.HTMLEscapeString(match.meaning))
			formatted.WriteString(`)</span>`)
		}
		position = match.end
	}
	formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[position:]))

	return digestPhraseView{
		Headwords: headwords,
		// Phrase is safe because every user-provided segment is escaped before
		// being combined with the fixed formatting tags above.
		Phrase: template.HTML(formatted.String()),
		Single: len(headwords) == 1,
	}
}
