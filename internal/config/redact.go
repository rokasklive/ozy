package config

import "strings"

const redactedMask = "****"

// Redacted returns a copy of the raw configuration safe to display in
// diagnostics and logs (SPEC.md §11, §16). Env-reference values keep their
// {env:NAME} form; literal header/environment values are masked.
func (l *Loaded) Redacted() *Config {
	if l == nil || l.Raw == nil {
		return nil
	}
	out := cloneConfig(*l.Raw)
	for id, s := range out.MCP {
		for k, v := range s.Headers {
			s.Headers[k] = redactValue(v)
		}
		for k, v := range s.Environment {
			s.Environment[k] = redactValue(v)
		}
		out.MCP[id] = s
	}
	return &out
}

// redactValue leaves env references intact (they are not resolved secrets) and
// masks any non-empty literal value so a real secret is never rendered.
func redactValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.Contains(v, "{env:") {
		return v
	}
	return redactedMask
}
