package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
)

const authorizationHeaderKey = "Authorization"

type AuthResolver interface {
	ResolveAuth(ctx context.Context, credential string) (authId uint64, err error)
}

type AuthMiddleware struct {
	resolver            AuthResolver
	credentialHeaderKey string
}

func NewAuthMiddleware(resolver AuthResolver, credentialHeaderKey string) *AuthMiddleware {
	return &AuthMiddleware{
		resolver:            resolver,
		credentialHeaderKey: credentialHeaderKey,
	}
}

func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		credential, err := m.extractCredential(r)
		if err != nil {
			writeUnauthorized(w)
			return
		}

		if m.resolver == nil {
			writeUnauthorized(w)
			return
		}

		authId, err := m.resolver.ResolveAuth(r.Context(), credential)
		if err != nil || authId == 0 {
			writeUnauthorized(w)
			return
		}

		next(w, requestcontext.NewRequestWithAuthContext(r, &requestcontext.AuthContext{
			AuthId: authId,
		}))
	}
}

func (m *AuthMiddleware) extractCredential(r *http.Request) (string, error) {
	headerKey := strings.TrimSpace(m.credentialHeaderKey)
	if headerKey == "" {
		return "", errors.New("credential header key is required")
	}

	credential := strings.TrimSpace(r.Header.Get(headerKey))
	if credential == "" {
		return "", errors.New("credential is required")
	}

	if strings.EqualFold(headerKey, authorizationHeaderKey) {
		parts := strings.Fields(credential)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			return "", errors.New("bearer credential is required")
		}
		return parts[1], nil
	}

	return credential, nil
}

func writeUnauthorized(w http.ResponseWriter) {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
