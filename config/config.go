package config

// Config represents the root configuration structure.
type Config struct {
	Server      ServerConfig       `yaml:"server"`
	Middlewares []MiddlewareConfig `yaml:"middlewares"`
}

// ServerConfig holds the physical server settings.
type ServerConfig struct {
	Port int `yaml:"port"`
}

// MiddlewareConfig acts as a generic container for any middleware configuration.
// It relies on mapstructure to decode the 'Config' map into strict structs later.
type MiddlewareConfig struct {
	Name    string                 `yaml:"name"`
	Enabled bool                   `yaml:"enabled"`
	Config  map[string]interface{} `yaml:"config"`
}
