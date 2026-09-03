package email

import (
	"strings"
	"testing"
)

func TestRenderMagicLinkIncludesDarkModeStyles(t *testing.T) {
	html, err := renderMagicLink("reader@example.com", "https://getphrasely.com/auth-verify?token=test")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		`name="color-scheme" content="light dark"`,
		`name="supported-color-schemes" content="light dark"`,
		`@media (prefers-color-scheme: dark)`,
		`class="sign-in-card"`,
		`class="sign-in-link"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered email is missing dark-mode marker %q", want)
		}
	}
}
