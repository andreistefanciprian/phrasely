package email

import (
	"strings"
	"testing"
)

func TestPrepareDigestPhraseExtractsMeaning(t *testing.T) {
	got := prepareDigestPhrase(DigestPhrase{
		Headwords: []string{"sieve"},
		Phrase:    "I have to sieve (sort and filter) the useful information from the noise.",
	})

	formatted := string(got.Phrase)
	if !strings.Contains(formatted, `<strong style="font-weight:700;">sieve</strong>`) {
		t.Errorf("formatted phrase does not emphasize the headword: %q", formatted)
	}
	if strings.Count(formatted, "sort and filter") != 1 {
		t.Errorf("meaning count = %d, want 1", strings.Count(formatted, "sort and filter"))
	}
}

func TestPrepareDigestPhraseExtractsMultipleMarkdownMeanings(t *testing.T) {
	headwords := []string{"ethos", "openness", "decentralization"}
	got := prepareDigestPhrase(DigestPhrase{
		Headwords: headwords,
		Phrase: "The ethos\u00a0*(guiding values and beliefs)* of the early internet was rooted in " +
			"openness\u00a0*(transparency and accessibility)* and decentralization\u00a0*(distributing power away from a central authority)*, " +
			"fostering a collaborative online community.",
	})

	formatted := string(got.Phrase)
	if strings.Contains(formatted, "*") || strings.Contains(formatted, "\u00a0") {
		t.Fatalf("formatted phrase still contains markdown or a non-breaking space: %q", formatted)
	}

	wantMeanings := []string{
		"guiding values and beliefs",
		"transparency and accessibility",
		"distributing power away from a central authority",
	}
	for i, want := range wantMeanings {
		if strings.Count(formatted, want) != 1 {
			t.Errorf("meaning %q count = %d, want 1", want, strings.Count(formatted, want))
		}
		if !strings.Contains(formatted, `<strong style="font-weight:700;">`+headwords[i]+`</strong>`) {
			t.Errorf("formatted phrase does not emphasize %q", headwords[i])
		}
	}
}

func TestPrepareDigestPhraseLeavesUnrelatedParentheses(t *testing.T) {
	got := prepareDigestPhrase(DigestPhrase{
		Headwords: []string{"aside"},
		Phrase:    "As an aside, the plan worked (which surprised me).",
	})

	want := `As an <strong style="font-weight:700;">aside</strong>, the plan worked (which surprised me).`
	if string(got.Phrase) != want {
		t.Fatalf("Phrase = %q, want %q", got.Phrase, want)
	}
}

func TestRenderPhraseDigestShowsMeaningOnce(t *testing.T) {
	html, err := renderPhraseDigest([]DigestPhrase{{
		ID:        "phrase-1",
		Headwords: []string{"sieve"},
		Phrase:    "I have to sieve (sort and filter) the useful information from the noise.",
	}})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Count(html, "sort and filter") != 1 {
		t.Errorf("meaning count = %d, want 1", strings.Count(html, "sort and filter"))
	}
	if !strings.Contains(html, `class="inline-meaning" style="color:#625CD9;font-size:0.88em;font-style:normal;">(sort and filter)</span>`) {
		t.Error("rendered meaning does not use the smaller inline style")
	}
	if !strings.Contains(html, "https://getphrasely.com/shuffle?id=phrase-1") {
		t.Error("rendered email is missing its phrase deep link")
	}
	if strings.Count(html, "Manage digest settings or unsubscribe") != 1 {
		t.Error("rendered email should contain one combined settings link")
	}
}

func TestRenderPhraseDigestIncludesDarkModeStyles(t *testing.T) {
	html, err := renderPhraseDigest([]DigestPhrase{{
		ID:        "phrase-dark",
		Headwords: []string{"sieve"},
		Phrase:    "I have to sieve (sort and filter) the useful information from the noise.",
	}})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`name="color-scheme" content="light dark"`,
		`name="supported-color-schemes" content="light dark"`,
		`@media (prefers-color-scheme: dark)`,
		`class="phrase-card"`,
		`class="inline-meaning"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered email is missing dark-mode marker %q", want)
		}
	}
}

func TestRenderPhraseDigestUsesBulletSeparatedHeadwords(t *testing.T) {
	tests := []struct {
		name      string
		headwords []string
		phrase    string
		bullets   int
	}{
		{
			name:      "two headwords",
			headwords: []string{"candid", "tactful"},
			phrase:    "Her response was candid (honest and direct), yet tactful (careful not to offend).",
			bullets:   1,
		},
		{
			name:      "three headwords",
			headwords: []string{"ethos", "openness", "decentralization"},
			phrase:    "The ethos (guiding values) was rooted in openness (transparency) and decentralization (distributed power).",
			bullets:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := renderPhraseDigest([]DigestPhrase{{
				ID:        "phrase-multiple",
				Headwords: tt.headwords,
				Phrase:    tt.phrase,
			}})
			if err != nil {
				t.Fatal(err)
			}

			if strings.Count(html, "•") != tt.bullets {
				t.Errorf("bullet count = %d, want %d", strings.Count(html, "•"), tt.bullets)
			}
			if strings.Contains(html, ">vs<") {
				t.Error("rendered email still uses vs between headwords")
			}
		})
	}
}

func TestPrepareDigestPhraseEscapesUserContent(t *testing.T) {
	got := prepareDigestPhrase(DigestPhrase{
		Headwords: []string{"sieve"},
		Phrase:    `<script>alert("x")</script> sieve (<b>sort</b>)`,
	})
	formatted := string(got.Phrase)

	if strings.Contains(formatted, "<script>") || strings.Contains(formatted, "<b>sort</b>") {
		t.Fatalf("formatted phrase contains unescaped user HTML: %q", formatted)
	}
	if !strings.Contains(formatted, "&lt;script&gt;") || !strings.Contains(formatted, "&lt;b&gt;sort&lt;/b&gt;") {
		t.Fatalf("formatted phrase is missing escaped user HTML: %q", formatted)
	}
}
