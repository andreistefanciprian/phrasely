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
	if !strings.Contains(formatted, `font-size:0.88em;font-style:normal;">(sort and filter)</span>`) {
		t.Errorf("formatted phrase does not render the smaller inline meaning: %q", formatted)
	}
	if strings.Count(formatted, "sort and filter") != 1 {
		t.Errorf("meaning count = %d, want 1", strings.Count(formatted, "sort and filter"))
	}
	if len(got.Headwords) != 1 {
		t.Fatalf("len(Headwords) = %d, want 1", len(got.Headwords))
	}
	if got.Headwords[0].Meaning != "sort and filter" {
		t.Errorf("Meaning = %q, want %q", got.Headwords[0].Meaning, "sort and filter")
	}
	if !got.Single {
		t.Error("Single = false, want true")
	}
}

func TestPrepareDigestPhraseExtractsMultipleMarkdownMeanings(t *testing.T) {
	got := prepareDigestPhrase(DigestPhrase{
		Headwords: []string{"ethos", "openness", "decentralization"},
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
		if got.Headwords[i].Meaning != want {
			t.Errorf("Headwords[%d].Meaning = %q, want %q", i, got.Headwords[i].Meaning, want)
		}
		if got.Headwords[i].First != (i == 0) {
			t.Errorf("Headwords[%d].First = %v", i, got.Headwords[i].First)
		}
		if strings.Count(formatted, want) != 1 {
			t.Errorf("meaning %q count = %d, want 1", want, strings.Count(formatted, want))
		}
		if !strings.Contains(formatted, `<strong style="font-weight:700;">`+got.Headwords[i].Text+`</strong>`) {
			t.Errorf("formatted phrase does not emphasize %q", got.Headwords[i].Text)
		}
	}
	if got.Single {
		t.Error("Single = true, want false")
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
	if got.Headwords[0].Meaning != "" {
		t.Fatalf("Meaning = %q, want empty", got.Headwords[0].Meaning)
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
	if !strings.Contains(html, `font-size:0.88em;font-style:normal;">(sort and filter)</span>`) {
		t.Error("rendered meaning does not use the smaller inline style")
	}
	if !strings.Contains(html, "https://getphrasely.com/shuffle?id=phrase-1") {
		t.Error("rendered email is missing its phrase deep link")
	}
	if strings.Count(html, "Manage digest settings or unsubscribe") != 1 {
		t.Error("rendered email should contain one combined settings link")
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
