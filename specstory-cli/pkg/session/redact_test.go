package session

import (
	"strings"
	"testing"
)

func TestRedactContent_BuiltinPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		label    string
		contains string // expected label in output
	}{
		{
			name:     "GitHub PAT",
			input:    "token: ghp_FakePATTokenForTestingOnlyNotRealXXX",
			contains: "[REDACTED:GITHUB_PAT]",
		},
		{
			name:     "GitHub Models Token",
			input:    "GITHUB_MODELS_TOKEN=gghp_FakeModelsTokenForTestingNotRealXXXX",
			contains: "[REDACTED:GITHUB_MODELS_TOKEN]",
		},
		{
			// Real fine-grained PATs are github_pat_ + exactly 82 word chars
			// (gitleaks github-fine-grained-pat).
			name:     "GitHub fine-grained PAT",
			input:    "auth: github_pat_1234567890123456789012345678901234567890123456789012345678901234567890FAKETESTABCD",
			contains: "[REDACTED:GITHUB_FINE_GRAINED_PAT]",
		},
		{
			// Real OAuth tokens are gho_ + exactly 36 alphanumerics (gitleaks github-oauth).
			name:     "GitHub OAuth token",
			input:    "token=gho_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			contains: "[REDACTED:GITHUB_OAUTH]",
		},
		{
			// Real app/Actions tokens are ghs_ + exactly 36 alphanumerics (gitleaks github-app-token).
			name:     "GitHub Actions token",
			input:    "GITHUB_TOKEN=ghs_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij",
			contains: "[REDACTED:GITHUB_APP_TOKEN]",
		},
		{
			name:     "Groq API key",
			input:    "GROQ_API_KEY=\"gsk_TestFakeKeyForUnitTestingPurposesOnlyNotARealKeyXXXXXXXXXX\"",
			contains: "[REDACTED:GROQ_API_KEY]",
		},
		{
			name:     "Anthropic API key",
			input:    "key: sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
			contains: "[REDACTED:ANTHROPIC_API_KEY]",
		},
		{
			name:     "OpenAI project key",
			input:    "OPENAI_API_KEY=sk-proj-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwx",
			contains: "[REDACTED:OPENAI_PROJECT_KEY]",
		},
		{
			name:     "OpenAI legacy key",
			input:    "sk-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz01234567",
			contains: "[REDACTED:OPENAI_API_KEY]",
		},
		{
			// Real keys are AIza + exactly 35 chars (gitleaks gcp-api-key).
			name:     "Google API key",
			input:    "key=AIzaSyC0Da1ABCDEFGHIJKLMNOPQRSTUVWXYZab",
			contains: "[REDACTED:GCP_API_KEY]",
		},
		{
			name:     "AWS access key ID",
			input:    "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
			contains: "[REDACTED:AWS_ACCESS_TOKEN]",
		},
		{
			// Temporary session keys (ASIA prefix) — covered by gitleaks
			// aws-access-token, missed by the old hand-written AKIA-only rule.
			name:     "AWS temporary session key",
			input:    "AWS_ACCESS_KEY_ID=ASIAIOSFODNN7EXAMPLE",
			contains: "[REDACTED:AWS_ACCESS_TOKEN]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactContent(tt.input, nil)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("RedactContent(%q) = %q, want it to contain %q", tt.input, got, tt.contains)
			}
			// Original secret text should be gone
			if got == tt.input {
				t.Errorf("RedactContent(%q): content was not modified", tt.input)
			}
		})
	}
}

func TestRedactContent_MultipleSecretsInOneString(t *testing.T) {
	input := "PAT: ghp_FakePATTokenForTestingOnlyNotRealXXX and groq: gsk_TestFakeKeyForUnitTestingPurposesOnlyNotARealKeyXXXXXXXXXX"
	got := RedactContent(input, nil)
	if !strings.Contains(got, "[REDACTED:GITHUB_PAT]") {
		t.Errorf("expected GITHUB_PAT redacted, got: %q", got)
	}
	if !strings.Contains(got, "[REDACTED:GROQ_API_KEY]") {
		t.Errorf("expected GROQ_API_KEY redacted, got: %q", got)
	}
}

func TestRedactContent_NoSecrets(t *testing.T) {
	input := "This is a normal conversation with no secrets."
	got := RedactContent(input, nil)
	if got != input {
		t.Errorf("RedactContent(%q) = %q, want unchanged", input, got)
	}
}

func TestRedactContent_CustomPattern(t *testing.T) {
	input := "my-token-abc123DEF456ghi789JKL012"
	got := RedactContent(input, []string{`my-token-[A-Za-z0-9]{24,}`})
	if !strings.Contains(got, "[REDACTED:custom]") {
		t.Errorf("RedactContent with custom pattern: got %q, want [REDACTED:custom]", got)
	}
}

func TestRedactContent_InvalidCustomPattern(t *testing.T) {
	// Invalid regex should be skipped, not panic
	input := "some content with no-token-12345"
	got := RedactContent(input, []string{`[invalid(`})
	// Content should be unchanged since the pattern was invalid
	if got != input {
		t.Errorf("RedactContent with invalid pattern: got %q, want unchanged %q", got, input)
	}
}

func TestRedactContent_AnthropicNotMatchedByOpenAI(t *testing.T) {
	// sk-ant- should be caught by ANTHROPIC_API_KEY, not OPENAI_API_KEY
	input := "sk-ant-api03-ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	got := RedactContent(input, nil)
	if !strings.Contains(got, "[REDACTED:ANTHROPIC_API_KEY]") {
		t.Errorf("expected ANTHROPIC_API_KEY, got: %q", got)
	}
	if strings.Contains(got, "[REDACTED:OPENAI_API_KEY]") {
		t.Errorf("Anthropic key should not match OPENAI_API_KEY pattern, got: %q", got)
	}
}

func TestRedactContent_EmptyInput(t *testing.T) {
	got := RedactContent("", nil)
	if got != "" {
		t.Errorf("RedactContent(\"\") = %q, want \"\"", got)
	}
}

// TestRedactContent_GeneratedPatterns exercises one realistic fake token per
// high-value category added by the gitleaks-derived generated rule set. All
// tokens are invented for testing and are not real secrets.
func TestRedactContent_GeneratedPatterns(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Slack bot token",
			input:    "SLACK_TOKEN=xoxb-1234567890123-1234567890123-AbCdEfGhIjKlMnOpQrStUvWx",
			contains: "[REDACTED:SLACK_BOT_TOKEN]",
		},
		{
			name:     "Stripe live secret key",
			input:    "stripe: sk_live_FakeStripeKey12345678901234",
			contains: "[REDACTED:STRIPE_ACCESS_TOKEN]",
		},
		{
			name:     "SendGrid API token",
			input:    "SENDGRID_API_KEY='SG.abcdefghijklmnopqrstuvwxyz0123456789_-=.abcdefghijklmnopqrstuvwxyz'",
			contains: "[REDACTED:SENDGRID_API_TOKEN]",
		},
		{
			name:     "Twilio API key",
			input:    "twilio key SK0123456789abcdef0123456789abcdef",
			contains: "[REDACTED:TWILIO_API_KEY]",
		},
		{
			name:     "npm access token",
			input:    "//registry.npmjs.org/:_authToken=npm_abcdefghijklmnopqrstuvwxyz0123456789",
			contains: "[REDACTED:NPM_ACCESS_TOKEN]",
		},
		{
			name:     "PyPI upload token",
			input:    "password: pypi-AgEIcHlwaS5vcmcABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz01",
			contains: "[REDACTED:PYPI_UPLOAD_TOKEN]",
		},
		{
			name: "PEM private key block",
			input: "-----BEGIN RSA PRIVATE KEY-----\n" +
				"MIIFakeKeyMaterialForTestingOnlyNotARealKeyAbCdEfGhIjKlMnOpQrStUvWxYz012345\n" +
				"6789FakeKeyMaterialForTestingOnlyNotARealKey==\n" +
				"-----END RSA PRIVATE KEY-----",
			contains: "[REDACTED:PRIVATE_KEY]",
		},
		{
			name:     "JWT",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkZha2UifQ.abc123DEF456ghi789JKL012mno345PQR",
			contains: "[REDACTED:JWT]",
		},
		{
			name:     "Azure AD client secret",
			input:    "client_secret= abc8Q~FakeAzureSecretValue0123456789-_",
			contains: "[REDACTED:AZURE_AD_CLIENT_SECRET]",
		},
		{
			// Anthropic OAuth tokens (sk-ant-oat01-…) are missed by gitleaks'
			// exact-format rules; the hand-written superset must catch them.
			name:     "Anthropic OAuth token",
			input:    "token: sk-ant-oat01-FakeOAuthTokenForTestingOnlyNotReal0123456789",
			contains: "[REDACTED:ANTHROPIC_API_KEY]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactContent(tt.input, nil)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("RedactContent(%q) = %q, want it to contain %q", tt.input, got, tt.contains)
			}
		})
	}
}

// TestRedactContent_GroupReplacementKeepsContext verifies that context-style
// rules (secret in a capture group) redact only the secret and preserve the
// surrounding text the pattern needed in order to match.
func TestRedactContent_GroupReplacementKeepsContext(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		want       string // exact expected output
	}{
		{
			name:  "AWS secret access key keeps assignment context",
			input: `aws_secret_access_key = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYFAKEKEY123"`,
			want:  `aws_secret_access_key = "[REDACTED:AWS_SECRET_ACCESS_KEY]"`,
		},
		{
			name:  "connection string keeps everything but the password",
			input: "DATABASE_URL=postgres://admin:s3cretPW@db.example.com:5432/mydb",
			want:  "DATABASE_URL=postgres://admin:[REDACTED:URL_PASSWORD]@db.example.com:5432/mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactContent(tt.input, nil); got != tt.want {
				t.Errorf("RedactContent(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRedactContent_NoFalsePositives guards look-alike strings that must NOT be
// redacted — over-redaction mangles saved transcripts.
func TestRedactContent_NoFalsePositives(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "short sk- word", input: "the sk-test flag enables sandbox mode"},
		{name: "URL without password", input: "see https://docs.example.com/path?q=1 for details"},
		{name: "SK prefix that is not Twilio", input: "ticket SKI2345678 was closed"},
		{name: "ordinary prose about keys", input: "the api key setting lives in config.toml under [auth]"},
		{name: "npm package name", input: "run npm_config_registry=https://example.com npm install"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactContent(tt.input, nil); got != tt.input {
				t.Errorf("RedactContent(%q) = %q, want unchanged", tt.input, got)
			}
		})
	}
}
