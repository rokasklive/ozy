# Incident: Suspended Accounts Incorrectly Invoiced

On 2026-06-14 the billing run invoiced **four accounts that were in SUSPENDED
status**, for $10,625.75 total. Per policy, SUSPENDED accounts must never be
invoice-eligible. This worked previously, so it is a regression.

You are investigating the `acme-billing` service. Your tools let you search the
code, read source files and documentation, inspect the repository's git history,
and query a read-only incident database. Diagnose the regression by working the
evidence with those tools:

- confirm the affected invoices in the **incident database**,
- locate where invoice-eligibility is decided in the **source**,
- use the **git history** of that code to find the change that introduced the bug,
- then propose the fix and the regression test that would catch it.

Ground every claim in something a tool returned — a query result, a file you
read, or a specific commit — not assumption.

## Required output (all five are mandatory)

1. **Root cause** — what the bug is and why it lets suspended accounts be invoiced.
2. **File + function** — the exact file path and the function that contains the bug.
3. **Culprit commit** — the subject line of the commit that introduced the regression.
4. **Minimal patch** — the smallest change that fixes it (a single line if possible).
5. **Regression test** — the name of a test that would catch this regression.
