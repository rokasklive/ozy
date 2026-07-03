package cli

import (
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

func TestSecretHygiene_FlagsInlineTokenWithoutPrintingIt(t *testing.T) {
	t.Parallel()
	const token = "ghp_ThisIsALeakedTokenValue1234567890"
	raw := &config.Config{MCP: map[string]config.ServerConfig{
		"github": {Type: "local", Environment: map[string]string{
			"GITHUB_PERSONAL_ACCESS_TOKEN": token,
		}},
	}}

	checks := secretHygieneChecks(raw)
	if len(checks) != 1 {
		t.Fatalf("checks = %d, want 1 finding", len(checks))
	}
	c := checks[0]
	if c.Status != contract.CheckWarn || c.Name != "secrets" {
		t.Fatalf("finding = %+v, want a secrets WARN", c)
	}
	for _, want := range []string{"github", "environment", "GITHUB_PERSONAL_ACCESS_TOKEN", "GitHub token", "{env:NAME}", "rotate"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("detail missing %q: %q", want, c.Detail)
		}
	}
	if strings.Contains(c.Detail, "ghp_This") || strings.Contains(c.Detail, token) {
		t.Fatalf("detail leaked the token value: %q", c.Detail)
	}
}

func TestSecretHygiene_EnvReferencesAndCleanValuesPass(t *testing.T) {
	t.Parallel()
	raw := &config.Config{MCP: map[string]config.ServerConfig{
		"atlassian": {Type: "remote", Headers: map[string]string{
			"Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}",
		}},
		"fs": {Type: "local", Environment: map[string]string{
			"WORKSPACE": "/home/user/desk-lamp-project", // sk- inside a word must not fire
		}},
	}}

	if checks := secretHygieneChecks(raw); len(checks) != 0 {
		t.Fatalf("clean config produced findings: %+v", checks)
	}
}

func TestSecretHygiene_BearerLiteralIsFlagged(t *testing.T) {
	t.Parallel()
	raw := &config.Config{MCP: map[string]config.ServerConfig{
		"api": {Type: "remote", Headers: map[string]string{
			"Authorization": "Bearer abc123-literal-credential",
		}},
	}}

	checks := secretHygieneChecks(raw)
	if len(checks) != 1 || !strings.Contains(checks[0].Detail, "bearer credential") {
		t.Fatalf("bearer literal should be flagged, got %+v", checks)
	}
	if strings.Contains(checks[0].Detail, "abc123") {
		t.Fatalf("detail leaked the credential: %q", checks[0].Detail)
	}
}
