package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
)

type fakeAuthResolver struct {
	credential string
	authId     uint64
	err        error
}

func (f *fakeAuthResolver) ResolveAuth(ctx context.Context, credential string) (uint64, error) {
	f.credential = credential
	if f.err != nil {
		return 0, f.err
	}
	return f.authId, nil
}

func TestAuthMiddlewareWritesAuthContextFromBearerAuthorization(t *testing.T) {
	resolver := &fakeAuthResolver{authId: 42}
	m := NewAuthMiddleware(resolver, "Authorization")
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()
	var gotAuthId uint64

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		gotAuthId = requestcontext.GetAuthContext(r.Context()).AuthId
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if resolver.credential != "access-token" {
		t.Fatalf("credential = %q, want access-token", resolver.credential)
	}
	if gotAuthId != 42 {
		t.Fatalf("auth id = %d, want 42", gotAuthId)
	}
}

func TestAuthMiddlewareUsesCustomCredentialHeader(t *testing.T) {
	resolver := &fakeAuthResolver{authId: 77}
	m := NewAuthMiddleware(resolver, "X-Agent-API-Key")
	req := httptest.NewRequest(http.MethodPost, "/agent/challenges", nil)
	req.Header.Set("X-Agent-API-Key", " cfk_test_key ")
	rec := httptest.NewRecorder()
	var gotAuthId uint64

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		gotAuthId = requestcontext.GetAuthContext(r.Context()).AuthId
		w.WriteHeader(http.StatusOK)
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if resolver.credential != "cfk_test_key" {
		t.Fatalf("credential = %q, want cfk_test_key", resolver.credential)
	}
	if gotAuthId != 77 {
		t.Fatalf("auth id = %d, want 77", gotAuthId)
	}
}

func TestAuthMiddlewareRejectsMissingCredential(t *testing.T) {
	resolver := &fakeAuthResolver{authId: 1}
	m := NewAuthMiddleware(resolver, "X-Agent-API-Key")
	req := httptest.NewRequest(http.MethodGet, "/agent", nil)
	rec := httptest.NewRecorder()
	called := false

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})(rec, req)

	if called {
		t.Fatal("next handler should not run")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if resolver.credential != "" {
		t.Fatalf("resolver should not receive credential, got %q", resolver.credential)
	}
}

func TestAuthMiddlewareRejectsMalformedBearer(t *testing.T) {
	resolver := &fakeAuthResolver{authId: 1}
	m := NewAuthMiddleware(resolver, "Authorization")
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Token access-token")
	rec := httptest.NewRecorder()

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if resolver.credential != "" {
		t.Fatalf("resolver should not receive credential, got %q", resolver.credential)
	}
}

func TestAuthMiddlewareRejectsResolverError(t *testing.T) {
	resolver := &fakeAuthResolver{err: errors.New("token invalid")}
	m := NewAuthMiddleware(resolver, "Authorization")
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	rec := httptest.NewRecorder()

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if resolver.credential != "bad-token" {
		t.Fatalf("credential = %q, want bad-token", resolver.credential)
	}
}

func TestAuthMiddlewareRejectsZeroAuthID(t *testing.T) {
	resolver := &fakeAuthResolver{authId: 0}
	m := NewAuthMiddleware(resolver, "Authorization")
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	rec := httptest.NewRecorder()

	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not run")
	})(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
