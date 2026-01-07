package syncer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeonseon-ung/gateway/internal/engine"
)

func TestRouteFileRetainsLastGoodSnapshot(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	eng := engine.New()
	path := filepath.Join(t.TempDir(), "routes.yaml")
	watcher := NewRouteFile(eng, path)
	write := func(contents string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("routes:\n  - prefix: /catalog\n    upstreams: [" + backend.URL + "]\n")
	if err := watcher.Reload(); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"routes: null", "route: []", "routes: []\n---\nroutes: []", "routes: [broken"} {
		write(invalid)
		if err := watcher.Reload(); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, httptest.NewRequest("GET", "/catalog", nil))
		if w.Code != http.StatusNoContent {
			t.Fatalf("invalid snapshot disturbed routing: %d", w.Code)
		}
	}

	write("routes: []\n")
	if err := watcher.Reload(); err != nil {
		t.Fatal(err)
	}
	if eng.Ready() {
		t.Fatal("empty route table remained ready")
	}
}
