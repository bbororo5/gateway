package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckConfigContract(t *testing.T) {
	const route = "routes: [{prefix: /api, upstreams: ['http://localhost']}]\n"
	const fileConfig = "discovery: {provider: file, route_file: ROUTE}\n"
	cases := []struct {
		name, config, routes, wantError string
	}{
		{"occupied port", "server: {port: PORT}\ndiscovery: {provider: mock}\n", route, ""},
		{"relative route file", strings.ReplaceAll(fileConfig, "ROUTE", "routes.yaml"), route, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			occupied, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer occupied.Close()
			_, port, err := net.SplitHostPort(occupied.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			routePath := filepath.Join(dir, "routes.yaml")
			if tc.routes != "" {
				if err := os.WriteFile(routePath, []byte(tc.routes), 0600); err != nil {
					t.Fatal(err)
				}
			}
			configPath := filepath.Join(dir, "gateway.yaml")
			config := strings.NewReplacer("PORT", port, "ROUTE", routePath).Replace(tc.config)
			if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err = run(ctx, configPath, true)
			if tc.wantError == "" {
				if err != nil || ctx.Err() != nil {
					t.Fatalf("configuration check failed: %v (%v)", err, ctx.Err())
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got %v; want error containing %q", err, tc.wantError)
			}
		})
	}
}
