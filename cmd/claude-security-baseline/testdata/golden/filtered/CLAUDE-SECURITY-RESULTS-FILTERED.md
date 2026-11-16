# Claude Security results

> **Baseline applied.** This is a filtered copy of `CLAUDE-SECURITY-RESULTS.md`. It hides 2 of 4 findings. They match entries in the baseline file `testdata/baseline.yaml`. The summary and the Coverage section below still count all 4 findings. The remaining findings are: 1 HIGH, 1 LOW. The section "Suppressed by baseline" lists the hidden findings.

This scan found 4 findings: 1 HIGH, 2 MEDIUM and 1 LOW.

## Coverage

The scan covered `internal`.

## Findings

### F1 — TLS chain is not verified (HIGH, confidence high)

> **Baseline hint.** This finding has the same file and CWE as baseline entries: `v3:old` (accepted: TLS chain). If it is the same issue, add it to the baseline.

**What.** The code trusts every certificate:

```go
### This line is code, not a heading.
```

**Verification.** 3/3 lens verifiers confirmed.

### F4 — Certificate pinned on first use (LOW, confidence medium)

> **Baseline hint.** This finding has the same file and CWE as baseline entries: `v3:old` (accepted: TLS chain). If it is the same issue, add it to the baseline.

**What.** The certificate is not verified on first use.

## Suppressed by baseline

These findings match entries in `testdata/baseline.yaml`. The team reviewed them earlier.

| Finding | Severity | Title | Status | Reason |
| --- | --- | --- | --- | --- |
| F2 | MEDIUM | Password returned to viewers | false-positive | Only admins \| see it. |
| F3 | MEDIUM | Password \| returned in list | false-positive | Only admins \| see it. |

## What was verified

The panel verified all findings.
