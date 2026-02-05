package syncer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeonseon-ung/gateway/internal/engine"
)

func TestSnapshotFailureContract(t *testing.T) {
	const stable = "routes: [{prefix: /api, upstreams: ['http://localhost']}]\n"
	cases := []struct{ name, document, want string }{
		{"deleted file", "", "no such file"},
		{"oversized", strings.Repeat("#", routeFileLimit+1), "exceeds"},
		{"duplicate key", "routes: []\nroutes: []", "already defined"},
		{"unknown policy", "routes: [{prefix: /api, upstreams: [http://localhost], rewrite: true}]", "field rewrite not found"},
		{"duplicate route", "routes: [{prefix: /api, upstreams: [http://localhost]}, {prefix: /api, upstreams: [http://localhost]}]", "duplicate route prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "routes.yaml")
			eng := engine.New()
			defer eng.CloseIdleConnections()
			write := func(value string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(value), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(stable)
			watcher := NewRouteFile(eng, path)
			if err := watcher.Reload(); err != nil {
				t.Fatal(err)
			}
			if tc.document == "" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				write(tc.document)
			}
			err := watcher.Reload()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
			if !eng.Ready() {
				t.Fatal("last known good route was removed")
			}
		})
	}
}
