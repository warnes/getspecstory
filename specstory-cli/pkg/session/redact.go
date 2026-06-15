package session

import (
	"fmt"
	"log/slog"
	"regexp"
)

type redactPattern struct {
	re    *regexp.Regexp
	label string
}

// builtinPatterns covers common API keys and tokens from popular providers.
// Patterns are ordered from most-specific to least-specific so that longer
// prefixes (e.g. sk-ant-) are matched before shorter ones (e.g. sk-).
var builtinPatterns = []redactPattern{
	// GitHub tokens — ordered longest-prefix first so gghp_ is matched before ghp_.
	// \b prevents ghp_ from matching inside gghp_ (no word boundary between the two g's).
	{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), "GITHUB_FINE_GRAINED_PAT"},
	{regexp.MustCompile(`\bgghp_[A-Za-z0-9]{20,}`), "GITHUB_MODELS_TOKEN"},
	{regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}`), "GITHUB_PAT"},
	{regexp.MustCompile(`\bgho_[A-Za-z0-9]{20,}`), "GITHUB_OAUTH_TOKEN"},
	{regexp.MustCompile(`\bghs_[A-Za-z0-9]{20,}`), "GITHUB_ACTIONS_TOKEN"},
	{regexp.MustCompile(`\bgsk_[A-Za-z0-9]{20,}`), "GROQ_API_KEY"},
	// OpenAI/Anthropic — sk-ant- and sk-proj- before the generic sk- prefix.
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`), "ANTHROPIC_API_KEY"},
	{regexp.MustCompile(`sk-proj-[A-Za-z0-9_\-]{20,}`), "OPENAI_PROJECT_KEY"},
	{regexp.MustCompile(`sk-[A-Za-z0-9]{40,}`), "OPENAI_API_KEY"},
	{regexp.MustCompile(`AIza[A-Za-z0-9_\-]{35,}`), "GOOGLE_API_KEY"},
	{regexp.MustCompile(`AKIA[A-Z0-9]{16}`), "AWS_ACCESS_KEY_ID"},
}

// RedactContent replaces known secret patterns in content with labelled placeholders
// of the form [REDACTED:<LABEL>]. Built-in patterns cover common API keys for GitHub,
// Groq, OpenAI, Anthropic, Google, and AWS. extraPatterns adds caller-supplied Go
// regular expressions; invalid patterns are logged and skipped.
func RedactContent(content string, extraPatterns []string) string {
	patterns := make([]redactPattern, len(builtinPatterns))
	copy(patterns, builtinPatterns)

	for _, p := range extraPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			slog.Warn("Invalid redaction pattern, skipping", "pattern", p, "error", err)
			continue
		}
		patterns = append(patterns, redactPattern{re, "custom"})
	}

	for _, p := range patterns {
		replacement := fmt.Sprintf("[REDACTED:%s]", p.label)
		content = p.re.ReplaceAllString(content, replacement)
	}
	return content
}
