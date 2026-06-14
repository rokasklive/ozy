package installer

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgressPlainNoANSI(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, Platform{}) // not a TTY → plain fallback
	p.Start("Build")
	p.Done("Build")
	p.Skip("Config")
	p.Fail("PATH")

	out := buf.String()
	if strings.Contains(out, "\033[") {
		t.Errorf("non-TTY progress must not emit ANSI escapes:\n%q", out)
	}
	for _, want := range []string{"Build", "Config", "already done", "✗ PATH"} {
		if !strings.Contains(out, want) {
			t.Errorf("progress output missing %q:\n%s", want, out)
		}
	}
}

func TestProgressColorOnTTY(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf, Platform{TTY: true, Color: true})
	p.Done("Step")
	if !strings.Contains(buf.String(), "\033[32m") {
		t.Errorf("colour TTY should emit ANSI colour, got %q", buf.String())
	}
}
