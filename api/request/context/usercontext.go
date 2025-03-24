package context

import (
	"context"
	"net/http"
)

const userContextKey = "userContext"

type UserContext struct {
	UserId uint64
}

func NewRequestWithUserContext(r *http.Request, userContext *UserContext) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userContextKey, userContext))
}

func GetUserContext(ctx context.Context) *UserContext {
	return ctx.Value(userContextKey).(*UserContext)
}
