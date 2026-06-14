package installer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The PATH block is fenced with these markers so it can be added idempotently
// and removed cleanly on uninstall without touching anything else in the file.
const (
	pathBlockBegin = "# >>> ozy installer >>>"
	pathBlockEnd   = "# <<< ozy installer <<<"
)

// stepConfigurePath makes the bin dir reachable on PATH. It is detection-first:
// an already-reachable binary is a no-op. Otherwise it offers a consent-based
// rc/profile edit (Unix) or user-level PATH edit (Windows), falling back to
// copy-paste instructions when declined or unsafe.
func stepConfigurePath(c *execContext) error {
	if c.paths.Reachable() {
		c.log.Sayf("PATH already includes %s — nothing to do.", c.paths.UserBinDir)
		return nil
	}
	if c.plan.Platform.OS == "windows" {
		return configureWindowsPath(c)
	}
	return configureUnixPath(c)
}

func configureUnixPath(c *execContext) error {
	rc, shell := rcFileFor(os.Getenv("SHELL"), homeDir())
	if !c.allow(Risky, fmt.Sprintf("Append a marked PATH block to %s", rc)) {
		printPathInstructions(c, rc, shell)
		return nil
	}
	if err := appendBlockOnce(rc, pathBlock(c.paths.UserBinDir, shell)); err != nil {
		c.log.Sayf("Could not edit %s automatically: %v", rc, err)
		printPathInstructions(c, rc, shell)
		return nil
	}
	c.log.Sayf("Added a marked PATH block to %s. Open a new shell or run `source %s`.", rc, rc)
	return nil
}

// configureWindowsPath edits the user-level PATH only — never the machine-level
// PATH, which would require admin rights.
func configureWindowsPath(c *execContext) error {
	dir := c.paths.UserBinDir
	if !c.allow(Risky, "Add the bin directory to your user PATH") {
		c.log.Sayf("Add %s to your user PATH via System Settings, or run in PowerShell:", dir)
		c.log.Sayf("  %s", winPathCommand(dir))
		return nil
	}
	out, err := c.run("powershell", "-NoProfile", "-Command", winPathScript(dir))
	if err != nil {
		c.log.Sayf("Automatic PATH edit failed: %v\n%s", err, strings.TrimSpace(out))
		c.log.Sayf("Add %s to your user PATH manually.", dir)
		return nil
	}
	c.log.Sayf("Added %s to your user PATH (takes effect in new shells).", dir)
	return nil
}

// rcFileFor maps the login shell to its rc file and a shell tag for syntax.
func rcFileFor(shellPath, home string) (file, shell string) {
	switch filepath.Base(shellPath) {
	case "zsh":
		return filepath.Join(home, ".zshrc"), "zsh"
	case "bash":
		return filepath.Join(home, ".bashrc"), "bash"
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), "fish"
	default:
		return filepath.Join(home, ".profile"), "sh"
	}
}

func pathExportLine(dir, shell string) string {
	if shell == "fish" {
		return fmt.Sprintf("fish_add_path %s", dir)
	}
	return fmt.Sprintf("export PATH=\"%s:$PATH\"", dir)
}

func pathBlock(dir, shell string) string {
	return fmt.Sprintf("\n%s\n%s\n%s\n", pathBlockBegin, pathExportLine(dir, shell), pathBlockEnd)
}

// appendBlockOnce appends block to file unless the begin marker is already
// present, so reruns never add a duplicate block.
func appendBlockOnce(file, block string) error {
	existing, err := os.ReadFile(file) //nolint:gosec // user rc path
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if strings.Contains(string(existing), pathBlockBegin) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil { //nolint:gosec // shell dirs are world-readable by convention
		return err
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644) //nolint:gosec // rc files are not secret
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(block)
	return err
}

// removePathBlock strips the fenced installer block (and the blank line it adds)
// from file. A file without the markers is left untouched, so it is safe to call
// repeatedly during uninstall.
func removePathBlock(file string) (bool, error) {
	data, err := os.ReadFile(file) //nolint:gosec // user rc path
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	text := string(data)
	start := strings.Index(text, pathBlockBegin)
	end := strings.Index(text, pathBlockEnd)
	if start < 0 || end < 0 || end < start {
		return false, nil
	}
	end += len(pathBlockEnd)
	// Drop the leading newline we inserted before the block, if present.
	if start > 0 && text[start-1] == '\n' {
		start--
	}
	cleaned := text[:start] + text[end:]
	cleaned = strings.TrimRight(cleaned, "\n") + "\n"
	if err := os.WriteFile(file, []byte(cleaned), 0o644); err != nil { //nolint:gosec // rc files are not secret
		return false, err
	}
	return true, nil
}

func printPathInstructions(c *execContext, rc, shell string) {
	c.log.Sayf("Ozy is installed at %s but that directory is not on your PATH.", c.paths.UserBinDir)
	c.log.Sayf("Add this line to %s:", rc)
	c.log.Sayf("  %s", pathExportLine(c.paths.UserBinDir, shell))
}

func winPathScript(dir string) string {
	return fmt.Sprintf(
		"[Environment]::SetEnvironmentVariable('PATH', "+
			"[Environment]::GetEnvironmentVariable('PATH','User') + ';%s', 'User')", dir)
}

func winPathCommand(dir string) string {
	return "powershell -NoProfile -Command \"" + winPathScript(dir) + "\""
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
