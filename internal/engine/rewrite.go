package engine

import (
	"net/url"
	"strings"
)

func stripRoutePrefix(u *url.URL, prefix string) {
	// The engine has already rejected encoded slashes and ambiguous paths.
	if u.RawPath != "" {
		parts := strings.SplitN(u.EscapedPath(), "/", strings.Count(prefix, "/")+2)
		u.RawPath = ""
		if len(parts) == strings.Count(prefix, "/")+2 {
			u.RawPath = "/" + parts[len(parts)-1]
		}
	}
	u.Path = strings.TrimPrefix(u.Path, prefix)
	if u.Path == "" {
		u.Path = "/"
	}
}
