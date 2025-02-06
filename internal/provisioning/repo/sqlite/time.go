package sqlite

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

type datetime time.Time

// Value implements the sql driver.Valuer interface.
func (t datetime) Value() (driver.Value, error) {
	return time.Time(t), nil
}

// Scan implements the sql.Scanner interface.
func (t *datetime) Scan(v any) error {
	dtStr, ok := v.(string)
	if !ok {
		return fmt.Errorf("converting %T to textTime is unsupported", v)
	}

	// Handling of datetime values from sqlite following implementation in
	// https://github.com/mattn/go-sqlite3/blob/7658c06970ecf5588d8cd930ed1f2de7223f1010/sqlite3.go#L2262
	var err error
	dtStr = strings.TrimSuffix(dtStr, "Z")
	for _, format := range sqlite3.SQLiteTimestampFormats {
		var timeVal time.Time
		timeVal, err = time.ParseInLocation(format, dtStr, time.UTC)
		if err == nil {
			*t = datetime(timeVal)
			break
		}
	}

	return err
}
