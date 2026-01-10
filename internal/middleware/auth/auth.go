package auth

import (
	"crypto/sha256"
	"crypto/subtle"
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

type identity struct {
	digest       [sha256.Size]byte
	userID, role string
}

func New(cfg config.AuthConfig) func(http.Handler) http.Handler {
	keys := cfg.Credentials
	if cfg.DemoCredentials {
		keys = []config.Credential{
			{Key: "secret-key-123", UserID: "user-001", Role: "admin"},
			{Key: "guest-key-456", UserID: "user-002", Role: "viewer"},
			{Key: "member-key-789", UserID: "user-003", Role: "member"},
		}
	}
	identities := make([]identity, 0, len(keys))
	for _, key := range keys {
		if key.Key != "" {
			identities = append(identities, identity{sha256.Sum256([]byte(key.Key)), key.UserID, key.Role})
		}
	}
	signingKey := []byte(cfg.InternalSigningKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if nominatesAuthorization(r) {
				http.Error(w, "Invalid Connection header", http.StatusBadRequest)
				return
			}
			key := extractKey(r, cfg.ApiKeyHeader)
			digest := sha256.Sum256([]byte(key))
			matched := -1
			for i, who := range identities {
				matched = subtle.ConstantTimeSelect(subtle.ConstantTimeCompare(who.digest[:], digest[:]), i, matched)
			}
			if matched < 0 {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			who := identities[matched]
			internalToken, err := generateInternalToken(who.userID, who.role, signingKey)
			if err != nil {
				slog.Error("Failed to generate internal token", "error", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			out := r.Clone(r.Context())
			if cfg.ApiKeyHeader != "" {
				out.Header.Del(cfg.ApiKeyHeader)
			}
			out.Header.Set("Authorization", "Bearer "+internalToken)
			if out.Header.Get("X-Request-ID") == "" {
				out.Header.Set("X-Request-ID", uuid.NewString())
			}
			next.ServeHTTP(w, out)
		})
	}
}

func nominatesAuthorization(r *http.Request) bool {
	for _, value := range r.Header.Values("Connection") {
		for _, header := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(header), "Authorization") {
				return true
			}
		}
	}
	return false
}

func extractKey(r *http.Request, customHeader string) string {
	bearer := r.Header.Values("Authorization")
	var custom []string
	if customHeader != "" {
		custom = r.Header.Values(customHeader)
	}
	if len(custom) == 1 && len(bearer) == 0 {
		return strings.TrimSpace(custom[0])
	}
	if len(bearer) == 1 && len(custom) == 0 {
		scheme, key, ok := strings.Cut(bearer[0], " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(key)
		}
	}
	return ""
}

func generateInternalToken(userID, role string, secretKey []byte) (string, error) {
	now := time.Now()
	claims := InternalClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			Issuer:    "gateway",
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secretKey)
}
