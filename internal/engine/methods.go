package engine

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
)

func normalizeMethods(methods []string) ([]string, error) {
	if len(methods) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(methods)+1)
	for _, method := range methods {
		if method == "" || strings.IndexFunc(method, func(r rune) bool {
			return !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-')
		}) >= 0 {
			return nil, fmt.Errorf("invalid allowed method %q", method)
		}
		if seen[method] {
			return nil, fmt.Errorf("duplicate allowed method %q", method)
		}
		seen[method] = true
	}
	if seen[http.MethodGet] {
		seen[http.MethodHead] = true
	}
	normalized := make([]string, 0, len(seen))
	for method := range seen {
		normalized = append(normalized, method)
	}
	slices.Sort(normalized)
	return normalized, nil
}
