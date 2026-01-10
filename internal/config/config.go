package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Discovery  DiscoveryConfig  `yaml:"discovery"`
	Middleware MiddlewareConfig `yaml:"middleware"`
}

type ServerConfig struct {
	Port              int           `yaml:"port"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	AdminAddress      string        `yaml:"admin_address"`
	TrustedProxies    []string      `yaml:"trusted_proxies"`
}

type DiscoveryConfig struct {
	Provider      string        `yaml:"provider"`
	Address       string        `yaml:"address"`
	RouteFile     string        `yaml:"route_file"`
	WatchInterval time.Duration `yaml:"watch_interval"`
}

type MiddlewareConfig struct {
	GlobalEnabled []string        `yaml:"global_enabled"`
	RateLimit     RateLimitConfig `yaml:"ratelimit"`
	Auth          AuthConfig      `yaml:"auth"`
}

type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second"`
	Burst             int  `yaml:"burst"`
}

type AuthConfig struct {
	Enabled            bool         `yaml:"enabled"`
	ApiKeyHeader       string       `yaml:"api_key_header"`
	InternalSigningKey string       `yaml:"internal_signing_key"`
	SigningKeyEnv      string       `yaml:"signing_key_env"`
	DemoCredentials    bool         `yaml:"demo_credentials"`
	Credentials        []Credential `yaml:"credentials"`
}

type Credential struct {
	KeyEnv string `yaml:"key_env"`
	UserID string `yaml:"user_id"`
	Role   string `yaml:"role"`
	Key    string `yaml:"-"`
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := Config{
		Server: ServerConfig{
			Port: 8080, ReadTimeout: 5 * time.Second, ReadHeaderTimeout: 3 * time.Second,
			WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, ShutdownTimeout: 5 * time.Second,
		},
		Discovery: DiscoveryConfig{Provider: "mock", WatchInterval: 10 * time.Second},
	}
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("config must contain exactly one YAML document")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *Config) validate() error {
	srv := cfg.Server
	if srv.Port < 1 || srv.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if srv.ReadTimeout < 0 || srv.ReadHeaderTimeout <= 0 || srv.WriteTimeout < 0 || srv.IdleTimeout <= 0 || srv.ShutdownTimeout <= 0 {
		return fmt.Errorf("server timeouts must be positive, except read_timeout and write_timeout may be zero")
	}
	if srv.AdminAddress != "" {
		host, port, err := net.SplitHostPort(srv.AdminAddress)
		ip, ipErr := netip.ParseAddr(host)
		n, portErr := strconv.Atoi(port)
		if err != nil || ipErr != nil || !ip.IsLoopback() || portErr != nil || n < 1 || n > 65535 || n == srv.Port {
			return fmt.Errorf("server.admin_address must use a loopback IP and a different numeric port")
		}
	}
	if _, err := srv.ProxyPrefixes(); err != nil {
		return err
	}
	switch cfg.Discovery.Provider {
	case "mock":
	case "file":
		if cfg.Discovery.RouteFile == "" || cfg.Discovery.WatchInterval <= 0 {
			return fmt.Errorf("file discovery requires route_file and a positive watch_interval")
		}
	default:
		return fmt.Errorf("unknown discovery provider %q", cfg.Discovery.Provider)
	}
	seen := make(map[string]bool)
	for _, plugin := range cfg.Middleware.GlobalEnabled {
		switch plugin {
		case "auth", "logging", "ratelimit":
		default:
			return fmt.Errorf("unknown middleware %q", plugin)
		}
		if seen[plugin] {
			return fmt.Errorf("duplicate middleware %q", plugin)
		}
		seen[plugin] = true
	}
	limit := cfg.Middleware.RateLimit
	if limit.Enabled && (!seen["ratelimit"] || limit.RequestsPerSecond < 1 || limit.Burst < 1) {
		return fmt.Errorf("enabled rate limiter requires a middleware entry and positive rate and burst")
	}
	if cfg.Middleware.Auth.Enabled {
		if !seen["auth"] {
			return fmt.Errorf("enabled authentication requires an auth middleware entry")
		}
		return cfg.Middleware.Auth.resolve()
	}
	return nil
}

func (s ServerConfig) ProxyPrefixes() ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(s.TrustedProxies))
	for _, value := range s.TrustedProxies {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Bits() == 0 || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q", value)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (cfg *AuthConfig) resolve() error {
	header := strings.ToLower(cfg.ApiKeyHeader)
	if header != "" && (!strings.HasPrefix(header, "x-") || !headerName(header) ||
		strings.HasPrefix(header, "x-forwarded-") || header == "x-request-id" || header == "x-real-ip") {
		return fmt.Errorf("invalid api_key_header")
	}
	if cfg.SigningKeyEnv != "" {
		if cfg.InternalSigningKey != "" {
			return fmt.Errorf("configure only one JWT signing key source")
		}
		cfg.InternalSigningKey = os.Getenv(cfg.SigningKeyEnv)
	}
	if len(cfg.InternalSigningKey) < 32 {
		return fmt.Errorf("internal signing key must be at least 32 bytes")
	}
	if cfg.DemoCredentials {
		if len(cfg.Credentials) != 0 {
			return fmt.Errorf("demo credentials cannot be combined with configured credentials")
		}
		return nil
	}
	if cfg.SigningKeyEnv == "" || len(cfg.Credentials) == 0 {
		return fmt.Errorf("authentication requires signing_key_env and at least one credential outside demo mode")
	}
	seen := make(map[string]bool)
	for i := range cfg.Credentials {
		credential := &cfg.Credentials[i]
		credential.Key = os.Getenv(credential.KeyEnv)
		if len(credential.Key) < 24 || strings.ContainsAny(credential.Key, " \t\r\n") ||
			strings.TrimSpace(credential.UserID) == "" || strings.TrimSpace(credential.Role) == "" {
			return fmt.Errorf("credential %d requires a user, role, and environment key of at least 24 bytes without whitespace", i+1)
		}
		if seen[credential.Key] || credential.Key == cfg.InternalSigningKey {
			return fmt.Errorf("credential %d duplicates another credential or the signing key", i+1)
		}
		seen[credential.Key] = true
	}
	return nil
}

func headerName(value string) bool {
	for _, c := range value {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}
