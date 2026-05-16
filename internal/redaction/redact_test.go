package redaction

import (
	"reflect"
	"strings"
	"testing"
)

func newDefault(t *testing.T) *Redactor {
	t.Helper()

	r, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(DefaultConfig()): %v", err)
	}

	return r
}

func TestScrub_BuiltinKinds(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		kind   string
		marker string
	}{
		{
			name:   "aws_access_key",
			input:  "key=AKIAIOSFODNN7EXAMPLE here",
			kind:   "aws_access_key",
			marker: "«redacted:aws_access_key»",
		},
		{
			name:   "github_pat",
			input:  "token=ghp_" + strings.Repeat("a", 36) + " end",
			kind:   "github_pat",
			marker: "«redacted:github_pat»",
		},
		{
			name:   "slack_token",
			input:  "tok xoxb-abcdefghij1234567890 end",
			kind:   "slack_token",
			marker: "«redacted:slack_token»",
		},
		{
			name: "private_key_block",
			input: `pre
-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAKj34GkxFhD90vcN
-----END RSA PRIVATE KEY-----
post`,
			kind:   "private_key_block",
			marker: "«redacted:private_key_block»",
		},
		{
			name:   "env_secret_assignment",
			input:  "config: MY_API_KEY=hunter2",
			kind:   "env_secret_assignment",
			marker: "«redacted:env_secret_assignment»",
		},
		{
			name:   "aws_temp_credentials",
			input:  "tmp ASIAY34FZKBOKMUTVV7A end",
			kind:   "aws_access_key",
			marker: "«redacted:aws_access_key»",
		},
		{
			name:   "github_fine_grained_pat",
			input:  "use github_pat_" + strings.Repeat("a", 82) + " here",
			kind:   "github_fine_grained_pat",
			marker: "«redacted:github_fine_grained_pat»",
		},
		{
			name:   "gitlab_pat",
			input:  "ci uses glpat-" + strings.Repeat("a", 20) + " for deploy",
			kind:   "gitlab_pat",
			marker: "«redacted:gitlab_pat»",
		},
		{
			name:   "google_api_key",
			input:  "maps key AIza" + strings.Repeat("b", 35) + " end",
			kind:   "google_api_key",
			marker: "«redacted:google_api_key»",
		},
		{
			name:   "gcp_oauth_client_secret",
			input:  "secret GOCSPX-" + strings.Repeat("c", 28) + " end",
			kind:   "gcp_oauth_client_secret",
			marker: "«redacted:gcp_oauth_client_secret»",
		},
		{
			name:   "anthropic_api_key",
			input:  "ANTHROPIC=sk-ant-api03-" + strings.Repeat("d", 40) + " end",
			kind:   "anthropic_api_key",
			marker: "«redacted:anthropic_api_key»",
		},
		{
			name:   "huggingface_token",
			input:  "auth hf_" + strings.Repeat("e", 36) + " end",
			kind:   "huggingface_token",
			marker: "«redacted:huggingface_token»",
		},
		{
			name:   "openai_api_key_modern",
			input:  "key sk-proj-" + strings.Repeat("f", 74) + "T3BlbkFJ" + strings.Repeat("g", 74) + " end",
			kind:   "openai_api_key",
			marker: "«redacted:openai_api_key»",
		},
		{
			name:   "openai_api_key_legacy",
			input:  "key sk-" + strings.Repeat("h", 20) + "T3BlbkFJ" + strings.Repeat("i", 20) + " end",
			kind:   "openai_api_key",
			marker: "«redacted:openai_api_key»",
		},
		{
			name:   "stripe_secret_key_live",
			input:  "billing sk_live_" + strings.Repeat("j", 24) + " end",
			kind:   "stripe_secret_key",
			marker: "«redacted:stripe_secret_key»",
		},
		{
			name:   "stripe_secret_key_restricted",
			input:  "ci rk_test_" + strings.Repeat("k", 24) + " end",
			kind:   "stripe_secret_key",
			marker: "«redacted:stripe_secret_key»",
		},
		{
			name:   "stripe_webhook_secret",
			input:  "hook whsec_" + strings.Repeat("l", 32) + " end",
			kind:   "stripe_webhook_secret",
			marker: "«redacted:stripe_webhook_secret»",
		},
		{
			name:   "discord_bot_token",
			input:  "tok M" + strings.Repeat("m", 23) + "." + strings.Repeat("n", 6) + "." + strings.Repeat("o", 27) + " end",
			kind:   "discord_bot_token",
			marker: "«redacted:discord_bot_token»",
		},
		{
			name:   "discord_webhook",
			input:  "ping https://discord.com/api/webhooks/123456789012345678/" + strings.Repeat("a", 68) + " end",
			kind:   "discord_webhook",
			marker: "«redacted:discord_webhook»",
		},
		{
			name:   "slack_webhook",
			input:  "ops https://hooks.slack.com/services/T01ABCDEF/B01ABCDEF/abc123XYZdef456WERTYabcd end",
			kind:   "slack_webhook",
			marker: "«redacted:slack_webhook»",
		},
		{
			name:   "slack_token_new_variant",
			input:  "cookie xoxd-abcdefghij1234567890 end",
			kind:   "slack_token",
			marker: "«redacted:slack_token»",
		},
		{
			name:   "mailgun_api_key",
			input:  "mg key-" + strings.Repeat("a", 32) + " end",
			kind:   "mailgun_api_key",
			marker: "«redacted:mailgun_api_key»",
		},
		{
			name:   "sendgrid_api_key",
			input:  "mail SG." + strings.Repeat("p", 22) + "." + strings.Repeat("q", 43) + " end",
			kind:   "sendgrid_api_key",
			marker: "«redacted:sendgrid_api_key»",
		},
		{
			name:   "npm_token",
			input:  "publish npm_" + strings.Repeat("r", 36) + " end",
			kind:   "npm_token",
			marker: "«redacted:npm_token»",
		},
		{
			name:   "pypi_token",
			input:  "upload pypi-AgEIcHlwaS5vcmc" + strings.Repeat("s", 60) + " end",
			kind:   "pypi_token",
			marker: "«redacted:pypi_token»",
		},
		{
			name:   "linear_api_key",
			input:  "linear lin_api_" + strings.Repeat("t", 40) + " end",
			kind:   "linear_api_key",
			marker: "«redacted:linear_api_key»",
		},
		{
			name: "jwt",
			input: "tok eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9." +
				"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c end",
			kind:   "jwt",
			marker: "«redacted:jwt»",
		},
		{
			name:   "db_connection_string",
			input:  "DSN postgres://alice:hunter2@db.internal:5432/app end",
			kind:   "db_connection_string",
			marker: "«redacted:db_connection_string»",
		},
		{
			name:   "bearer_auth_header",
			input:  "curl -H 'Authorization: Bearer abcdef1234567890qwertyuiop' /api",
			kind:   "bearer_auth_header",
			marker: "«redacted:bearer_auth_header»",
		},
		{
			name:   "basic_auth_url",
			input:  "fetch https://alice:hunter2@example.com/api here",
			kind:   "basic_auth_url",
			marker: "«redacted:basic_auth_url»",
		},
	}

	r := newDefault(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, hits := r.Scrub(tc.input)

			if !strings.Contains(out, tc.marker) {
				t.Errorf("output missing marker %q\ngot: %q", tc.marker, out)
			}
			if len(hits) != 1 {
				t.Fatalf("expected 1 hit, got %d: %+v", len(hits), hits)
			}

			if hits[0].Kind != tc.kind {
				t.Errorf("hit kind = %q, want %q", hits[0].Kind, tc.kind)
			}
			if hits[0].Start < 0 || hits[0].End > len(tc.input) || hits[0].Start >= hits[0].End {
				t.Errorf("hit positions invalid: start=%d end=%d input_len=%d", hits[0].Start, hits[0].End, len(tc.input))
			}
		})
	}
}

func TestScrub_NearMisses(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"aws_lowercase", "akia0123456789abcdef"},
		{"aws_too_short", "AKIA0123456789ABC"},
		{"aws_wrong_prefix", "AKIB0123456789ABCDEF"},
		{"github_pat_too_short", "ghp_" + strings.Repeat("a", 10)},
		{"github_fine_grained_too_short", "github_pat_" + strings.Repeat("a", 50)},
		{"slack_wrong_kind", "xoxz-abcdefghij1234567890"},
		{"pem_half_block", "-----BEGIN RSA PRIVATE KEY-----\ndata\n"},
		{"env_no_value", "API_KEY="},
		{"env_lowercase_name", "api_key=foo"},
		{"env_bare_keyword", "KEY=foo"},
		{"google_api_key_too_short", "AIza0123"},
		{"google_api_key_wrong_prefix", "AAza" + strings.Repeat("a", 35)},
		{"gcp_oauth_wrong_prefix", "GOOGSPX-" + strings.Repeat("a", 28)},
		{"openai_no_marker", "sk-" + strings.Repeat("a", 40)},
		{"openai_short_legacy", "sk-" + strings.Repeat("a", 5) + "T3BlbkFJ" + strings.Repeat("a", 5)},
		{"anthropic_wrong_segment", "sk-ant-foo-" + strings.Repeat("a", 32)},
		{"stripe_wrong_env", "sk_prod_" + strings.Repeat("a", 24)},
		{"stripe_secret_too_short", "sk_live_" + strings.Repeat("a", 10)},
		{"webhook_secret_too_short", "whsec_" + strings.Repeat("a", 10)},
		{"discord_too_short", "M" + strings.Repeat("a", 5) + "." + strings.Repeat("a", 6) + "." + strings.Repeat("a", 27)},
		{"discord_wrong_prefix", "Z" + strings.Repeat("a", 23) + "." + strings.Repeat("a", 6) + "." + strings.Repeat("a", 27)},
		{"slack_webhook_wrong_host", "https://hooks.example.com/services/T01/B01/abc"},
		{"npm_too_short", "npm_" + strings.Repeat("a", 10)},
		{"pypi_wrong_prefix", "pypi-WrongPrefix" + strings.Repeat("a", 60)},
		{"linear_too_short", "lin_api_" + strings.Repeat("a", 10)},
		{"jwt_two_segments", "eyJabc.eyJdef"},
		{"db_no_credentials", "postgres://host:5432/db"},
		{"bearer_too_short", "Authorization: Bearer abc"},
		{"basic_no_credentials", "https://example.com/path"},
	}

	r := newDefault(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, hits := r.Scrub(tc.input)
			if out != tc.input {
				t.Errorf("near-miss was scrubbed:\nin:  %q\nout: %q", tc.input, out)
			}

			if hits != nil {
				t.Errorf("near-miss produced hits: %+v", hits)
			}
		})
	}
}

func TestScrub_OverlapPrefersLonger(t *testing.T) {
	r := newDefault(t)

	input := "config: AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE end"
	out, hits := r.Scrub(input)

	if len(hits) != 1 {
		t.Fatalf("expected exactly 1 hit (containing pattern wins), got %d: %+v", len(hits), hits)
	}

	if hits[0].Kind != "env_secret_assignment" {
		t.Errorf("expected env_secret_assignment to shadow aws_access_key, got %q", hits[0].Kind)
	}
	if strings.Contains(out, "AKIA") {
		t.Errorf("AKIA leaked into output: %q", out)
	}
}

func TestScrub_MultipleKinds(t *testing.T) {
	r := newDefault(t)

	input := "AKIAIOSFODNN7EXAMPLE then ghp_" + strings.Repeat("b", 36) + " and xoxb-abcdefghij1234567890"
	_, hits := r.Scrub(input)

	if len(hits) != 3 {
		t.Fatalf("expected 3 hits, got %d: %+v", len(hits), hits)
	}

	counts := KindCounts(hits)
	for _, k := range []string{"aws_access_key", "github_pat", "slack_token"} {
		if counts[k] != 1 {
			t.Errorf("counts[%s] = %d, want 1", k, counts[k])
		}
	}
}

func TestScrub_Idempotent(t *testing.T) {
	r := newDefault(t)

	inputs := []string{
		"clean text with nothing sensitive",
		"AKIAIOSFODNN7EXAMPLE",
		"ghp_" + strings.Repeat("c", 40),
		"xoxb-abcdefghij1234567890",
		"-----BEGIN RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----",
		"FOO_TOKEN=bar baz",
		"mixed: AKIAIOSFODNN7EXAMPLE and MY_API_KEY=hunter2",
		"pre-redacted «redacted:aws_access_key» content",
	}

	for _, in := range inputs {
		first, _ := r.Scrub(in)
		second, secondHits := r.Scrub(first)

		if first != second {
			t.Errorf("not idempotent:\nin:     %q\nfirst:  %q\nsecond: %q", in, first, second)
		}
		if len(secondHits) != 0 {
			t.Errorf("second pass produced hits on already-scrubbed text: %+v", secondHits)
		}
	}
}

func TestScrub_NilReceiver(t *testing.T) {
	var r *Redactor
	out, hits := r.Scrub("AKIAIOSFODNN7EXAMPLE")

	if out != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("nil receiver modified input: %q", out)
	}
	if hits != nil {
		t.Errorf("nil receiver returned hits: %+v", hits)
	}
}

func TestScrub_Empty(t *testing.T) {
	r := newDefault(t)

	out, hits := r.Scrub("")
	if out != "" || hits != nil {
		t.Errorf("empty input mishandled: out=%q hits=%+v", out, hits)
	}
}

func TestNew_UnknownBuiltin(t *testing.T) {
	_, err := New(Config{BuiltinKinds: []string{"bogus_kind"}})
	if err == nil {
		t.Fatal("expected error for unknown built-in kind")
	}
}

func TestNew_BadCustomPattern(t *testing.T) {
	_, err := New(Config{Custom: []CustomPattern{{Kind: "bad", Pattern: "[invalid"}}})
	if err == nil {
		t.Fatal("expected error for malformed regex")
	}
}

func TestNew_EmptyCustomKind(t *testing.T) {
	_, err := New(Config{Custom: []CustomPattern{{Kind: "", Pattern: ".+"}}})
	if err == nil {
		t.Fatal("expected error for empty custom kind")
	}
}

func TestScrub_CustomPattern(t *testing.T) {
	r, err := New(Config{
		BuiltinKinds: []string{"aws_access_key"},
		Custom:       []CustomPattern{{Kind: "internal_id", Pattern: `RID-[0-9]{6}`}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	out, hits := r.Scrub("ticket RID-123456 mentions AKIAIOSFODNN7EXAMPLE")
	want := []string{"«redacted:internal_id»", "«redacted:aws_access_key»"}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output missing %q: got %q", w, out)
		}
	}

	if len(hits) != 2 {
		t.Errorf("expected 2 hits, got %d: %+v", len(hits), hits)
	}
}

func TestKindCounts(t *testing.T) {
	hits := []Hit{
		{Kind: "aws_access_key"},
		{Kind: "github_pat"},
		{Kind: "aws_access_key"},
	}
	got := KindCounts(hits)
	want := map[string]int{"aws_access_key": 2, "github_pat": 1}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("KindCounts = %v, want %v", got, want)
	}
	if KindCounts(nil) != nil {
		t.Error("KindCounts(nil) should be nil")
	}
}

func TestKinds_StableOrder(t *testing.T) {
	a := Kinds()
	a[0] = "mutated"

	b := Kinds()
	if b[0] == "mutated" {
		t.Error("Kinds() returned shared underlying slice")
	}
}
