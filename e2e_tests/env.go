package e2e

import (
	"os"
	"strconv"
)

var (
	diskSize             = envOrDefault("OPERATIONS_CENTER_E2E_TEST_DISK_SIZE", "50GiB")
	memorySize           = envOrDefault("OPERATIONS_CENTER_E2E_TEST_MEMORY_SIZE", "4GiB")
	cpuCount             = envOrDefault("OPERATIONS_CENTER_E2E_TEST_CPU_COUNT", "2")
	concurrentSetup      = envOrDefault("OPERATIONS_CENTER_E2E_TEST_CONCURRENT_SETUP", "true")
	timeoutStretchFactor = envFloatOrDefault("OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR", 1.0)
	cpuArch              = envOrDefault("OPERATIONS_CENTER_E2E_TEST_CPU_ARCH", "amd64")
	debug                = envBoolOrDefault("OPERATIONS_CENTER_E2E_TEST_DEBUG", false)
)

func envOrDefault(envVar string, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}

	return value
}

func envFloatOrDefault(envVar string, defaultValue float64) float64 {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}

	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}

	return f
}

func envBoolOrDefault(envVar string, defaultValue bool) bool {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}

	b, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}

	return b
}
