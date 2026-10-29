package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type matchResult struct {
	// suppressed maps the finding number to the matching baseline entry.
	suppressed map[string]entry
	// hints maps the finding number to baseline entries with the same file and CWE.
	hints map[string][]entry
	// unused lists the baseline entries that match no finding.
	unused []entry
}

func match(findings []finding, entries []entry) matchResult {
	byID := make(map[string]entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	result := matchResult{
		suppressed: map[string]entry{},
		hints:      map[string][]entry{},
	}

	found := map[string]bool{}
	for _, f := range findings {
		found[f.ID] = true
		e, ok := byID[f.ID]
		if ok {
			result.suppressed[f.Number] = e
			continue
		}

		for _, e := range entries {
			if e.File == f.File && e.CWE != "" && e.CWE == f.CWE {
				result.hints[f.Number] = append(result.hints[f.Number], e)
			}
		}
	}

	for _, e := range entries {
		if !found[e.ID] {
			result.unused = append(result.unused, e)
		}
	}

	return result
}

func readReport(dir string) ([]finding, []byte, string, error) {
	jsonl, err := os.ReadFile(filepath.Join(dir, reportName+".jsonl"))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read report: %w", err)
	}

	findings, err := parseJSONL(jsonl)
	if err != nil {
		return nil, nil, "", err
	}

	sarif, err := os.ReadFile(filepath.Join(dir, reportName+".sarif"))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read report: %w", err)
	}

	markdown, err := os.ReadFile(filepath.Join(dir, reportName+".md"))
	if err != nil {
		return nil, nil, "", fmt.Errorf("read report: %w", err)
	}

	return findings, sarif, string(markdown), nil
}

func readBaseline(path string) ([]entry, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read baseline: %w", err)
	}

	entries, err := parseBaseline(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}

	return entries, data, nil
}

// filterReport checks all inputs before it writes the first file.
func filterReport(reportDir string, baselinePath string, outDir string, stdout io.Writer, stderr io.Writer) error {
	entries, _, err := readBaseline(baselinePath)
	if err != nil {
		return err
	}

	findings, sarif, markdown, err := readReport(reportDir)
	if err != nil {
		return err
	}

	md, err := splitMarkdown(markdown)
	if err != nil {
		return err
	}

	warnings, err := md.check(findings)
	if err != nil {
		return err
	}

	for _, w := range warnings {
		_, _ = fmt.Fprintf(stderr, "warning: %s\n", w)
	}

	matched := match(findings, entries)
	byID := make(map[string]entry, len(matched.suppressed))
	for _, e := range matched.suppressed {
		byID[e.ID] = e
	}

	filteredSARIF, err := suppressSARIF(sarif, byID)
	if err != nil {
		return err
	}

	outputs := []struct {
		name string
		data []byte
	}{
		{filteredName + ".md", renderMarkdown(md, findings, matched, baselinePath)},
		{filteredName + ".jsonl", filterJSONL(findings, matched.suppressed)},
		{filteredName + ".sarif", filteredSARIF},
	}

	err = os.MkdirAll(outDir, 0o755)
	if err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	for _, o := range outputs {
		err = os.WriteFile(filepath.Join(outDir, o.name), o.data, 0o644)
		if err != nil {
			return fmt.Errorf("write %s: %w", o.name, err)
		}
	}

	var kept []finding
	for _, f := range findings {
		_, ok := matched.suppressed[f.Number]
		if !ok {
			kept = append(kept, f)
		}
	}

	_, _ = fmt.Fprintf(stdout, "%d of %d findings suppressed by the baseline.\n", len(matched.suppressed), len(findings))
	_, _ = fmt.Fprintf(stdout, "Remaining findings: %s.\n", severityCounts(kept))
	for _, f := range kept {
		similar, ok := matched.hints[f.Number]
		if ok {
			_, _ = fmt.Fprintf(stdout, "Hint: %s has the same file and CWE as baseline entry %s.\n", f.Number, similar[0].ID)
		}
	}

	if len(matched.unused) > 0 {
		_, _ = fmt.Fprintf(stdout, "%d baseline entries match no finding in this report. The code may have changed, or the scan did not cover the file:\n", len(matched.unused))
		for _, e := range matched.unused {
			_, _ = fmt.Fprintf(stdout, "  %s  %s  %s\n", e.ID, e.File, e.Title)
		}
	}

	_, _ = fmt.Fprintf(stdout, "Wrote %s.{md,jsonl,sarif} to %s.\n", filteredName, outDir)

	return nil
}

type addOptions struct {
	reportDir    string
	baselinePath string
	wanted       []string
	status       string
	reason       string
	reviewer     string
	date         string
}

func addFindings(opts addOptions, stdout io.Writer) error {
	if !slices.Contains(statuses, opts.status) {
		return fmt.Errorf("status must be one of %s", strings.Join(statuses, ", "))
	}

	if strings.TrimSpace(opts.reason) == "" {
		return errors.New("missing reason")
	}

	entries, data, err := readBaseline(opts.baselinePath)
	if err != nil {
		return err
	}

	findings, _, _, err := readReport(opts.reportDir)
	if err != nil {
		return err
	}

	known := map[string]bool{}
	for _, e := range entries {
		known[e.ID] = true
	}

	var added []entry
	for _, wanted := range opts.wanted {
		i := slices.IndexFunc(findings, func(f finding) bool { return f.Number == wanted || f.ID == wanted })
		if i == -1 {
			return fmt.Errorf("finding %s is not in the report", wanted)
		}

		f := findings[i]
		if known[f.ID] {
			_, _ = fmt.Fprintf(stdout, "Skip %s: %s is already in the baseline.\n", f.Number, f.ID)
			continue
		}

		known[f.ID] = true
		added = append(added, entry{
			ID:         f.ID,
			Status:     opts.status,
			Reason:     opts.reason,
			Title:      f.Title,
			File:       f.File,
			CWE:        f.CWE,
			Severity:   f.Severity,
			Date:       opts.date,
			ReviewedBy: opts.reviewer,
		})
		_, _ = fmt.Fprintf(stdout, "Add %s (%s).\n", f.Number, f.ID)
	}

	if len(added) == 0 {
		return nil
	}

	updated, err := appendBaseline(data, added)
	if err != nil {
		return err
	}

	// Parse the result again to make sure the file stays valid.
	_, err = parseBaseline(updated)
	if err != nil {
		return fmt.Errorf("updated baseline is not valid: %w", err)
	}

	err = os.WriteFile(opts.baselinePath, updated, 0o644)
	if err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}

	return nil
}
