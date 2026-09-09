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
	"unicode"
	"unicode/utf8"

	"github.com/andreistefanciprian/phrasely/internal/db"
	"github.com/resend/resend-go/v2"
)

//go:embed templates/phrase-digest.html
var phraseDigestTemplateStr string

var phraseDigestTmpl = template.Must(template.New("phrase-digest").Parse(phraseDigestTemplateStr))

// DigestPhrase is the per-phrase data rendered into the digest email.
type DigestPhrase struct {
	ID        string
	Headwords []db.Headword
	Phrase    string
}

func (s *ResendSender) SendPhraseDigest(to string, phrases []DigestPhrase) error {
	html, err := renderPhraseDigest(phrases)
	if err != nil {
		return fmt.Errorf("render phrase digest template: %w", err)
	}

	subject := "Your Phrase Digest"
	if len(phrases) == 1 {
		subject = "Your phrase for today: " + strings.Join(headwordLabels(phrases[0].Headwords), " • ")
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
		headwords = append(headwords, headwordLabels(p.Headwords)...)
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
	Headwords []string
	Phrase    template.HTML
}

type phraseMatch struct {
	start       int
	end         int
	headwordEnd int
	meaning     string
	index       int
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
	matches := make([]phraseMatch, 0, len(phrase.Headwords))

	for index, headword := range phrase.Headwords {
		if strings.TrimSpace(headword.Text) == "" {
			continue
		}
		pattern := `(?i)` + regexp.QuoteMeta(headword.Text)
		for _, match := range regexp.MustCompile(pattern).FindAllStringIndex(phrase.Phrase, -1) {
			if !wordBoundary(phrase.Phrase, match[0], match[1]) {
				continue
			}
			matches = append(matches, phraseMatch{start: match[0], end: match[1], headwordEnd: match[1], meaning: headword.Meaning, index: index})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})

	var formatted strings.Builder
	position := 0
	seen := map[int]bool{}
	for _, match := range matches {
		if match.start < position {
			continue
		}
		formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[position:match.start]))
		formatted.WriteString(`<strong style="font-weight:700;">`)
		formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[match.start:match.headwordEnd]))
		formatted.WriteString(`</strong>`)
		if match.meaning != "" && !seen[match.index] {
			formatted.WriteString(` <span class="inline-meaning" style="color:#625CD9;font-size:0.88em;font-style:normal;">(`)
			formatted.WriteString(template.HTMLEscapeString(match.meaning))
			formatted.WriteString(`)</span>`)
		}
		seen[match.index] = true
		position = match.end
	}
	formatted.WriteString(template.HTMLEscapeString(phrase.Phrase[position:]))
	for i, w := range phrase.Headwords {
		if !seen[i] {
			formatted.WriteString(` <span class="inline-meaning">(` + template.HTMLEscapeString(w.Text+": "+w.Meaning) + `)</span>`)
		}
	}

	return digestPhraseView{
		Headwords: headwordLabels(phrase.Headwords),
		// Phrase is safe because every user-provided segment is escaped before
		// being combined with the fixed formatting tags above.
		Phrase: template.HTML(formatted.String()),
	}
}

func headwordLabels(words []db.Headword) []string {
	labels := make([]string, len(words))
	for i, w := range words {
		labels[i] = w.Canonical
	}
	return labels
}

func wordBoundary(sentence string, start, end int) bool {
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(sentence[start:end])
	last, _ := utf8.DecodeLastRuneInString(sentence[start:end])
	before, _ := utf8.DecodeLastRuneInString(sentence[:start])
	after, _ := utf8.DecodeRuneInString(sentence[end:])
	return !(word(before) && word(first)) && !(word(last) && word(after))
}
