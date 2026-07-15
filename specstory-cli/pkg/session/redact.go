package session

//go:generate go run ../../tools/gen-redact-patterns

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	gosync "sync"
)

// redactPattern is one redaction rule. group selects which regexp capture group
// holds the secret: 0 redacts the whole match; N redacts only group N, preserving
// the surrounding context the pattern needed in order to match (gitleaks-style
// context rules put the secret in group 1). keywords is the gitleaks-style regex
// prefilter: when non-empty, the regex runs only if one of these lowercase
// literals appears in the (lowercased) content — without this, running 200+
// regexes over every saved session is pathologically slow on large transcripts.
type redactPattern struct {
	re       *regexp.Regexp
	label    string
	group    int
	keywords []string
}

// builtinPatterns holds hand-maintained rules that must run BEFORE the generated
// gitleaks set: either because no gitleaks rule covers the token family, or
// because the gitleaks rule is deliberately narrower than what a redactor needs.
// A redactor's error asymmetry differs from a scanner's — a missed secret leaks
// into saved history, while an over-match merely over-redacts a transcript — so
// where a looser pattern is a strict superset of the gitleaks rule it stays here,
// with the reason.
//
// Equivalent-or-narrower predecessors of these rules were removed in favor of the
// generated gitleaks set: GITHUB_FINE_GRAINED_PAT, GITHUB_PAT, GITHUB_OAUTH_TOKEN,
// GITHUB_ACTIONS_TOKEN (gitleaks adds exact lengths and the ghu_ prefix),
// GOOGLE_API_KEY (now GCP_API_KEY), and AWS_ACCESS_KEY_ID (now AWS_ACCESS_TOKEN,
// which adds ASIA/A3T/ABIA/ACCA prefixes).
var builtinPatterns = []redactPattern{
	// No gitleaks rule covers GitHub Models tokens. Must also run before the
	// generated github-pat rule: that rule has no leading \b, so it would match
	// the ghp_… suffix inside a gghp_… token and leave the leading g behind.
	{re: regexp.MustCompile(`\bgghp_[A-Za-z0-9]{20,}`), label: "GITHUB_MODELS_TOKEN", keywords: []string{"gghp_"}},
	// No gitleaks rule covers Groq keys (v8.30.1).
	{re: regexp.MustCompile(`\bgsk_[A-Za-z0-9]{20,}`), label: "GROQ_API_KEY", keywords: []string{"gsk_"}},
	// Deliberately looser than gitleaks' anthropic rules, which match only the
	// sk-ant-api03-/sk-ant-admin01- formats. Anthropic issues other token kinds
	// with the same prefix (e.g. sk-ant-oat01- OAuth tokens, which Claude Code
	// transcripts routinely contain); this superset catches them all.
	{re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`), label: "ANTHROPIC_API_KEY", keywords: []string{"sk-ant-"}},
	// Deliberately looser than gitleaks' openai-api-key, which requires the
	// T3BlbkFJ marker and proj/svcacct/admin infix; this also catches project
	// keys of other vintages.
	{re: regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_-]{20,}`), label: "OPENAI_PROJECT_KEY", keywords: []string{"sk-proj-"}},
}

// fallbackPatterns holds hand-maintained rules that must run AFTER the generated
// gitleaks set, because they are broader than (and would otherwise shadow) more
// specific generated rules that should win the label.
var fallbackPatterns = []redactPattern{
	// Legacy OpenAI keys (no proj/svcacct/admin infix) that gitleaks'
	// marker-based rule misses. Runs after the generated set so the precise
	// openai-api-key rule labels modern keys first.
	{re: regexp.MustCompile(`\bsk-[A-Za-z0-9]{40,}`), label: "OPENAI_API_KEY", keywords: []string{"sk-"}},
	// AWS secret access keys have no distinctive prefix, so a bare 40-char
	// pattern would over-match; require the assignment-style context gitleaks
	// leaves to its (skipped) generic rule. The secret is group 1.
	{re: regexp.MustCompile(`(?i)aws[_\-\s.]{0,4}secret[_\-\s.]{0,4}(?:access[_\-\s.]{0,4})?key[_\-\s.]{0,10}[=:]\s*["']?([A-Za-z0-9/+=]{40})\b`), label: "AWS_SECRET_ACCESS_KEY", group: 1, keywords: []string{"aws"}},
	// Passwords embedded in connection-string URLs (postgres://user:pass@host,
	// mysql://…, https://user:token@host, …). Only the password (group 1) is
	// redacted so the rest of the URL stays readable.
	{re: regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^:/\s@'"]+:([^@\s'"]{3,})@`), label: "URL_PASSWORD", group: 1, keywords: []string{"://"}},
}

// compiledExtraCache caches compiled regexps for caller-supplied pattern strings.
// Keys are pattern strings; values are *regexp.Regexp or nil (for invalid patterns).
var compiledExtraCache gosync.Map

// Package-level redaction configuration, applied by GenerateMarkdownFromAgentSession
// so that EVERY markdown emission path is redacted by construction — sync's bulk
// path, --print, and the TUI previews all generate markdown without going through
// ProcessSingleSession, and each of them leaked secrets when redaction lived only
// there. ProcessSingleSession keeps its own explicit pass as a second layer
// (idempotent: placeholders don't re-match). Set once at startup, before any
// command runs; not synchronized.
var (
	redactionEnabled       = true
	redactionExtraPatterns []string
)

// ConfigureRedaction sets the process-wide redaction behavior from the resolved
// flag/config values. Call once during startup, before commands execute.
func ConfigureRedaction(enabled bool, extraPatterns []string) {
	redactionEnabled = enabled
	redactionExtraPatterns = extraPatterns
}

// redactIfEnabled applies the configured redaction to generated markdown.
func redactIfEnabled(content string) string {
	if !redactionEnabled {
		return content
	}
	return RedactContent(content, redactionExtraPatterns)
}

// RedactContent replaces known secret patterns in content with labelled placeholders
// of the form [REDACTED:<LABEL>]. Built-in coverage is the gitleaks default ruleset
// (see redact_patterns_gen.go) plus hand-maintained rules for token families gitleaks
// misses. extraPatterns adds caller-supplied Go regular expressions; invalid patterns
// are logged and skipped.
func RedactContent(content string, extraPatterns []string) string {
	// The keyword prefilter needs a lowercased view of the content. It is
	// recomputed only when a pattern actually changed the content, so the
	// common case (no secrets) lowercases exactly once.
	lower := strings.ToLower(content)

	apply := func(patterns []redactPattern) {
		for _, p := range patterns {
			if !p.keywordsPresent(lower) {
				continue
			}
			if redacted := p.apply(content); redacted != content {
				content = redacted
				lower = strings.ToLower(content)
			}
		}
	}

	// Order matters: hand-written specific rules, then the generated gitleaks
	// set, then hand-written broad fallbacks, then caller extras. Earlier rules
	// win the label for overlapping token shapes.
	apply(builtinPatterns)
	apply(generatedPatterns)
	apply(fallbackPatterns)
	apply(compileExtras(extraPatterns))
	return content
}

// keywordsPresent reports whether the pattern's regex should run at all: true
// when the pattern declares no keywords (always run — e.g. caller extras), or
// when any keyword occurs in the lowercased content.
func (p redactPattern) keywordsPresent(lower string) bool {
	if len(p.keywords) == 0 {
		return true
	}
	for _, kw := range p.keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// apply replaces this pattern's matches in content with the labelled placeholder.
// For group > 0 only that capture group's span is replaced, so the context the
// pattern matched around the secret survives in the output.
func (p redactPattern) apply(content string) string {
	replacement := fmt.Sprintf("[REDACTED:%s]", p.label)
	if p.group == 0 {
		return p.re.ReplaceAllString(content, replacement)
	}

	matches := p.re.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := -1, -1
		if 2*p.group+1 < len(m) {
			start, end = m[2*p.group], m[2*p.group+1]
		}
		if start < 0 {
			// The secret group did not participate in this match; redact the
			// whole match rather than risk leaving the secret in place.
			start, end = m[0], m[1]
		}
		b.WriteString(content[last:start])
		b.WriteString(replacement)
		last = end
	}
	b.WriteString(content[last:])
	return b.String()
}

// compileExtras compiles caller-supplied pattern strings, caching compilations
// (including failures) across calls.
func compileExtras(extraPatterns []string) []redactPattern {
	if len(extraPatterns) == 0 {
		return nil
	}
	extra := make([]redactPattern, 0, len(extraPatterns))
	for _, p := range extraPatterns {
		var re *regexp.Regexp
		if cached, ok := compiledExtraCache.Load(p); ok {
			if cached != nil {
				re = cached.(*regexp.Regexp)
			}
		} else {
			compiled, err := regexp.Compile(p)
			if err != nil {
				slog.Warn("Invalid redaction pattern, skipping", "pattern", p, "error", err)
				compiledExtraCache.Store(p, nil)
				continue
			}
			compiledExtraCache.Store(p, compiled)
			re = compiled
		}
		if re != nil {
			extra = append(extra, redactPattern{re: re, label: "custom"})
		}
	}
	return extra
}
