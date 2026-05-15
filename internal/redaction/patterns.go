package redaction

import "regexp"

type kindPattern struct {
	kind string
	re   *regexp.Regexp
}

var (
	builtinKindOrder = []string{
		// Cloud
		"aws_access_key",
		"gcp_oauth_client_secret",
		"google_api_key",

		// Source forges
		"github_pat",
		"github_fine_grained_pat",
		"gitlab_pat",

		// AI / ML APIs
		"anthropic_api_key",
		"huggingface_token",
		"openai_api_key",

		// Payments
		"stripe_secret_key",
		"stripe_webhook_secret",

		// Communication
		"discord_bot_token",
		"discord_webhook",
		"slack_token",
		"slack_webhook",

		// Email
		"mailgun_api_key",
		"sendgrid_api_key",

		// Package managers
		"npm_token",
		"pypi_token",

		// Project management
		"linear_api_key",

		// Generic / structural
		"jwt",
		"db_connection_string",
		"bearer_auth_header",
		"basic_auth_url",
		"private_key_block",
		"env_secret_assignment",
	}

	builtinPatterns = map[string]*regexp.Regexp{
		"aws_access_key":          regexp.MustCompile(`(?:AKIA|ASIA)[0-9A-Z]{16}`),
		"gcp_oauth_client_secret": regexp.MustCompile(`GOCSPX-[A-Za-z0-9_-]{28}`),
		"google_api_key":          regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
		"github_pat":              regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,255}`),
		"github_fine_grained_pat": regexp.MustCompile(`github_pat_[A-Za-z0-9_]{82}`),
		"gitlab_pat":              regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20,40}`),
		"anthropic_api_key":       regexp.MustCompile(`sk-ant-(?:api|sid)\w*-[A-Za-z0-9_-]{32,255}`),
		"huggingface_token":       regexp.MustCompile(`hf_[A-Za-z0-9]{34,40}`),
		"openai_api_key":          regexp.MustCompile(`sk-(?:(?:proj|svcacct|admin)-[A-Za-z0-9_-]{58,74}T3BlbkFJ[A-Za-z0-9_-]{58,74}|[A-Za-z0-9]{20}T3BlbkFJ[A-Za-z0-9]{20})`),
		"stripe_secret_key":       regexp.MustCompile(`(?:sk|rk)_(?:live|test)_[0-9a-zA-Z]{24,99}`),
		"stripe_webhook_secret":   regexp.MustCompile(`whsec_[A-Za-z0-9]{32,80}`),
		"discord_bot_token":       regexp.MustCompile(`[MNO][A-Za-z0-9_-]{23,25}\.[A-Za-z0-9_-]{6,7}\.[A-Za-z0-9_-]{27,40}`),
		"discord_webhook":         regexp.MustCompile(`https?://(?:(?:canary|ptb)\.)?discord(?:app)?\.com/api/webhooks/[0-9]{15,25}/[A-Za-z0-9_-]{50,100}`),
		"slack_token":             regexp.MustCompile(`xox[abcdeoprs]-[A-Za-z0-9-]{15,200}`),
		"slack_webhook":           regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]{8,12}/B[A-Z0-9]{8,12}/[A-Za-z0-9]{20,30}`),
		"mailgun_api_key":         regexp.MustCompile(`key-[a-f0-9]{32}`),
		"sendgrid_api_key":        regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
		"npm_token":               regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
		"pypi_token":              regexp.MustCompile(`pypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,200}`),
		"linear_api_key":          regexp.MustCompile(`lin_api_[A-Za-z0-9]{40}`),
		"jwt":                     regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,500}\.eyJ[A-Za-z0-9_-]{10,1000}\.[A-Za-z0-9_-]{10,500}`),
		"db_connection_string":    regexp.MustCompile(`(?i)(?:postgres(?:ql)?|mysql|mongodb(?:\+srv)?|redis|amqp|mssql)://[^:@\s/]{1,255}:[^@\s/]{1,255}@[^\s/]{1,500}`),
		"bearer_auth_header":      regexp.MustCompile(`(?i)authorization:\s*bearer\s+[A-Za-z0-9._-]{20,1000}`),
		"basic_auth_url":          regexp.MustCompile(`(?i)https?://[^:@\s/]{1,255}:[^@\s/]{1,255}@[^\s/]{1,500}`),
		"private_key_block":       regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
		"env_secret_assignment":   regexp.MustCompile(`[A-Z][A-Z0-9_]{0,64}(TOKEN|KEY|SECRET|PASSWORD|API_KEY)\s*=\s*\S{1,500}`),
	}
)
