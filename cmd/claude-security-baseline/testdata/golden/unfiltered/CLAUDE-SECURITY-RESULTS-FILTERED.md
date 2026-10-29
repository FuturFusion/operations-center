# Claude Security results

> **Baseline applied.** This is a filtered copy of `CLAUDE-SECURITY-RESULTS.md`. It hides 0 of 4 findings. They match entries in the baseline file `testdata/baseline-empty.yaml`. The summary and the Coverage section below still count all 4 findings. The remaining findings are: 1 HIGH, 2 MEDIUM, 1 LOW.

This scan found 4 findings: 1 HIGH, 2 MEDIUM and 1 LOW.

## Coverage

The scan covered `internal`.

## Findings

### F1 — TLS chain is not verified (HIGH, confidence high)

**What.** The code trusts every certificate:

```go
### This line is code, not a heading.
```

**Verification.** 3/3 lens verifiers confirmed.

### F2 — Password returned to viewers (MEDIUM, confidence high)

**What.** The password is in the response.

### F3 — Password | returned in list (MEDIUM, confidence medium)

**What.** Same as F2, for the list.

### F4 — Certificate pinned on first use (LOW, confidence medium)

**What.** The certificate is not verified on first use.

## What was verified

The panel verified all findings.
