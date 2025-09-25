package config

// IsBackgroundTasksDisabled checks OPERATIONS_CENTER_DISABLE_BACKGROUND_TASKS
// env var. If the env var has a value indicating true ("1", "t", "T", "true",
// "TRUE", "True"), true is returned. False is returned otherwise.
//
// If true, all background tasks are disabled. This is mainly useful during
// development or for integration tests.
func IsBackgroundTasksDisabled() bool {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	return globalInternalConfig.IsBackgroundTasksDisabled
}

// SourcePollSkipFirst checks OPERATIONS_CENTER_SOURCE_POLL_SKIP_FIRST env var.
// If the env var has a value indicating true ("1", "t", "T", "true", "TRUE",
// "True"), true is returned. False is returned otherwise.
//
// If true, the first execution of the task to update the updates from the
// configured source is skipped. This is mainly useful during development or for
// integration tests.
func SourcePollSkipFirst() bool {
	globalConfigInstanceMu.Lock()
	defer globalConfigInstanceMu.Unlock()

	return globalInternalConfig.SourcePollSkipFirst
}
