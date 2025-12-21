package middleware

import (
	"log/slog"
	"net/http"

	"github.com/jeonseon-ung/gateway/internal/config"
	"github.com/jeonseon-ung/gateway/internal/middleware/auth"
	"github.com/jeonseon-ung/gateway/internal/middleware/logger"
	"github.com/jeonseon-ung/gateway/internal/middleware/ratelimit"
)

type Middleware func(http.Handler) http.Handler

func BuildChain(cfg config.MiddlewareConfig, finalHandler http.Handler) http.Handler {
	chain := finalHandler

	enabledPlugins := cfg.GlobalEnabled
	for i := len(enabledPlugins) - 1; i >= 0; i-- {
		name := enabledPlugins[i]
		switch name {
		case "logging":
			chain = logger.Handler(chain)
			slog.Info("Middleware enabled", "name", "logging")
		case "ratelimit":
			if cfg.RateLimit.Enabled {
				limiter := ratelimit.New(float64(cfg.RateLimit.RequestsPerSecond), cfg.RateLimit.Burst)
				chain = limiter.Handler(chain)
				slog.Info("Middleware enabled", "name", "ratelimit")
			}
		case "auth":
			if cfg.Auth.Enabled {
				chain = auth.New(cfg.Auth)(chain)
				slog.Info("Middleware enabled", "name", "auth")
			}
		default:
			slog.Warn("Unknown middleware specified in config", "name", name)
		}
	}

	return chain
}
