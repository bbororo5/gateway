package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesCredentialsAndRejectsUnsafeConfig(t *testing.T) {
	t.Setenv("GATEWAY_TEST_SIGNING_KEY", "signing-key-not-real-at-least-thirty-two-bytes")
	t.Setenv("GATEWAY_TEST_CLIENT_KEY", "client-key-not-real-at-least-twenty-four-bytes")
	const valid = `
server:
  admin_address: 127.0.0.1:9090
  trusted_proxies: [192.0.2.0/24]
discovery:
  provider: file
  route_file: routes.yaml
middleware:
  global_enabled: [auth]
  auth:
    enabled: true
    api_key_header: X-API-Key
    signing_key_env: GATEWAY_TEST_SIGNING_KEY
    credentials:
      - key_env: GATEWAY_TEST_CLIENT_KEY
        user_id: app
        role: reader
`
	path := filepath.Join(t.TempDir(), "gateway.yaml")
	load := func(raw string) (*Config, error) {
		t.Helper()
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	cfg, err := load(valid)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 || cfg.Middleware.Auth.Credentials[0].Key != os.Getenv("GATEWAY_TEST_CLIENT_KEY") {
		t.Fatal("defaults or credentials missing")
	}

	for _, edit := range [][2]string{
		{"127.0.0.1:9090", "0.0.0.0:9090"},
		{"127.0.0.1:9090", "127.0.0.1:8080"},
		{"192.0.2.0/24", "0.0.0.0/0"},
		{"provider: file", "provider: unknown"},
		{"global_enabled: [auth]", "global_enabled: [auth, auth]"},
		{"global_enabled: [auth]", "global_enabled: []"},
		{"api_key_header: X-API-Key", "api_key_header: X-Request-ID"},
		{"signing_key_env: GATEWAY_TEST_SIGNING_KEY", "signing_key_env: GATEWAY_TEST_MISSING"},
		{"route_file:", "unknown:"},
	} {
		t.Run(edit[1], func(t *testing.T) {
			if _, err := load(strings.Replace(valid, edit[0], edit[1], 1)); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
