package e2e

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// envSetting records an environment variable together with its effective value.
type envSetting struct {
	name      string
	raw       string
	effective string
}

// envSettings collects the environment variables in the order they are read.
var envSettings []envSetting

var (
	diskSize             = envOrDefault("OPERATIONS_CENTER_E2E_TEST_DISK_SIZE", "50GiB")
	memorySize           = envOrDefault("OPERATIONS_CENTER_E2E_TEST_MEMORY_SIZE", "4GiB")
	cpuCount             = envOrDefault("OPERATIONS_CENTER_E2E_TEST_CPU_COUNT", "2")
	concurrentSetup      = envBoolOrDefault("OPERATIONS_CENTER_E2E_TEST_CONCURRENT_SETUP", true)
	timeoutStretchFactor = envFloatOrDefault("OPERATIONS_CENTER_E2E_TEST_TIMEOUT_STRETCH_FACTOR", 1.0)
	testTimeout          = envDurationOrDefault("OPERATIONS_CENTER_E2E_TEST_TIMEOUT", 45*time.Minute)
	cpuArch              = envOrDefault("OPERATIONS_CENTER_E2E_TEST_CPU_ARCH", "amd64")
	debug                = envBoolOrDefault("OPERATIONS_CENTER_E2E_TEST_DEBUG", false)
	noCleanup            = envBoolOrDefault("OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP", false)
	noCleanupOnError     = envBoolOrDefault("OPERATIONS_CENTER_E2E_TEST_NO_CLEANUP_ON_ERROR", false)
	bmcProxyAddress      = envOrDefault("OPERATIONS_CENTER_E2E_TEST_BMC_PROXY_ADDRESS", "")
	hostAddress          = envOrDefault("OPERATIONS_CENTER_E2E_TEST_HOST_ADDRESS", "")

	goCoverDir      = ""
	ocE2EGoCoverDir = ""
)

func init() {
	goCoverDir = os.Getenv("GOCOVERDIR")
	if goCoverDir == "" {
		tmpDir, err := os.MkdirTemp("", "oc-e2e-tests-*")
		if err != nil {
			panic("failed to create tmp dir: " + err.Error())
		}

		goCoverDir = tmpDir
	}

	recordEnv("GOCOVERDIR", goCoverDir)

	ocE2EGoCoverDir = envOrDefault("OPERATIONS_CENTER_E2E_GOCOVERDIR", goCoverDir)
}

var logEnvSettingsOnce sync.Once

// logEnvSettings logs the configuration of the end 2 end tests once per test run.
func logEnvSettings(t *testing.T) {
	t.Helper()

	logEnvSettingsOnce.Do(func() {
		t.Log("End 2 end test configuration:")
		for _, setting := range envSettings {
			t.Logf("  %s=%q (effective: %q)", setting.name, setting.raw, setting.effective)
		}
	})
}

func recordEnv(envVar string, effective any) {
	envSettings = append(envSettings, envSetting{
		name:      envVar,
		raw:       os.Getenv(envVar),
		effective: fmt.Sprint(effective),
	})
}

func envOrDefault(envVar string, defaultValue string) string {
	value := os.Getenv(envVar)
	if value == "" {
		value = defaultValue
	}

	recordEnv(envVar, value)

	return value
}

func envFloatOrDefault(envVar string, defaultValue float64) float64 {
	return envParsedOrDefault(envVar, defaultValue, func(value string) (float64, error) {
		return strconv.ParseFloat(value, 64)
	})
}

func envDurationOrDefault(envVar string, defaultValue time.Duration) time.Duration {
	return envParsedOrDefault(envVar, defaultValue, time.ParseDuration)
}

func envBoolOrDefault(envVar string, defaultValue bool) bool {
	return envParsedOrDefault(envVar, defaultValue, strconv.ParseBool)
}

// envParsedOrDefault falls back to the default value, if the value is not set or
// does not parse.
func envParsedOrDefault[T any](envVar string, defaultValue T, parse func(string) (T, error)) T {
	value := defaultValue

	raw := os.Getenv(envVar)
	if raw != "" {
		parsed, err := parse(raw)
		if err == nil {
			value = parsed
		}
	}

	recordEnv(envVar, value)

	return value
}
