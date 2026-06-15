package bench

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// FixtureMeta holds resolved metadata after generating the acme-billing fixture.
type FixtureMeta struct {
	CulpritHash    string
	CulpritSubject string
	TargetDir      string
}

// GenerateIncidentDB creates the read-only incident SQLite database with rows
// for suspended accounts incorrectly invoiced on 2026-06-14.
func GenerateIncidentDB(targetDir string) error {
	dbPath := targetDir + "/db/incident.sqlite"
	if err := os.MkdirAll(targetDir+"/db", 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=rwc")
	if err != nil {
		return fmt.Errorf("open incident db: %w", err)
	}
	defer db.Close()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS invoices (
			id INTEGER PRIMARY KEY,
			account_id TEXT NOT NULL,
			account_name TEXT NOT NULL,
			status TEXT NOT NULL,
			amount REAL NOT NULL,
			invoice_date TEXT NOT NULL,
			billing_run_id TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS billing_runs (
			id TEXT PRIMARY KEY,
			run_date TEXT NOT NULL,
			status TEXT NOT NULL,
			total_invoices INTEGER NOT NULL,
			error_count INTEGER NOT NULL
		)`,
		`INSERT INTO billing_runs VALUES ('BR-20260614-001', '2026-06-14', 'COMPLETED', 1503, 0)`,
		`INSERT INTO invoices VALUES (1, 'ACC-0042', 'Acme Corp', 'SUSPENDED', 1250.00, '2026-06-14', 'BR-20260614-001')`,
		`INSERT INTO invoices VALUES (2, 'ACC-0198', 'Globex Inc', 'SUSPENDED', 3400.50, '2026-06-14', 'BR-20260614-001')`,
		`INSERT INTO invoices VALUES (3, 'ACC-0301', 'Initech', 'SUSPENDED', 875.25, '2026-06-14', 'BR-20260614-001')`,
		`INSERT INTO invoices VALUES (4, 'ACC-0555', 'Umbrella Co', 'ACTIVE', 2200.00, '2026-06-14', 'BR-20260614-001')`,
		`INSERT INTO invoices VALUES (5, 'ACC-0812', 'Hooli LLC', 'SUSPENDED', 5100.00, '2026-06-14', 'BR-20260614-001')`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("exec incident db statement: %w", err)
		}
	}

	return nil
}

// fixedGitEnv returns the pinned git author/committer identity and date for
// deterministic history. The caller must also set Dir on the *exec.Cmd so
// git operates in the target repository.
func fixedGitEnv(date string) []string {
	return []string{
		"GIT_AUTHOR_NAME=Acme CI",
		"GIT_AUTHOR_EMAIL=ci@acme.example.com",
		"GIT_COMMITTER_NAME=Acme CI",
		"GIT_COMMITTER_EMAIL=ci@acme.example.com",
		"GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_DATE=" + date,
	}
}

// git runs "git" with args and optional env in dir. It inherits PATH from the
// parent but overrides env vars listed in extraEnv.
func git(dir string, extraEnv []string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// gitString runs "git" and returns the trimmed first line of stdout.
func gitString(dir string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// srcDir returns the path to the Java source tree under targetDir.
func srcDir(targetDir string) string {
	return filepath.Join(targetDir, "src", "main", "java", "com", "acme", "billing")
}

// testDir returns the path to the Java test tree under targetDir.
func testDir(targetDir string) string {
	return filepath.Join(targetDir, "src", "test", "java", "com", "acme", "billing")
}

// docsDir returns the path to the docs directory under targetDir.
func docsDir(targetDir string) string {
	return filepath.Join(targetDir, "docs")
}

// writeFile creates a file with content under targetDir. It creates parent
// directories as needed.
func writeFile(targetDir, relPath, content string) error {
	full := filepath.Join(targetDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", relPath, err)
	}
	return nil
}

// GenerateFixture creates the deterministic acme-billing fixture in targetDir.
// It writes Java source files, documentation, initializes a git repository,
// creates a multi-commit history, resolves the culprit commit hash, and
// returns FixtureMeta.
func GenerateFixture(targetDir string) (*FixtureMeta, error) {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir targetDir: %w", err)
	}

	// -----------------------------------------------------------------------
	// Commit 1 — "Initial project structure"
	// -----------------------------------------------------------------------
	if err := writeAccountStatus(targetDir); err != nil {
		return nil, err
	}
	if err := writeDocsCommit1(targetDir); err != nil {
		return nil, err
	}

	if err := git(targetDir, nil, "init"); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}
	// Use a branch name for deterministic history.
	if err := git(targetDir, nil, "checkout", "-b", "main"); err != nil {
		return nil, fmt.Errorf("git checkout: %w", err)
	}

	env1 := fixedGitEnv("2026-06-01T10:00:00+0000")
	if err := git(targetDir, env1, "add", "-A"); err != nil {
		return nil, fmt.Errorf("git add commit 1: %w", err)
	}
	if err := git(targetDir, env1, "commit", "-m", "Initial project structure"); err != nil {
		return nil, fmt.Errorf("git commit 1: %w", err)
	}

	// -----------------------------------------------------------------------
	// Commit 2 (CULPRIT) — "Normalize account status mapping"
	// -----------------------------------------------------------------------
	if err := writeStatusMapper(targetDir); err != nil {
		return nil, err
	}
	if err := writeInvoiceEligibilityService(targetDir); err != nil {
		return nil, err
	}

	env2 := fixedGitEnv("2026-06-10T10:00:00+0000")
	if err := git(targetDir, env2, "add", "-A"); err != nil {
		return nil, fmt.Errorf("git add commit 2: %w", err)
	}
	if err := git(targetDir, env2, "commit", "-m", "Normalize account status mapping"); err != nil {
		return nil, fmt.Errorf("git commit 2: %w", err)
	}

	// -----------------------------------------------------------------------
	// Commit 3 — "Add billing run processor"
	// -----------------------------------------------------------------------
	if err := writeBillingRunProcessor(targetDir); err != nil {
		return nil, err
	}

	env3 := fixedGitEnv("2026-06-12T10:00:00+0000")
	if err := git(targetDir, env3, "add", "-A"); err != nil {
		return nil, fmt.Errorf("git add commit 3: %w", err)
	}
	if err := git(targetDir, env3, "commit", "-m", "Add billing run processor"); err != nil {
		return nil, fmt.Errorf("git commit 3: %w", err)
	}

	// -----------------------------------------------------------------------
	// Commit 4 — "Add docs and test scaffolding"
	// -----------------------------------------------------------------------
	if err := writeBillingRunProcessorTest(targetDir); err != nil {
		return nil, err
	}
	if err := writeDocsCommit4(targetDir); err != nil {
		return nil, err
	}

	env4 := fixedGitEnv("2026-06-14T10:00:00+0000")
	if err := git(targetDir, env4, "add", "-A"); err != nil {
		return nil, fmt.Errorf("git add commit 4: %w", err)
	}
	if err := git(targetDir, env4, "commit", "-m", "Add docs and test scaffolding"); err != nil {
		return nil, fmt.Errorf("git commit 4: %w", err)
	}

	// -----------------------------------------------------------------------
	// Commit 5 — "docs: update account lifecycle notes" (noise)
	// -----------------------------------------------------------------------
	if err := writeDocsCommit5(targetDir); err != nil {
		return nil, err
	}

	env5 := fixedGitEnv("2026-06-15T10:00:00+0000")
	if err := git(targetDir, env5, "add", "-A"); err != nil {
		return nil, fmt.Errorf("git add commit 5: %w", err)
	}
	if err := git(targetDir, env5, "commit", "-m", "docs: update account lifecycle notes"); err != nil {
		return nil, fmt.Errorf("git commit 5: %w", err)
	}

	// Resolve the culprit commit hash.
	hash, err := gitString(targetDir, nil, "log", "--format=%H", "--grep=Normalize account status mapping")
	if err != nil {
		return nil, fmt.Errorf("resolve culprit hash: %w", err)
	}
	if hash == "" {
		return nil, fmt.Errorf("culprit commit not found in git log")
	}

	// Generate the read-only incident SQLite database.
	if err := GenerateIncidentDB(targetDir); err != nil {
		return nil, fmt.Errorf("generate incident db: %w", err)
	}

	return &FixtureMeta{
		CulpritHash:    hash,
		CulpritSubject: "Normalize account status mapping",
		TargetDir:      targetDir,
	}, nil
}

// ---------------------------------------------------------------------------
// Java source writers
// ---------------------------------------------------------------------------

const accountStatusContent = `package com.acme.billing;

/**
 * Enumeration of customer account statuses in the Acme Billing system.
 *
 * <p>An account moves through the lifecycle ACTIVE → SUSPENDED → CLOSED.
 * Only ACTIVE accounts are eligible for automated invoicing.
 */
public enum AccountStatus {

    /** Account is in good standing and invoice eligible. */
    ACTIVE,

    /** Account has been temporarily suspended; not invoice eligible. */
    SUSPENDED,

    /** Account has been permanently closed; not invoice eligible. */
    CLOSED;
}
`

func writeAccountStatus(targetDir string) error {
	return writeFile(targetDir, filepath.Join("src", "main", "java", "com", "acme", "billing", "AccountStatus.java"), accountStatusContent)
}

const statusMapperContent = `package com.acme.billing;

/**
 * Maps raw external status strings to {@link AccountStatus} values.
 *
 * <p>This mapper is the single point of truth for how upstream status
 * representations are normalized before downstream processing.
 */
public final class StatusMapper {

    private StatusMapper() {
        // utility class — no instances
    }

    /**
     * Convert a raw status string into the canonical {@link AccountStatus}.
     *
     * @param raw the external status string; may be null
     * @return the corresponding enum value, or {@link AccountStatus#CLOSED}
     *         for unrecognised or null input
     */
    public static AccountStatus fromString(String raw) {
        if (raw == null) {
            return AccountStatus.CLOSED;
        }
        switch (raw.trim().toUpperCase()) {
            case "ACTIVE":
                return AccountStatus.ACTIVE;
            case "SUSPENDED":
                return AccountStatus.ACTIVE;  // BUG: maps SUSPENDED to ACTIVE
            case "CLOSED":
                return AccountStatus.CLOSED;
            default:
                return AccountStatus.CLOSED;
        }
    }
}
`

func writeStatusMapper(targetDir string) error {
	return writeFile(targetDir, filepath.Join("src", "main", "java", "com", "acme", "billing", "StatusMapper.java"), statusMapperContent)
}

const invoiceEligibilityServiceContent = `package com.acme.billing;

/**
 * Determines whether an account may receive an invoice during the nightly
 * billing run.
 *
 * <p>This service encodes the business rule that only ACTIVE accounts are
 * eligible; SUSPENDED and CLOSED accounts must be skipped.
 */
public final class InvoiceEligibilityService {

    private InvoiceEligibilityService() {
        // utility class — no instances
    }

    /**
     * Returns {@code true} if the given account status permits invoicing.
     *
     * <p>SUSPENDED and CLOSED accounts are <em>not</em> eligible.
     *
     * @param status the account status to check
     * @return {@code true} only for {@link AccountStatus#ACTIVE}
     */
    public static boolean isEligible(AccountStatus status) {
        if (status == null) {
            return false;
        }
        return status == AccountStatus.ACTIVE;
    }
}
`

func writeInvoiceEligibilityService(targetDir string) error {
	return writeFile(targetDir, filepath.Join("src", "main", "java", "com", "acme", "billing", "InvoiceEligibilityService.java"), invoiceEligibilityServiceContent)
}

const billingRunProcessorContent = `package com.acme.billing;

import java.util.Arrays;
import java.util.List;

/**
 * Processes the nightly billing run: reads account statuses, maps them
 * through {@link StatusMapper}, and decides whether to generate an invoice
 * via {@link InvoiceEligibilityService}.
 *
 * <p>This program simulates the logic executed every night by the
 * production billing scheduler. Because of the mapping bug in
 * {@link StatusMapper#fromString(String)}, SUSPENDED accounts are
 * incorrectly treated as ACTIVE and therefore invoiced.
 */
public final class BillingRunProcessor {

    private BillingRunProcessor() {
        // utility class
    }

    /**
     * Entry-point for local simulation.
     *
     * <p>Output shows the chain: raw status → mapped status → eligibility.
     * When the bug is triggered, "SUSPENDED → ACTIVE eligible=true" is
     * printed — a violation of the business rule.
     */
    public static void main(String[] args) {
        List<String> rawStatuses = Arrays.asList(
            "ACTIVE",
            "SUSPENDED",
            "CLOSED",
            "UNKNOWN",
            "   active   "
        );

        for (String raw : rawStatuses) {
            AccountStatus mapped = StatusMapper.fromString(raw);
            boolean eligible = InvoiceEligibilityService.isEligible(mapped);
            System.out.printf("%-15s → %-10s eligible=%b%n",
                raw, mapped, eligible);
        }
    }
}
`

func writeBillingRunProcessor(targetDir string) error {
	return writeFile(targetDir, filepath.Join("src", "main", "java", "com", "acme", "billing", "BillingRunProcessor.java"), billingRunProcessorContent)
}

const billingRunProcessorTestContent = `package com.acme.billing;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/**
 * Unit tests for the billing-run processing chain.
 *
 * <p>The {@link #suspendedAccountsNotInvoiced()} test encodes the business
 * rule that SUSPENDED accounts must not be invoice eligible. With the
 * current bug in {@link StatusMapper#fromString(String)} this test
 * <strong>will fail</strong>.
 */
public class BillingRunProcessorTest {

    @Test
    public void activeAccountsAreInvoiced() {
        AccountStatus status = StatusMapper.fromString("ACTIVE");
        assertTrue(
            "ACTIVE accounts must be invoice eligible",
            InvoiceEligibilityService.isEligible(status));
    }

    @Test
    public void suspendedAccountsNotInvoiced() {
        AccountStatus status = StatusMapper.fromString("SUSPENDED");
        assertFalse(
            "SUSPENDED accounts must not be invoice eligible",
            InvoiceEligibilityService.isEligible(status));
    }

    @Test
    public void closedAccountsNotInvoiced() {
        AccountStatus status = StatusMapper.fromString("CLOSED");
        assertFalse(
            "CLOSED accounts must not be invoice eligible",
            InvoiceEligibilityService.isEligible(status));
    }
}
`

func writeBillingRunProcessorTest(targetDir string) error {
	return writeFile(targetDir, filepath.Join("src", "test", "java", "com", "acme", "billing", "BillingRunProcessorTest.java"), billingRunProcessorTestContent)
}

// ---------------------------------------------------------------------------
// Documentation writers (three revisions for multi-commit history)
// ---------------------------------------------------------------------------

const docsCommit1 = `# Account Lifecycle

## Statuses

- **ACTIVE** — account in good standing, invoice eligible.
- **SUSPENDED** — account temporarily suspended; NOT invoice eligible.
- **CLOSED** — account permanently closed; NOT invoice eligible.

## Business Rule

SUSPENDED accounts are not invoice eligible. The nightly billing run must skip
any account in SUSPENDED status.
`

func writeDocsCommit1(targetDir string) error {
	return writeFile(targetDir, filepath.Join("docs", "account-lifecycle.md"), docsCommit1)
}

const docsCommit4 = `# Account Lifecycle

## Statuses

- **ACTIVE** — account in good standing, invoice eligible.
- **SUSPENDED** — account temporarily suspended; NOT invoice eligible.
- **CLOSED** — account permanently closed; NOT invoice eligible.

## Business Rule

SUSPENDED accounts are not invoice eligible. The nightly billing run must skip
any account in SUSPENDED status.

## Lifecycle Transitions

- ACTIVE → SUSPENDED: triggered by payment failure past the grace period.
- SUSPENDED → ACTIVE: triggered by successful payment of the overdue balance.
- ACTIVE → CLOSED: explicit customer request.
- SUSPENDED → CLOSED: automatic after 90 days without payment.
`

func writeDocsCommit4(targetDir string) error {
	return writeFile(targetDir, filepath.Join("docs", "account-lifecycle.md"), docsCommit4)
}

const docsCommit5 = `# Account Lifecycle

## Statuses

- **ACTIVE** — account in good standing, invoice eligible.
- **SUSPENDED** — account temporarily suspended; NOT invoice eligible.
- **CLOSED** — account permanently closed; NOT invoice eligible.

## Business Rule

SUSPENDED accounts are not invoice eligible. The nightly billing run must skip
any account in SUSPENDED status.

## Lifecycle Transitions

- ACTIVE → SUSPENDED: triggered by payment failure past the grace period.
- SUSPENDED → ACTIVE: triggered by successful payment of the overdue balance.
- ACTIVE → CLOSED: explicit customer request.
- SUSPENDED → CLOSED: automatic after 90 days without payment.

## Operational Notes

- The nightly billing run executes at 02:00 UTC and processes all accounts
  with status not equal to CLOSED.
- Invoice generation is governed by InvoiceEligibilityService.isEligible();
  see src/main/java/com/acme/billing/InvoiceEligibilityService.java.
- A status normalisation layer (StatusMapper) converts raw upstream
  status strings before eligibility is evaluated. Any mapping that treats
  SUSPENDED as ACTIVE would cause the billing run to generate invoices for
  suspended accounts — a compliance violation.
`

func writeDocsCommit5(targetDir string) error {
	return writeFile(targetDir, filepath.Join("docs", "account-lifecycle.md"), docsCommit5)
}
