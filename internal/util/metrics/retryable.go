package metrics

import (
	"strconv"

	"github.com/FuturFusion/operations-center/internal/domain"
)

// RetryableLabel returns the value of the retryable label for err. The value
// is "true" if err is marked as retryable and "false" otherwise.
func RetryableLabel(err error) string {
	return strconv.FormatBool(domain.IsRetryableError(err))
}
