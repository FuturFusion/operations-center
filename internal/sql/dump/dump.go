package dump

import "strconv"

// Option represents different types of dump.
type Option int

func (d Option) String() string {
	return strconv.Itoa(int(d))
}

// Dump response options.
const (
	OptionDefault Option = iota
	OptionSchema
	OptionTables
)

type SQLDump struct {
	Text string `json:"text" yaml:"text"`
}

type SQLQuery struct {
	Query string `json:"query" yaml:"query"`
}

// SQLBatch represents a batch result.
type SQLBatch struct {
	Results []SQLResult
}

// SQLResult represents a query result.
type SQLResult struct {
	Type         string   `json:"type"          yaml:"type"`
	Columns      []string `json:"columns"       yaml:"columns"`
	Rows         [][]any  `json:"rows"          yaml:"rows"`
	RowsAffected int64    `json:"rows_affected" yaml:"rows_affected"`
}
