package config

import "strings"

const redactedMask = "****"

// Redacted returns a copy of the raw configuration safe to display in
// diagnostics and logs (SPEC.md §11, §16). Env-reference values keep their
// ${VAR} form (which never exposes a secret); any literal auth value is masked.
func (l *Loaded) Redacted() *Config {
	if l == nil || l.Raw == nil {
		return nil
	}
	out := *l.Raw
	out.Servers = make(map[string]ServerConfig, len(l.Raw.Servers))
	for id, s := range l.Raw.Servers {
		if s.Auth != nil {
			auth := *s.Auth
			auth.Value = redactValue(auth.Value)
			s.Auth = &auth
		}
		out.Servers[id] = s
	}
	return &out
}

// redactValue leaves env references intact (they are not secrets) and masks any
// non-empty literal value so a real secret is never rendered.
func redactValue(v string) string {
	if v == "" {
		return ""
	}
	if strings.Contains(v, "${") {
		return v
	}
	return redactedMask
}
