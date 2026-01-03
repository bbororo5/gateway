package utils

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestTrustedProxyChain(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/24"), netip.MustParsePrefix("2001:db8::/64")}
	handler := TrustClientIP(trusted, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(GetClientIP(r) + "|" + GetClientScheme(r)))
	}))
	for _, test := range []struct{ name, peer, forwarded, want string }{
		{"direct spoof", "192.0.2.3:40", "203.0.113.99", "192.0.2.3|http"},
		{"trusted chain", "10.0.0.1:40", "203.0.113.99, 192.0.2.3, 10.0.0.2", "192.0.2.3|https"},
		{"trusted IPv6", "[2001:db8::1]:40", "2001:db8:1::2", "2001:db8:1::2|https"},
		{"malformed hop", "10.0.0.1:40", "192.0.2.3, invalid", "10.0.0.1|https"},
		{"mapped IPv4 peer", "[::ffff:10.0.0.1]:40", "192.0.2.3", "192.0.2.3|https"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = test.peer
			r.Header.Set("X-Forwarded-For", test.forwarded)
			r.Header.Set("X-Forwarded-Proto", "https")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if got := w.Body.String(); got != test.want {
				t.Fatalf("IP = %q, want %q", got, test.want)
			}
		})
	}
}
