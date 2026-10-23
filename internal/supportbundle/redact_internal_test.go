package supportbundle

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_redactJSON(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name: "nested secrets",
			value: map[string]any{
				"bmc_config": map[string]any{
					"username": "admin",
					"password": "secret",
				},
				"registration_token": "f9f8bd9f-6cb4-4d4b-a0c0-1d4a4d2c0d4f",
				"security": map[string]any{
					"encryption_recovery_keys":           []any{"abcd-efgh"},
					"encryption_recovery_keys_retrieved": true,
				},
				"config": map[string]any{
					"core.https_address":            ":8443",
					"loki.auth.password":            "secret",
					"cloud-init.user-data":          "#cloud-config",
					"environment.API_KEY":           "secret",
					"limits.cpu":                    "2",
					"acme.eab.hmac":                 "secret",
					"acme.provider.environment":     "CF_DNS_API_TOKEN=secret",
					"instances.tpm.platform_key":    "secret",
					"truenas.api_key":               "secret",
					"peers.ns1.key":                 "secret",
					"systemd.credential.foo":        "secret",
					"systemd.credential-binary.foo": "c2VjcmV0",
					"core.remote_token_expiry":      "1d",
					"user.ssh":                      "-----BEGIN OPENSSH PRIVATE KEY-----\n...",
				},
				"peers": []any{
					map[string]any{"public_key": "pub", "private_key": "priv", "port": 51820},
				},
				"storage": map[string]any{"key": "secret", "empty_password": ""},
			},
			want: `{
  "bmc_config": {
    "password": "[REDACTED]",
    "username": "admin"
  },
  "config": {
    "acme.eab.hmac": "[REDACTED]",
    "acme.provider.environment": "[REDACTED]",
    "cloud-init.user-data": "[REDACTED]",
    "core.https_address": ":8443",
    "core.remote_token_expiry": "1d",
    "environment.API_KEY": "[REDACTED]",
    "instances.tpm.platform_key": "[REDACTED]",
    "limits.cpu": "2",
    "loki.auth.password": "[REDACTED]",
    "peers.ns1.key": "[REDACTED]",
    "systemd.credential-binary.foo": "[REDACTED]",
    "systemd.credential.foo": "[REDACTED]",
    "truenas.api_key": "[REDACTED]",
    "user.ssh": "[REDACTED]"
  },
  "peers": [
    {
      "port": 51820,
      "private_key": "[REDACTED]",
      "public_key": "pub"
    }
  ],
  "registration_token": "[REDACTED]",
  "security": {
    "encryption_recovery_keys": "[REDACTED]",
    "encryption_recovery_keys_retrieved": true
  },
  "storage": {
    "empty_password": "",
    "key": "[REDACTED]"
  }
}`,
		},
		{
			name:  "list",
			value: []any{map[string]any{"token": "secret"}},
			want: `[
  {
    "token": "[REDACTED]"
  }
]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := redactJSON(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}
