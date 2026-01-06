package syncer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jeonseon-ung/gateway/internal/engine"
	"gopkg.in/yaml.v3"
)

const routeFileLimit = 1 << 20

type RouteFile struct {
	engine *engine.Engine
	path   string
	digest [sha256.Size]byte
	loaded bool
}

func NewRouteFile(eng *engine.Engine, path string) *RouteFile {
	return &RouteFile{engine: eng, path: path}
}

// Reload retains the active table on read, parse, or route validation errors.
// The caller owns the reload loop and must serialize calls to Reload.
func (f *RouteFile) Reload() error {
	file, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, routeFileLimit+1))
	if err != nil {
		return err
	}
	if len(data) > routeFileLimit {
		return fmt.Errorf("route file exceeds %d bytes", routeFileLimit)
	}
	digest := sha256.Sum256(data)
	if f.loaded && digest == f.digest {
		return nil
	}

	var document struct {
		Routes []engine.Route `yaml:"routes"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode routes: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("route file must contain exactly one YAML document")
	}
	if document.Routes == nil {
		return fmt.Errorf("route file must specify routes (use [] for an empty table)")
	}
	if err := f.engine.ReplaceRoutes(document.Routes); err != nil {
		return err
	}
	f.digest, f.loaded = digest, true
	slog.Info("Route table reloaded", "routes", len(document.Routes))
	return nil
}

func (f *RouteFile) Watch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastError := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := f.Reload(); err != nil {
				if err.Error() != lastError {
					slog.Warn("Route reload rejected; active table retained", "error", err)
					lastError = err.Error()
				}
			} else {
				lastError = ""
			}
		}
	}
}
