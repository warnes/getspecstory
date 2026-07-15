package session

import (
	"strings"
	"testing"
)

// BenchmarkRedactContent_LargeCleanDocument models the hot path that made bulk
// sync pathologically slow before the keyword prefilter: a large transcript
// containing no secrets, where every one of the 220+ patterns used to run its
// full regex over the whole document.
func BenchmarkRedactContent_LargeCleanDocument(b *testing.B) {
	// ~1 MB of ordinary transcript-like prose, no secret keywords.
	doc := strings.Repeat("The user asked about refactoring the parser and the agent replied with a plan. ", 13000)
	b.SetBytes(int64(len(doc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RedactContent(doc, nil)
	}
}

// BenchmarkRedactContent_LargeDocumentWithSecret measures the cost when a
// keyword IS present, forcing one regex to run over the full document.
func BenchmarkRedactContent_LargeDocumentWithSecret(b *testing.B) {
	doc := strings.Repeat("The user asked about refactoring the parser and the agent replied with a plan. ", 13000) +
		"token: ghp_FakeTokenAbCdEfGhIjKlMnOpQrStUvWxYz0"
	b.SetBytes(int64(len(doc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RedactContent(doc, nil)
	}
}
