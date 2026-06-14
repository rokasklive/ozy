package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const redactedMask = "****"

var (
	bearerRe = regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/=-]+`)
	// key/value pairs whose key name suggests a credential, in `k=v`, `k: v`, or
	// `--k v` form. The value (\S+) is masked; the key and separator are kept.
	secretKVRe = regexp.MustCompile(`(?i)\b(token|secret|password|authorization|api[_-]?key|apikey)([=:]\s*|\s+)\S+`)
)

// Redact masks obvious credential patterns in s. It is a defense-in-depth net,
// not a guarantee.
// ponytail: pattern-based masking; the real guarantee is structural — callers
// log structured fields and never hand raw config contents or env values to the
// logger. Tighten the patterns only if a concrete leak vector shows up.
func Redact(s string) string {
	s = bearerRe.ReplaceAllString(s, "$1 "+redactedMask)
	s = secretKVRe.ReplaceAllString(s, "$1$2"+redactedMask)
	return s
}

// Logger writes a durable, redacted file log for one install/uninstall run and
// streams friendly lines to the terminal. The file log is detailed for
// debugging; the terminal stream is for humans. Secrets are never written.
type Logger struct {
	file    io.WriteCloser
	term    io.Writer
	path    string
	Verbose bool // when set, Logf detail also streams to the terminal (--verbose)
}

// NewLogger opens a timestamped log file under dir (kind is "install" or
// "uninstall") and streams human-facing lines to term (may be nil).
func NewLogger(dir, kind string, term io.Writer) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s-%s.log", kind, time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // Ozy-owned log path
	if err != nil {
		return nil, err
	}
	return &Logger{file: f, term: term, path: path}, nil
}

// Path returns the log file path, printed at the end of every run.
func (l *Logger) Path() string { return l.path }

// Logf writes a timestamped, redacted line to the file log. Under --verbose the
// same line also streams to the terminal.
func (l *Logger) Logf(format string, args ...any) {
	line := Redact(fmt.Sprintf(format, args...))
	if l.Verbose && l.term != nil {
		fmt.Fprintln(l.term, line)
	}
	l.writeFile(line)
}

// Sayf writes a redacted line to both the terminal and the file log.
func (l *Logger) Sayf(format string, args ...any) {
	line := Redact(fmt.Sprintf(format, args...))
	if l.term != nil {
		fmt.Fprintln(l.term, line)
	}
	l.writeFile(line)
}

// Log satisfies the sidecar.Logger interface so the installer's logger captures
// provisioning output in the same file.
func (l *Logger) Log(msg string) { l.Logf("%s", msg) }

// Close flushes and closes the log file.
func (l *Logger) Close() error { return l.file.Close() }

func (l *Logger) writeFile(line string) {
	fmt.Fprintf(l.file, "%s %s\n", time.Now().UTC().Format(time.RFC3339), line)
}
