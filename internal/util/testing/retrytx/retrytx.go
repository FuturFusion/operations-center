package retrytx

import (
	"testing"
	"time"

	clusterTx "github.com/cowsql/go-cowsql/cluster/db/transaction"

	"github.com/FuturFusion/operations-center/internal/sql/transaction"
)

// Register makes transaction.Do retry its function like the transactor of the
// global database. Tests that call it must not run in parallel, since the
// transactor applies to all transactions.
func Register(t *testing.T) {
	t.Helper()

	transaction.Register(transactor{})
	t.Cleanup(func() {
		transaction.Register(nil)
	})
}

type transactor struct{}

func (transactor) DBTX() clusterTx.DBTX     { return nil }
func (transactor) MaxRetries() int          { return 3 }
func (transactor) OnTxStart(bool) func()    { return nil }
func (transactor) TxTimeout() time.Duration { return 0 }
func (transactor) EnterExclusive() error    { return nil }
