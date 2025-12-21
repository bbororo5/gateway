package auth

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jeonseon-ung/gateway/internal/config"
)

type InternalClaims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type AuthConfig struct {
	Enabled      bool   `yaml:"enabled"`
	ApiKeyHeader string `yaml:"api_key_header"`
	InternalSigningKey string `yaml:"internal_signing_key"`
}

func New(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. 헤더 추출
			apiKey := extractKey(r)
			if apiKey == "" {
				slog.Warn("Missing API Key or Token", "ip", r.RemoteAddr)
				http.Error(w, "Unauthorized: Identity required", http.StatusUnauthorized)
				return
			}

			// 2. 외부 인증
			userID, role, ok := validateApiKey(apiKey)
			if !ok {
				slog.Warn("Invalid API Key", "key", apiKey, "ip", r.RemoteAddr)
				http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
				return
			}

			// 3. Zero Trust & Auth Offloading
			internalToken, err := generateInternalToken(userID, role, []byte(cfg.InternalSigningKey))
			if err != nil {
				slog.Error("Failed to generate internal token", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// 4. 헤더 교체
			r.Header.Del("Authorization")
			r.Header.Set("Authorization", "Bearer "+internalToken)

			// 5. 로깅 미들웨어가 꺼졌을 때를 대비하여 추적성 보완
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.NewString()
				r.Header.Set("X-Request-ID", requestID)
			}

			slog.Info("Auth successful (Offloaded)",
				"request_id", requestID,
				"user_id", userID,
				"role", role,
			)

			// 6. 다음 핸들러 호출
			next.ServeHTTP(w, r)
		})
	}
}

func extractKey(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

func generateInternalToken(userID, role string, secretKey []byte) (string, error) {
	claims := InternalClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Minute)),
			Issuer:    "gateway",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}

func validateApiKey(key string) (string, string, bool) {
	validKeys := map[string]struct {
		UserID string
		Role   string
	}{
		"secret-key-123": {UserID: "user-001", Role: "admin"},
		"guest-key-456":  {UserID: "user-002", Role: "viewer"},
		"member-key-789": {UserID: "user-003", Role: "member"},
	}

	user, exists := validKeys[key]
	if !exists {
		return "", "", false
	}
	return user.UserID, user.Role, true
}
