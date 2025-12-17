package auth

import (
	"crypto/subtle"
    "log"
    "strings"
	"github.com/mitchellh/mapstructure"
	"net/http"
)

type Config struct {
	Enabled bool   `mapstructure:"enabled"`
	Secret  string `mapstructure:"secret"`
	Header  string `mapstructure:"header"` 
}

func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
		Secret:  "default-secret-key",
		Header:  "Authorization",
	}
}

func ParseConfig(rawConfig map[string]interface{}) (*Config, error) {
	config := DefaultConfig()

	if rawConfig == nil {
		return config, nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Metadata:         nil,
		Result:           config,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
	})
	if err != nil {
		return nil, err
	}

	if err := decoder.Decode(rawConfig); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Middleware(next http.Handler) http.Handler {
	//   1. 현재 설정을 읽고 키를 가져와서 파싱하기
	//   2. 키 검증하기
	//   3. 키가 유효하다면 next 핸들러 호출하기
	//   4. 키가 유효하지 않다면 401 Unauthorized 응답 반환하기

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !c.Enabled || c.isAuthorized(r) {
			next.ServeHTTP(w, r)
			return
		}

		c.reject(w, r)
	})
}

func (c *Config) isAuthorized(r *http.Request) bool {
    authHeader := r.Header.Get(c.Header)
    if authHeader == "" {
        return false
    }

    parts := strings.SplitN(authHeader, " ", 2)
    token := parts[0]
    if len(parts) == 2 {
        token = parts[1]
    }

    return subtle.ConstantTimeCompare([]byte(token), []byte(c.Secret)) == 1
}

func (c *Config) reject(w http.ResponseWriter, r *http.Request) {
	log.Printf("[AUTH] Unauthorized: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
    http.Error(w, "Unauthorized", http.StatusUnauthorized)
}