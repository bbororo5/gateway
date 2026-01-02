package utils

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type peerKey struct{}

type peerInfo struct {
	ip     string
	scheme string
}

func GetClientIP(r *http.Request) string {
	if peer, ok := r.Context().Value(peerKey{}).(peerInfo); ok {
		return peer.ip
	}
	return peerIP(r.RemoteAddr)
}

func GetClientScheme(r *http.Request) string {
	if peer, ok := r.Context().Value(peerKey{}).(peerInfo); ok && peer.scheme != "" {
		return peer.scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func peerIP(address string) string {
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	if ip, err := netip.ParseAddr(address); err == nil {
		return ip.Unmap().String()
	}
	return address
}

// TrustClientIP walks the forwarding chain from the connected peer toward the client.
func TrustClientIP(trusted []netip.Prefix, next http.Handler) http.Handler {
	isTrusted := func(ip netip.Addr) bool {
		for _, prefix := range trusted {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := peerInfo{ip: peerIP(r.RemoteAddr)}
		ip, err := netip.ParseAddr(info.ip)
		if err == nil && isTrusted(ip) {
			if proto := r.Header.Values("X-Forwarded-Proto"); len(proto) == 1 && (proto[0] == "http" || proto[0] == "https") {
				info.scheme = proto[0]
			}
			chain := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
			for i := len(chain) - 1; i >= 0 && isTrusted(ip); i-- {
				hop, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
				if err != nil || hop.Zone() != "" {
					ip, _ = netip.ParseAddr(info.ip)
					break
				}
				ip = hop.Unmap()
			}
			info.ip = ip.String()
		}
		ctx := context.WithValue(r.Context(), peerKey{}, info)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
