package bench

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// walkFiles returns a map from relative path (slash-separated, rooted at dir)
// to hex-encoded SHA-256 for every regular file under dir, excluding .git.
func walkFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("filepath.Rel(%q, %q): %w", dir, path, err)
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}
		h := sha256.Sum256(data)
		m[rel] = fmt.Sprintf("%x", h)
		return nil
	})
	if err != nil {
		t.Fatalf("walkFiles(%q): %v", dir, err)
	}
	return m
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFixtureDeterministic(t *testing.T) {
	t.Parallel()

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	meta1, err := GenerateFixture(dir1)
	if err != nil {
		t.Fatalf("GenerateFixture run 1: %v", err)
	}
	meta2, err := GenerateFixture(dir2)
	if err != nil {
		t.Fatalf("GenerateFixture run 2: %v", err)
	}

	// a. Culprit subjects must match.
	if meta1.CulpritSubject != meta2.CulpritSubject {
		t.Errorf("culprit subject mismatch: %q vs %q",
			meta1.CulpritSubject, meta2.CulpritSubject)
	}

	// b. File trees (excluding .git) must have identical paths and sha256s.
	files1 := walkFiles(t, dir1)
	files2 := walkFiles(t, dir2)

	if len(files1) == 0 {
		t.Fatal("run 1 file tree is empty")
	}
	if len(files2) == 0 {
		t.Fatal("run 2 file tree is empty")
	}

	for rel, sha1 := range files1 {
		sha2, ok := files2[rel]
		if !ok {
			t.Errorf("file %q present in run 1 but missing in run 2", rel)
			continue
		}
		if sha1 != sha2 {
			t.Errorf("file %q sha256 mismatch: %s (run1) vs %s (run2)",
				rel, sha1, sha2)
		}
	}
	for rel := range files2 {
		if _, ok := files1[rel]; !ok {
			t.Errorf("file %q present in run 2 but missing in run 1", rel)
		}
	}
}

func TestFixtureCulpritDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	meta, err := GenerateFixture(dir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	cmd := exec.Command("git", "show", meta.CulpritHash)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git show %s: %v", meta.CulpritHash, err)
	}

	diff := string(out)

	if !strings.Contains(diff, "SUSPENDED") {
		t.Errorf("culprit diff does not contain SUSPENDED")
	}
	// The diff should show SUSPENDED mapping to ACTIVE (the bug).
	if !strings.Contains(diff, "ACTIVE") {
		t.Errorf("culprit diff does not contain ACTIVE")
	}
}

func TestFixtureGroundTruthPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	_, err := GenerateFixture(dir)
	if err != nil {
		t.Fatalf("GenerateFixture: %v", err)
	}

	required := []string{
		"src/main/java/com/acme/billing/AccountStatus.java",
		"src/main/java/com/acme/billing/StatusMapper.java",
		"src/main/java/com/acme/billing/InvoiceEligibilityService.java",
		"src/main/java/com/acme/billing/BillingRunProcessor.java",
		"src/test/java/com/acme/billing/BillingRunProcessorTest.java",
		"docs/account-lifecycle.md",
	}

	for _, p := range required {
		full := filepath.Join(dir, p)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("required file %q missing: %v", p, err)
		}
	}

	// Verify doc contains the required statement.
	data, err := os.ReadFile(filepath.Join(dir, "docs", "account-lifecycle.md"))
	if err != nil {
		t.Fatalf("read account-lifecycle.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "SUSPENDED accounts are not invoice eligible") {
		t.Errorf("account-lifecycle.md does not declare SUSPENDED accounts are not invoice eligible")
	}
	if !strings.Contains(content, "NOT invoice eligible") {
		t.Errorf("account-lifecycle.md does not state NOT invoice eligible for SUSPENDED")
	}
}
