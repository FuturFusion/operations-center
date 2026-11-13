package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	reportName   = "CLAUDE-SECURITY-RESULTS"
	filteredName = "CLAUDE-SECURITY-RESULTS-FILTERED"
	idKey        = "claudeSecurityPluginFindingId"
)

var severities = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}

type finding struct {
	// Number is the finding number of the report, for example "F3".
	Number   string `json:"id"`
	ID       string `json:"claudeSecurityPluginFindingId"`
	Title    string `json:"title"`
	File     string `json:"file"`
	CWE      string `json:"cwe_id"`
	Severity string `json:"severity"`
	jsonLine string
}

func parseJSONL(data []byte) ([]finding, error) {
	var findings []finding
	for i, line := range strings.SplitAfter(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var f finding
		err := json.Unmarshal([]byte(line), &f)
		if err != nil {
			return nil, fmt.Errorf("JSONL line %d: %w", i+1, err)
		}

		if f.Number == "" || f.ID == "" {
			return nil, fmt.Errorf("JSONL line %d: record without id or %s", i+1, idKey)
		}

		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}

		f.jsonLine = line
		findings = append(findings, f)
	}

	return findings, nil
}

func filterJSONL(findings []finding, suppressed map[string]entry) []byte {
	var out strings.Builder
	for _, f := range findings {
		_, ok := suppressed[f.Number]
		if !ok {
			out.WriteString(f.jsonLine)
		}
	}

	return []byte(out.String())
}

type sarifSuppression struct {
	Kind          string `json:"kind"`
	Status        string `json:"status"`
	Justification string `json:"justification"`
}

type sarifProperties struct {
	Bag struct {
		ID string `json:"claudeSecurityPluginFindingId"`
	} `json:"claudeSecurityPlugin"`
}

// suppressSARIF keeps the suppressed results and marks them with the SARIF
// suppressions property. SARIF viewers show them as suppressed.
// The JSON objects stay raw, so all other content passes through unchanged.
func suppressSARIF(data []byte, byID map[string]entry) ([]byte, error) {
	var log map[string]json.RawMessage
	err := json.Unmarshal(data, &log)
	if err != nil {
		return nil, fmt.Errorf("parse SARIF: %w", err)
	}

	var runs []map[string]json.RawMessage
	err = unmarshalIfPresent(log["runs"], &runs)
	if err != nil {
		return nil, fmt.Errorf("parse SARIF runs: %w", err)
	}

	for _, run := range runs {
		var results []map[string]json.RawMessage
		err = unmarshalIfPresent(run["results"], &results)
		if err != nil {
			return nil, fmt.Errorf("parse SARIF results: %w", err)
		}

		for _, result := range results {
			var properties sarifProperties
			err = unmarshalIfPresent(result["properties"], &properties)
			if err != nil {
				return nil, fmt.Errorf("parse SARIF result properties: %w", err)
			}

			e, ok := byID[properties.Bag.ID]
			if !ok {
				continue
			}

			result["suppressions"], err = marshal([]sarifSuppression{{
				Kind:          "external",
				Status:        "accepted",
				Justification: e.Status + ": " + e.Reason,
			}})
			if err != nil {
				return nil, err
			}
		}

		if results != nil {
			run["results"], err = marshal(results)
			if err != nil {
				return nil, err
			}
		}
	}

	if runs != nil {
		log["runs"], err = marshal(runs)
		if err != nil {
			return nil, err
		}
	}

	compact, err := marshal(log)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	err = json.Indent(&out, compact, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode SARIF: %w", err)
	}

	out.WriteString("\n")

	return out.Bytes(), nil
}

func unmarshalIfPresent(raw json.RawMessage, v any) error {
	if raw == nil {
		return nil
	}

	return json.Unmarshal(raw, v)
}

// marshal does not escape HTML characters. This keeps "<", ">" and "&" of
// the report as they are.
func marshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("encode SARIF: %w", err)
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func severityCounts(findings []finding) string {
	counts := map[string]int{}
	for _, f := range findings {
		counts[f.Severity]++
	}

	var parts []string
	for _, s := range severities {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}
