package supportbundle

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// secretKeyPattern matches the keys of values, which are replaced in the bundle.
var secretKeyPattern = regexp.MustCompile(`(?i)(password|secret|token|hmac|private_key|client_key|server_key|platform_key|api_key|access_key|tls_.*_key|recovery_keys|encryption_key|auth_key|setup_key|preshared_key|keyring|(^|\.)key$|^cloud-init\.(user|vendor)-data$|^user\.(user|vendor)-data$|^environment\.|^acme\.provider\.environment$|^systemd\.credential(-binary)?\.)`)

// publicKeyPattern matches the keys of values, which are kept, although they match secretKeyPattern.
var publicKeyPattern = regexp.MustCompile(`(?i)(_expiry|_status)$`)

// redactJSON returns v as indented JSON with the values of secret keys replaced.
func redactJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var decoded any
	err = decoder.Decode(&decoded)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(redact(decoded), "", "  ")
}

func redact(v any) any {
	switch value := v.(type) {
	case map[string]any:
		for key, item := range value {
			if secretKeyPattern.MatchString(key) && !publicKeyPattern.MatchString(key) && isSecretValue(item) {
				value[key] = redacted
				continue
			}

			value[key] = redact(item)
		}

	case []any:
		for i, item := range value {
			value[i] = redact(item)
		}

	case string:
		// Catches private keys stored under arbitrary keys, e.g. user.*.
		if strings.Contains(value, "PRIVATE KEY-----") {
			return redacted
		}
	}

	return v
}

// isSecretValue keeps flags and numbers, e.g. whether the recovery keys have been retrieved.
func isSecretValue(v any) bool {
	switch value := v.(type) {
	case string:
		return value != ""

	case []any:
		return len(value) > 0

	case map[string]any:
		return len(value) > 0

	default:
		return false
	}
}
