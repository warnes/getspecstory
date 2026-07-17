package session

import (
	"strings"
	"testing"

	"github.com/specstoryai/getspecstory/specstory-cli/pkg/providers/copilotide"
)

// TestCopilotIDEConversion_RedactsSecrets locks in redaction for a secret that
// enters through the VS Code Copilot IDE provider's OWN reconstruction
// (ConvertToSessionData -> GenerateMarkdownFromAgentSession), not just a
// hand-built SessionData as in TestGenerateMarkdown_RedactsSecrets.
//
// It is the second, provider-anchored layer of the belt-and-suspenders guard on
// the copilot-ide capture path: it fails if a future change to the copilotide
// conversion routes any content (message text, pre-rendered tool markdown, …)
// into the saved history in a way that bypasses generation-level redaction.
//
// The token is invented for this test — 'ghp_' plus 36 made-up alphanumerics
// matching the GITHUB_PAT rule — and is not a real secret.
func TestCopilotIDEConversion_RedactsSecrets(t *testing.T) {
	const fakeToken = "ghp_FakeTokenAbCdEfGhIjKlMnOpQrStUvWxYz0"

	composer := copilotide.VSCodeComposer{
		Host:              "vscode",
		SessionID:         "copilotide-redact-test",
		Version:           3,
		RequesterUsername: "tester",
		Requests: []copilotide.VSCodeRequestBlock{{
			RequestID: "r1",
			Message:   copilotide.VSCodeMessage{Text: "please store this key: " + fakeToken},
		}},
	}

	provider := copilotide.NewProvider(copilotide.VSCode)
	session := provider.ConvertToSessionData(composer, "/tmp/project", nil)

	md, err := GenerateMarkdownFromAgentSession(session.SessionData, false, true)
	if err != nil {
		t.Fatalf("generate markdown from copilotide session: %v", err)
	}
	if strings.Contains(md, fakeToken) {
		t.Errorf("raw copilotide token leaked into generated markdown:\n%s", md)
	}
	if !strings.Contains(md, "[REDACTED:GITHUB_PAT]") {
		t.Errorf("expected [REDACTED:GITHUB_PAT] from the copilotide conversion path:\n%s", md)
	}
}
