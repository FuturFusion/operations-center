package main

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

var statuses = []string{"accepted", "false-positive"}

type entry struct {
	ID         string `yaml:"id"`
	Status     string `yaml:"status"`
	Reason     string `yaml:"reason"`
	Title      string `yaml:"title,omitempty"`
	File       string `yaml:"file,omitempty"`
	CWE        string `yaml:"cwe,omitempty"`
	Severity   string `yaml:"severity,omitempty"`
	Date       string `yaml:"date,omitempty"`
	ReviewedBy string `yaml:"reviewed_by,omitempty"`
}

type baselineFile struct {
	Findings []entry `yaml:"findings"`
}

func parseBaseline(data []byte) ([]entry, error) {
	var file baselineFile
	err := yaml.Unmarshal(data, &file)
	if err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}

	seen := make(map[string]bool, len(file.Findings))
	for i, e := range file.Findings {
		where := fmt.Sprintf("baseline entry %d", i+1)
		if e.ID == "" {
			return nil, fmt.Errorf("%s: missing id", where)
		}

		if seen[e.ID] {
			return nil, fmt.Errorf("%s: duplicate id %s", where, e.ID)
		}

		if !slices.Contains(statuses, e.Status) {
			return nil, fmt.Errorf("%s: status must be one of %s", where, strings.Join(statuses, ", "))
		}

		if strings.TrimSpace(e.Reason) == "" {
			return nil, fmt.Errorf("%s: missing reason", where)
		}

		seen[e.ID] = true
	}

	return file.Findings, nil
}

// appendBaseline edits the YAML node tree to keep the comments of the file.
func appendBaseline(data []byte, entries []entry) ([]byte, error) {
	var doc yaml.Node
	err := yaml.Unmarshal(data, &doc)
	if err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}

	// A file with only comments has no document. Keep the comments as a prefix.
	var prefix []byte
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		prefix = data
		if len(prefix) > 0 && !bytes.HasSuffix(prefix, []byte("\n")) {
			prefix = append(prefix, '\n')
		}
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("baseline: top level is not a mapping")
	}

	var findings *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "findings" {
			findings = root.Content[i+1]
			break
		}
	}

	if findings == nil {
		findings = &yaml.Node{}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "findings"}, findings)
	}

	isNull := findings.Kind == yaml.ScalarNode && findings.Tag == "!!null"
	if findings.Kind != yaml.SequenceNode && findings.Kind != 0 && !isNull {
		return nil, errors.New("baseline: findings is not a list")
	}

	findings.Kind = yaml.SequenceNode
	findings.Tag = "!!seq"
	findings.Value = ""
	findings.Style = 0

	for _, e := range entries {
		var node yaml.Node
		err = node.Encode(e)
		if err != nil {
			return nil, fmt.Errorf("encode baseline entry %s: %w", e.ID, err)
		}

		findings.Content = append(findings.Content, &node)
	}

	out, err := yaml.Dump(&doc, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return nil, fmt.Errorf("encode baseline: %w", err)
	}

	return append(prefix, out...), nil
}
