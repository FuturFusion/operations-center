package config

import (
	"os"
	"strconv"
	"sync/atomic"
)

// globalInternalConfig holds the settings which are read from the environment
// rather than from the config file. It is written by Init and by InitTest and is
// unrelated to the config store, so it carries no lock of its own.
var globalInternalConfig atomic.Pointer[InternalConfig]

func init() {
	globalInternalConfig.Store(&InternalConfig{})
}

func initInternalConfig() {
	env := os.Getenv(ApplicationEnvPrefix + "_DISABLE_BACKGROUND_TASKS")
	isBackgroundTasksDisabled, _ := strconv.ParseBool(env)

	env = os.Getenv(ApplicationEnvPrefix + "_SOURCE_POLL_SKIP_FIRST")
	sourcePollSkipFirst, _ := strconv.ParseBool(env)

	globalInternalConfig.Store(&InternalConfig{
		IsBackgroundTasksDisabled: isBackgroundTasksDisabled,
		SourcePollSkipFirst:       sourcePollSkipFirst,
	})
}

// IsBackgroundTasksDisabled checks OPERATIONS_CENTER_DISABLE_BACKGROUND_TASKS
// env var. If the env var has a value indicating true ("1", "t", "T", "true",
// "TRUE", "True"), true is returned. False is returned otherwise.
//
// If true, all background tasks are disabled. This is mainly useful during
// development or for integration tests.
func IsBackgroundTasksDisabled() bool {
	return globalInternalConfig.Load().IsBackgroundTasksDisabled
}

// SourcePollSkipFirst checks OPERATIONS_CENTER_SOURCE_POLL_SKIP_FIRST env var.
// If the env var has a value indicating true ("1", "t", "T", "true", "TRUE",
// "True"), true is returned. False is returned otherwise.
//
// If true, the first execution of the task to update the updates from the
// configured source is skipped. This is mainly useful during development or for
// integration tests.
func SourcePollSkipFirst() bool {
	return globalInternalConfig.Load().SourcePollSkipFirst
}
