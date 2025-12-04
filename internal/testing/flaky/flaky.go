// Package flaky allows to skip flaky tests on error, which prevents
// the complete test suite from failing, if a "known to be flaky" test fails.
//
// The skipping of flaky tests needs to be enabled with the environment
// variable "FLAKY_SKIP_ON_FAIL".
package flaky

import (
	"os"
	"strconv"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testifyT interface {
	require.TestingT
	assert.TestingT
	Helper()
}

type testingT interface {
	Fatal(args ...any)
	SkipNow()
	Skipf(format string, args ...any)
	testifyT
}

// SkipOnFail wraps testing.T such that instead of a test failing, it is skipped
// with the reason being logged as usual. This is intended to be used with
// test conditions, that occasionally fail, e.g. calling to an external resource
// as part of a test.
// This should be used sparingly. Providing a justification is required.
//
// The skipping is only enabled, if the environment variable "FLAKY_SKIP_ON_FAIL"
// is set to a truthy value.
func SkipOnFail(t testingT, justification string) testifyT {
	if len(justification) < 10 {
		t.Fatal("OnFail requires a justification with minimum length of 10 characters")
		return t
	}

	skipOnFail := os.Getenv("FLAKY_SKIP_ON_FAIL")
	skip, _ := strconv.ParseBool(skipOnFail)

	if !skip {
		return t
	}

	return onFail{
		testingT:      t,
		justification: justification,
	}
}

type onFail struct {
	testingT

	justification string
}

func (o onFail) Errorf(format string, args ...any) {
	o.Skipf(o.justification+format, args...)
}

func (o onFail) FailNow() {
	o.SkipNow()
}
