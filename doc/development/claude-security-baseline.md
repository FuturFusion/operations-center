# Claude Security Baseline

## Overview

The `/claude-security` command of Claude Code scans the code base for security
issues. It writes the report to a directory `CLAUDE-SECURITY-<timestamp>` with
three files:

* `CLAUDE-SECURITY-RESULTS.md`: the report for the reviewer.
* `CLAUDE-SECURITY-RESULTS.jsonl`: one JSON record per finding, in the JSON
  Lines (JSONL) format.
* `CLAUDE-SECURITY-RESULTS.sarif`: the findings in the Static Analysis Results
  Interchange Format (SARIF).

The scan has no way to mark a finding as reviewed. Every scan reports the same
findings again. The baseline solves this. It lists the findings that the team
reviewed and decided not to fix. A filter removes these findings from the
report.

The baseline is the file `.claude-security-baseline.yaml` in the root of the
repository. The tool `cmd/claude-security-baseline` edits the baseline and
filters the reports.

## The baseline file

Each entry marks one finding with one of two states:

* `accepted`: the finding is a real issue, but the team accepts the risk.
* `false-positive`: the finding is not a real issue.

```yaml
findings:
  - id: v3:5f0c...
    status: false-positive
    reason: The path comes from the configuration file, not from the user.
    title: Path traversal in update file download
    file: internal/provisioning/adapter/updateserver/client.go
    cwe: CWE-22
    severity: MEDIUM
    date: "2026-10-29"
    reviewed_by: Jane Doe
```

The fields `id`, `status` and `reason` are required. The field `cwe` is the
Common Weakness Enumeration (CWE) ID of the issue. The `reason` explains the
decision to the next reviewer. The other fields help the reader. The filter also
uses `file` and `cwe` to give hints, see below.

The `id` is the `claudeSecurityPluginFindingId` of the finding. The scan
computes it from the rule, the file path and the code around the finding. Lines
added elsewhere in the file do not change the ID. A change to the code around
the finding changes the ID. The finding then shows up again and needs a new
review. This is intended, because the change can affect the decision.

Do not explain the decision in a code comment instead. The scan reports text in
the code that claims a finding is a false positive as a prompt injection
attempt.

## Workflow

### Scan

Run `/claude-security` in Claude Code. The report directory is created in the
root of the repository.

### Filter the report

```shell
make claude-security-filter
```

The target uses the newest `CLAUDE-SECURITY-<timestamp>` directory. Set `REPORT`
to use a different one:

```shell
make claude-security-filter REPORT=CLAUDE-SECURITY-20261029-101500
```

The filter writes three new files next to the original files:

* `CLAUDE-SECURITY-RESULTS-FILTERED.md`
* `CLAUDE-SECURITY-RESULTS-FILTERED.jsonl`
* `CLAUDE-SECURITY-RESULTS-FILTERED.sarif`

The original files stay unchanged. Read the filtered markdown report. It only
contains the findings that need a review.

### Triage the findings

Decide for each remaining finding:

* Fix the issue in the code.
* Add the finding to the baseline as `accepted` or `false-positive`.

### Add findings to the baseline

Use the finding numbers of the report, for example `F3` and `F7`:

```shell
go run ./cmd/claude-security-baseline add \
  -status false-positive \
  -reason "The path comes from the configuration file, not from the user." \
  -by "Jane Doe" \
  CLAUDE-SECURITY-20261029-101500 F3 F7
```

The finding numbers are only valid for this report. A new scan numbers the
findings again. The command looks up the ID of each finding and writes it to
the baseline. It also copies the title, file, CWE and severity. One call sets
the same status and reason for all given findings. Use one call per decision.

The command keeps the comments in the baseline file. It skips findings that are
already in the baseline. It fails without a change if a finding is not in the
report.

Run the filter again to check the result. Commit the baseline together with the
fixes. The reviewer of the pull request reviews the new entries like code.

## The filtered report

### Markdown

The filter removes the section of each suppressed finding from the `Findings`
section. It also adds the following:

* A note below the title. The note says how many findings the filter hid and
  which findings remain.
* A section `Suppressed by baseline` before `What was verified`. It lists the
  hidden findings with the status and reason from the baseline.
* A hint below the heading of a remaining finding, if it has the same file and
  CWE as a baseline entry. This often means the code changed and the finding
  got a new ID. Check if it is the same issue. If yes, add the finding to the
  baseline again and remove the old entry.

The filter does not change the summary at the top or the `Coverage` section.
Claude writes these as free text, so the filter can not change them reliably.
They still count all findings. The note points this out.

### JSONL and SARIF

The JSONL file only contains the remaining findings. The SARIF file contains all
findings. The suppressed findings get a `suppressions` entry with the status and
reason from the baseline. SARIF viewers show them as suppressed.

## Maintain the baseline

The filter lists the baseline entries that match no finding of the report. There
are two reasons for this:

* The code changed and the finding got a new ID or is gone.
* The scan did not cover the file this time.

Remove an entry once the finding is gone for good. Do not remove an entry only
because one scan did not report it.

## Errors

The filter checks the report before it writes any file. It fails if the
sections of the markdown report do not match the JSONL records. It also fails
if the severity of a finding differs between the two files. Claude writes the
markdown report, so its format can change with a new version of the plugin. If
the filter fails, update the parser in `cmd/claude-security-baseline/markdown.go`.

A different title in the markdown report and the JSONL record only gives a
warning.
