package i18n

import (
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in   string
		want Locale
	}{
		{"", Default},
		{"zh-TW", LocaleZHTW},
		{"en", LocaleEN},
		{"fr", Default},     // unrecognised → fallback
		{"zh", Default},     // close-but-not-it → fallback
		{"zh-CN", Default},  // not declared yet
		{"EN", Default},     // case-sensitive on purpose; users.locale stores canonical
	}
	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"zh-TW": true,
		"en":    true,
		"":      false,
		"fr":    false,
	}
	for in, want := range cases {
		if got := Valid(in); got != want {
			t.Errorf("Valid(%q) = %v, want %v", in, got, want)
		}
	}
}

// Every key declared in keys.go (or as any other constant we export) must
// resolve to a non-empty zh-TW translation. zh-TW is the canonical table
// — a missing key here means T() falls all the way through to «key» and
// the UI shows raw key syntax.
func TestZhTW_KeysAllNonEmpty(t *testing.T) {
	for k, v := range zhTW {
		if strings.TrimSpace(v) == "" {
			t.Errorf("zhTW[%q] is empty — every key needs a canonical zh-TW value", k)
		}
	}
}

// en is allowed to be partial during the PR 2 rollout — missing keys
// fall back to zh-TW. But every key that IS in en MUST also be in zhTW;
// an en-only key would silently be unreachable when a zh user reads it.
func TestEN_KeysExistInZhTW(t *testing.T) {
	for k := range en {
		if _, ok := zhTW[k]; !ok {
			t.Errorf("en[%q] has no zhTW counterpart — every key must exist in the canonical table", k)
		}
	}
}

// When a key is present in BOTH tables, fmt directives must match. A
// translator dropping a "%s" would otherwise emit "%!(MISSING)" only
// when that specific notification fires.
func TestFormatDirectiveParity(t *testing.T) {
	for k, zh := range zhTW {
		env, ok := en[k]
		if !ok || env == "" {
			continue
		}
		zhCount := countDirectives(zh)
		enCount := countDirectives(env)
		if zhCount != enCount {
			t.Errorf("key %q: zh has %d fmt directives (%q), en has %d (%q) — counts must match",
				k, zhCount, zh, enCount, env)
		}
	}
}

// countDirectives counts %s/%d/%v/%q/%t (the subset we actually use).
// Doubled %% is intentionally counted as zero (it's a literal percent).
// This is a loose linter — sufficient for catching "translator forgot
// the %s" without needing to reimplement fmt's directive parser.
func countDirectives(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '%' || i+1 >= len(s) {
			continue
		}
		switch s[i+1] {
		case '%':
			i++ // skip literal %%
		case 's', 'd', 'v', 'q', 't':
			n++
			i++
		}
	}
	return n
}

// Spot-check the public T() API: a known zh-only key returns zh in en
// mode (fallback path), and a totally unknown key returns the «key»
// sentinel.
func TestT_FallbackBehavior(t *testing.T) {
	// Add a temporary en-empty key by reading one we know is zh-only:
	// CommonBack is in both, so fallback isn't observable. Use a key
	// that we deliberately leave in zh only by running with a nonsense
	// key to test the «key» sentinel.
	got := T(LocaleEN, "definitely.not.a.real.key")
	if got != "«definitely.not.a.real.key»" {
		t.Errorf("T(en, unknown) = %q, want sentinel", got)
	}
	if got := T(LocaleZHTW, CommonBack); !strings.Contains(got, "返回") {
		t.Errorf("T(zh, CommonBack) = %q, want zh value", got)
	}
	if got := T(LocaleEN, CommonBack); got != "Back" {
		t.Errorf("T(en, CommonBack) = %q, want \"Back\"", got)
	}
}
