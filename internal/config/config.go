package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Discovery  DiscoveryConfig  `yaml:"discovery"`
	Middleware MiddlewareConfig `yaml:"middleware"`
}

type ServerConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type DiscoveryConfig struct {
	Provider      string        `yaml:"provider"`
	Address       string        `yaml:"address"`
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
	Enabled            bool   `yaml:"enabled"`
	ApiKeyHeader       string `yaml:"api_key_header"`
	InternalSigningKey string `yaml:"internal_signing_key"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
