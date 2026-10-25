package i18n_test

import (
	"strings"
	"testing"

	"github.com/GOAT-Foundry/CORTEX/internal/i18n"
)

func TestParseAndNormalize(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantOK  bool
		normOut string
	}{
		{"en", "en", true, "en"},
		{"en-US", "en", true, "en"},
		{"ja", "ja", true, "ja"},
		{"ja-JP", "ja", true, "ja"},
		{"JA_jp", "ja", true, "ja"},
		{"fr", "", false, "en"},
		{"", "", false, "en"},
	}

	for _, tc := range cases {
		got, ok := i18n.ParseStrict(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Fatalf("ParseStrict(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
		if norm := i18n.Normalize(tc.in); norm != tc.normOut {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, norm, tc.normOut)
		}
	}
}

func TestManagerResolveAndDirectives(t *testing.T) {
	mgr := i18n.NewManager("en")
	if mgr.Get() != "en" {
		t.Fatalf("expected en, got %s", mgr.Get())
	}

	// Resolve from Accept-Language when explicit and header are empty
	if got := mgr.ResolveLocale("", "", "ja-JP,ja;q=0.9,en-US;q=0.8"); got != "ja" {
		t.Fatalf("expected ja from Accept-Language, got %s", got)
	}

	// Switch manager locale to ja
	mgr.Set("ja")
	if !strings.Contains(mgr.AgentLanguageDirective(""), "Japanese (ja)") {
		t.Fatalf("expected Japanese directive, got %s", mgr.AgentLanguageDirective(""))
	}
	if !strings.Contains(mgr.SystemReminder(""), "ja (Japanese") {
		t.Fatalf("expected Japanese system reminder, got %s", mgr.SystemReminder(""))
	}

	// Override per-call with en
	if !strings.Contains(mgr.AgentLanguageDirective("en"), "English (en)") {
		t.Fatalf("expected English override directive, got %s", mgr.AgentLanguageDirective("en"))
	}
}
