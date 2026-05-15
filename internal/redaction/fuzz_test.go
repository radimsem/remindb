package redaction

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func FuzzScrub(f *testing.F) {
	seeds := []string{
		"",
		"plain text with no secrets",
		"AKIAIOSFODNN7EXAMPLE",
		"ASIAY34FZKBOKMUTVV7A",
		"ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"github_pat_" + strings.Repeat("a", 82),
		"glpat-aaaaaaaaaaaaaaaaaaaa",
		"AIza" + strings.Repeat("b", 35),
		"GOCSPX-" + strings.Repeat("c", 28),
		"sk-ant-api03-" + strings.Repeat("d", 40),
		"hf_" + strings.Repeat("e", 36),
		"sk-proj-" + strings.Repeat("f", 74) + "T3BlbkFJ" + strings.Repeat("g", 74),
		"sk_live_" + strings.Repeat("h", 24),
		"whsec_" + strings.Repeat("i", 32),
		"xoxb-abcdefghij1234567890",
		"xoxd-cookie-token-12345",
		"https://hooks.slack.com/services/T01ABCDEF/B01ABCDEF/abc123XYZdef456WERTYabcd",
		"https://discord.com/api/webhooks/123456789012345678/" + strings.Repeat("a", 68),
		"M" + strings.Repeat("a", 23) + "." + strings.Repeat("b", 6) + "." + strings.Repeat("c", 27),
		"key-" + strings.Repeat("a", 32),
		"SG." + strings.Repeat("p", 22) + "." + strings.Repeat("q", 43),
		"npm_" + strings.Repeat("r", 36),
		"pypi-AgEIcHlwaS5vcmc" + strings.Repeat("s", 60),
		"lin_api_" + strings.Repeat("t", 40),
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
		"postgres://alice:hunter2@db.internal:5432/app",
		"Authorization: Bearer abcdef1234567890qwertyuiop",
		"https://alice:hunter2@example.com/api",
		"-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----",
		"MY_API_KEY=hunter2",
		"akia0123456789abcdef",                  // lowercase near-miss
		"AKIA0123456789ABC",                     // too short
		"ghp_short",                             // PAT under length
		"-----BEGIN RSA PRIVATE KEY-----\nhalf", // half-formed PEM
		"AKIAIOSFODNN7EXAMPLE then ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"日本語 boundary AKIAIOSFODNN7EXAMPLE 日本語",
		"\xe3 multi-byte split at AKIA",
		"«redacted:aws_access_key» pre-injected",
		"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",
		"nested: -----BEGIN RSA PRIVATE KEY-----\nAKIAIOSFODNN7EXAMPLE\n-----END RSA PRIVATE KEY-----",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	r, err := New(DefaultConfig())
	if err != nil {
		f.Fatalf("New: %v", err)
	}

	f.Fuzz(func(t *testing.T, s string) {
		out, hits := r.Scrub(s)

		// Idempotency.
		out2, hits2 := r.Scrub(out)
		if out != out2 {
			t.Errorf("not idempotent:\nin:     %q\nfirst:  %q\nsecond: %q", s, out, out2)
		}
		if len(hits2) != 0 {
			t.Errorf("second pass produced hits: %+v", hits2)
		}

		// UTF-8 preservation: valid input → valid output.
		if utf8.ValidString(s) && !utf8.ValidString(out) {
			t.Errorf("UTF-8 invariant broken:\nin:  %q\nout: %q", s, out)
		}

		// Hit positions must be in-range against the original input.
		for _, h := range hits {
			if h.Start < 0 || h.End > len(s) || h.Start >= h.End {
				t.Errorf("hit out of range: %+v (input len %d)", h, len(s))
			}
		}

		// Non-empty hits ⇒ output differs from input.
		if len(hits) > 0 && out == s {
			t.Errorf("hits reported but output unchanged:\nin: %q\nhits: %+v", s, hits)
		}

		// Every hit must show up as a marker in the output.
		for _, h := range hits {
			marker := "«redacted:" + h.Kind + "»"
			if !strings.Contains(out, marker) {
				t.Errorf("hit kind %q not represented in output: %q", h.Kind, out)
			}
		}
	})
}
