package context

import (
	"context"
	"net/http"
)

const authContextKey = "authContext"

type AuthContext struct {
	AuthId uint64
}

func NewRequestWithAuthContext(r *http.Request, authContext *AuthContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), authContextKey, authContext))
}

func GetAuthContext(ctx context.Context) *AuthContext {
	return ctx.Value(authContextKey).(*AuthContext)
}
