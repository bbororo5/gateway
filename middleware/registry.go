package middleware

import (
	"net/http"
)

type Middleware func(http.Handler) http.Handler

var registry = make(map[string]Middleware)

func Register(name string, m Middleware) {
	registry[name] = m
}

func Get(name string) (Middleware, bool) {
	m, ok := registry[name]
	return m, ok
}
