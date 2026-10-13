package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
)

// NewSessionAPIKey creates a fresh credential for this process lifetime only.
// The key is intentionally not persisted to disk or the database.
func NewSessionAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "mp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// RequireSessionAPIKey protects the merchant API with the current process-session key.
// It accepts either Authorization: Bearer <key> or X-API-Key: <key>.
func RequireSessionAPIKey(sessionKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := extractAPIKey(r)
			if strings.TrimSpace(sessionKey) == "" || provided == "" || !constantTimeEqual(provided, sessionKey) {
				w.Header().Set("WWW-Authenticate", `Bearer realm="monero-pay-api"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractAPIKey(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-API-Key")); value != "" {
		return value
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:6], "Bearer") && auth[6] == ' ' {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
