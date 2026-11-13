package main

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// The report spec of the plugin fixes this shape:
// "### F1 — <title> (HIGH, confidence medium)".
var findingHeading = regexp.MustCompile(`^### (F\d+) [—–-] (.*) \((CRITICAL|HIGH|MEDIUM|LOW), confidence \w+\)\s*$`)

// markdownReport is the markdown report split around the findings section.
// All parts keep their line endings.
type markdownReport struct {
	head   []string
	intro  []string
	order  []string
	blocks map[string][]string
	tail   []string
}

func splitMarkdown(text string) (markdownReport, error) {
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var headings []int
	fenced := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "```"), strings.HasPrefix(line, "~~~"):
			fenced = !fenced

		case !fenced && strings.HasPrefix(line, "#"):
			headings = append(headings, i)
		}
	}

	start := slices.IndexFunc(headings, func(i int) bool { return strings.TrimSpace(lines[i]) == "## Findings" })
	if start == -1 {
		return markdownReport{head: lines, blocks: map[string][]string{}}, nil
	}

	end := len(lines)
	var findingStarts []int
	for _, i := range headings[start+1:] {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}

		findingStarts = append(findingStarts, i)
	}

	report := markdownReport{
		head:   lines[:headings[start]+1],
		blocks: map[string][]string{},
		tail:   lines[end:],
	}

	introEnd := end
	if len(findingStarts) > 0 {
		introEnd = findingStarts[0]
	}

	report.intro = lines[headings[start]+1 : introEnd]

	for n, first := range findingStarts {
		last := end
		if n+1 < len(findingStarts) {
			last = findingStarts[n+1]
		}

		match := findingHeading.FindStringSubmatch(strings.TrimRight(lines[first], "\n"))
		if match == nil {
			return markdownReport{}, fmt.Errorf("markdown line %d: unexpected heading %q", first+1, strings.TrimSpace(lines[first]))
		}

		_, ok := report.blocks[match[1]]
		if ok {
			return markdownReport{}, fmt.Errorf("markdown line %d: second section for %s", first+1, match[1])
		}

		report.order = append(report.order, match[1])
		report.blocks[match[1]] = lines[first:last]
	}

	return report, nil
}

// check makes sure that the sections map one to one to the findings.
// It returns warnings for differences that do not break the mapping.
func (m markdownReport) check(findings []finding) ([]string, error) {
	byNumber := make(map[string]finding, len(findings))
	for _, f := range findings {
		byNumber[f.Number] = f
	}

	var missing, unknown []string
	for _, f := range findings {
		_, ok := m.blocks[f.Number]
		if !ok {
			missing = append(missing, f.Number)
		}
	}

	for _, number := range m.order {
		_, ok := byNumber[number]
		if !ok {
			unknown = append(unknown, number)
		}
	}

	if len(missing) > 0 || len(unknown) > 0 {
		return nil, fmt.Errorf("markdown sections do not match the JSONL records (missing sections: %s, unknown sections: %s)", listOrNone(missing), listOrNone(unknown))
	}

	var warnings []string
	for _, number := range m.order {
		match := findingHeading.FindStringSubmatch(strings.TrimRight(m.blocks[number][0], "\n"))
		f := byNumber[number]
		if match[3] != f.Severity {
			return nil, fmt.Errorf("%s: severity %s in markdown, %s in JSONL", number, match[3], f.Severity)
		}

		if strings.TrimSpace(match[2]) != strings.TrimSpace(f.Title) {
			warnings = append(warnings, number+": the markdown title differs from the JSONL title")
		}
	}

	return warnings, nil
}

func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}

	return strings.Join(items, ", ")
}

func renderMarkdown(m markdownReport, findings []finding, matched matchResult, baselineName string) []byte {
	var kept []finding
	var hidden []finding
	for _, f := range findings {
		_, ok := matched.suppressed[f.Number]
		if ok {
			hidden = append(hidden, f)
		} else {
			kept = append(kept, f)
		}
	}

	note := fmt.Sprintf("\n> **Baseline applied.** This is a filtered copy of `%s.md`."+
		" It hides %d of %d findings."+
		" They match entries in the baseline file `%s`."+
		" The summary and the Coverage section below still count all %d findings."+
		" The remaining findings are: %s.",
		reportName, len(hidden), len(findings), baselineName, len(findings), severityCounts(kept))
	if len(hidden) > 0 {
		note += ` The section "Suppressed by baseline" lists the hidden findings.`
	}

	var out strings.Builder
	noteWritten := false
	for _, line := range m.head {
		out.WriteString(line)
		if !noteWritten && strings.HasPrefix(line, "# ") {
			out.WriteString(note + "\n")
			noteWritten = true
		}
	}

	writeLines(&out, m.intro)

	for _, number := range m.order {
		_, ok := matched.suppressed[number]
		if ok {
			continue
		}

		block := m.blocks[number]
		out.WriteString(block[0])
		similar, ok := matched.hints[number]
		if ok {
			refs := make([]string, 0, len(similar))
			for _, e := range similar {
				refs = append(refs, fmt.Sprintf("`%s` (%s: %s)", e.ID, e.Status, cmp.Or(e.Title, "no title")))
			}

			out.WriteString("\n> **Baseline hint.** This finding has the same file and CWE as baseline entries: " +
				strings.Join(refs, "; ") + ". If it is the same issue, add it to the baseline.\n")
		}

		writeLines(&out, block[1:])
	}

	if len(hidden) > 0 {
		out.WriteString("## Suppressed by baseline\n\n")
		out.WriteString("These findings match entries in `" + baselineName + "`. The team reviewed them earlier.\n\n")
		out.WriteString("| Finding | Severity | Title | Status | Reason |\n")
		out.WriteString("| --- | --- | --- | --- | --- |\n")
		for _, f := range hidden {
			e := matched.suppressed[f.Number]
			out.WriteString("| " + strings.Join([]string{
				f.Number, f.Severity, tableCell(f.Title), e.Status, tableCell(e.Reason),
			}, " | ") + " |\n")
		}

		out.WriteString("\n")
	}

	writeLines(&out, m.tail)

	return []byte(out.String())
}

func writeLines(out *strings.Builder, lines []string) {
	for _, line := range lines {
		out.WriteString(line)
	}
}

func tableCell(text string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(text), " "), "|", `\|`)
}
