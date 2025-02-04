package transaction_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrorDriverQueryRow(t *testing.T) {
	errDB, _ := sql.Open("sqlerrordriver", "")

	row := errDB.QueryRow("boom!")

	require.Error(t, row.Err())
	require.ErrorContains(t, row.Err(), "boom!")
}
