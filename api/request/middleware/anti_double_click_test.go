package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
	"github.com/toby1991/go-zero-utils/bizredis"
)

func TestAntiDoubleClickMiddlewareRejectsDuplicateInFlightRequestWith429(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)

	entered := make(chan struct{})
	release := make(chan struct{})
	handler := m.Handle(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler(httptest.NewRecorder(), newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))
	}()
	<-entered

	rec := httptest.NewRecorder()
	calls := 0
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))

	close(release)
	<-firstDone

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want duplicate not to enter next", calls)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "duplicate_request" {
		t.Fatalf("body = %+v, want duplicate_request code", body)
	}
}

func TestAntiDoubleClickMiddlewareUsesPrincipalScopeToSeparateSameAuthID(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	userMiddleware := NewAntiDoubleClickMiddleware(redis, 3, WithAntiDoubleClickPrincipalScope("user"))
	adminMiddleware := NewAntiDoubleClickMiddleware(redis, 3, WithAntiDoubleClickPrincipalScope("admin"))

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		userMiddleware.Handle(func(w http.ResponseWriter, r *http.Request) {
			close(entered)
			<-release
			w.WriteHeader(http.StatusNoContent)
		})(httptest.NewRecorder(), newAntiDoubleClickRequest(http.MethodPost, "/api/v1/shared/action", 1001))
	}()
	<-entered

	adminRec := httptest.NewRecorder()
	adminCalls := 0
	adminMiddleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		adminCalls++
		w.WriteHeader(http.StatusNoContent)
	})(adminRec, newAntiDoubleClickRequest(http.MethodPost, "/api/v1/shared/action", 1001))

	duplicateUserRec := httptest.NewRecorder()
	userCalls := 0
	userMiddleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		userCalls++
	})(duplicateUserRec, newAntiDoubleClickRequest(http.MethodPost, "/api/v1/shared/action", 1001))

	close(release)
	<-firstDone

	if adminRec.Code != http.StatusNoContent || adminCalls != 1 {
		t.Fatalf("admin scoped request status=%d calls=%d, want pass-through", adminRec.Code, adminCalls)
	}
	if duplicateUserRec.Code != http.StatusTooManyRequests || userCalls != 0 {
		t.Fatalf("duplicate user status=%d calls=%d, want duplicate rejection", duplicateUserRec.Code, userCalls)
	}
	assertAntiDoubleClickSeenKey(t, redis, "api_anti_double_click_lock:user:1001:POST:/api/v1/shared/action")
	assertAntiDoubleClickSeenKey(t, redis, "api_anti_double_click_lock:admin:1001:POST:/api/v1/shared/action")
}

func TestAntiDoubleClickMiddlewareUsesCustomKeyPrefix(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3, WithAntiDoubleClickKeyPrefix("landingpage_anti_double_click"))

	rec := httptest.NewRecorder()
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/password", 1001))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	assertAntiDoubleClickSeenKey(t, redis, "landingpage_anti_double_click:1001:PUT:/api/v1/user/password")
}

func TestAntiDoubleClickMiddlewareUsesCleanPathAndLeavesBodyReadable(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)
	body := `{"resource_id":"123","secret":"keep-out-of-lock-key"}`
	var downstreamBody string

	rec := httptest.NewRecorder()
	req := newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname?tab=profile", 1001)
	req.Body = io.NopCloser(strings.NewReader(body))
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		downstreamBody = string(raw)
		w.WriteHeader(http.StatusNoContent)
	})(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if downstreamBody != body {
		t.Fatalf("downstream body = %q, want %q", downstreamBody, body)
	}

	key := "api_anti_double_click_lock:1001:PUT:/api/v1/user/nickname"
	assertAntiDoubleClickSeenKey(t, redis, key)
	redis.lockMu.Lock()
	defer redis.lockMu.Unlock()
	for seenKey := range redis.seenKeys {
		if strings.Contains(seenKey, "tab=") || strings.Contains(seenKey, "keep-out-of-lock-key") {
			t.Fatalf("lock key leaked query or body: %q", seenKey)
		}
	}
}

func TestAntiDoubleClickMiddlewareRejectsMissingAuthContext(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)

	rec := httptest.NewRecorder()
	calls := 0
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
	})(rec, httptest.NewRequest(http.MethodPut, "/api/v1/user/nickname", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want auth failure not to enter next", calls)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "invalid_auth_context" {
		t.Fatalf("body = %+v, want invalid_auth_context", body)
	}
}

func TestAntiDoubleClickMiddlewareFailsOpenOnRedisAcquireError(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	redis.scriptErr = errors.New("redis unavailable")
	m := NewAntiDoubleClickMiddleware(redis, 3)

	rec := httptest.NewRecorder()
	calls := 0
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want fail-open to enter next", calls)
	}
}

func TestAntiDoubleClickMiddlewareReleasesAfterHandlerErrorResponse(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)

	first := httptest.NewRecorder()
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "validation failed", http.StatusBadRequest)
	})(first, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))
	if first.Code != http.StatusBadRequest {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	calls := 0
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})(second, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))
	if second.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("second status=%d calls=%d, want lock released after error response", second.Code, calls)
	}
}

func TestAntiDoubleClickMiddlewareReleasesWhenHandlerPanics(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected handler panic")
			}
		}()
		m.Handle(func(w http.ResponseWriter, r *http.Request) {
			panic("handler failed")
		})(httptest.NewRecorder(), newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))
	}()

	rec := httptest.NewRecorder()
	calls := 0
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))
	if rec.Code != http.StatusNoContent || calls != 1 {
		t.Fatalf("status=%d calls=%d, want lock released after panic", rec.Code, calls)
	}
}

func TestAntiDoubleClickMiddlewareReleaseDoesNotDeleteDifferentOwner(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	m := NewAntiDoubleClickMiddleware(redis, 3)
	key := "api_anti_double_click_lock:1001:PUT:/api/v1/user/password"

	rec := httptest.NewRecorder()
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		redis.setLock(key, "other-owner")
		w.WriteHeader(http.StatusNoContent)
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/password", 1001))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !redis.hasLock(key) {
		t.Fatal("release deleted a lock owned by a different owner")
	}
}

func TestAntiDoubleClickMiddlewareDoesNotOverwriteResponseOnReleaseError(t *testing.T) {
	redis := newAntiDoubleClickFakeRedis()
	redis.releaseErr = errors.New("release failed")
	m := NewAntiDoubleClickMiddleware(redis, 3)

	rec := httptest.NewRecorder()
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "handler response", http.StatusTeapot)
	})(rec, newAntiDoubleClickRequest(http.MethodPut, "/api/v1/user/nickname", 1001))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("status = %d body=%s, want handler response preserved", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "handler response") {
		t.Fatalf("body = %q, want handler body preserved", rec.Body.String())
	}
}

func newAntiDoubleClickRequest(method, target string, authID uint64) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return requestcontext.NewRequestWithAuthContext(req, &requestcontext.AuthContext{AuthId: authID})
}

func assertAntiDoubleClickSeenKey(t *testing.T, redis *antiDoubleClickFakeRedis, key string) {
	t.Helper()
	redis.lockMu.Lock()
	defer redis.lockMu.Unlock()
	if _, ok := redis.seenKeys[key]; !ok {
		t.Fatalf("seen keys = %+v, want %q", redis.seenKeys, key)
	}
}

type antiDoubleClickFakeRedis struct {
	lockMu     sync.Mutex
	locks      map[string]string
	seenKeys   map[string]struct{}
	scriptErr  error
	releaseErr error
}

func newAntiDoubleClickFakeRedis() *antiDoubleClickFakeRedis {
	return &antiDoubleClickFakeRedis{
		locks:    make(map[string]string),
		seenKeys: make(map[string]struct{}),
	}
}

func (f *antiDoubleClickFakeRedis) ScriptLoad(script string) (string, error) {
	return "", nil
}

func (f *antiDoubleClickFakeRedis) ScriptLoadCtx(ctx context.Context, script string) (string, error) {
	return "", nil
}

func (f *antiDoubleClickFakeRedis) ScriptRun(script *bizredis.Script, keys []string, args ...any) (any, error) {
	return f.ScriptRunCtx(context.Background(), script, keys, args...)
}

func (f *antiDoubleClickFakeRedis) ScriptRunCtx(ctx context.Context, script *bizredis.Script, keys []string, args ...any) (any, error) {
	if len(keys) != 1 || len(args) != 1 {
		return nil, nil
	}
	raw, ok := args[0].([]string)
	if !ok || len(raw) == 0 {
		return nil, nil
	}

	f.lockMu.Lock()
	defer f.lockMu.Unlock()

	key := keys[0]
	f.seenKeys[key] = struct{}{}
	lockID := raw[0]
	switch len(raw) {
	case 2:
		if f.scriptErr != nil {
			return nil, f.scriptErr
		}
		if _, exists := f.locks[key]; exists {
			return nil, nil
		}
		f.locks[key] = lockID
		return "OK", nil
	case 1:
		if f.releaseErr != nil {
			return nil, f.releaseErr
		}
		if f.locks[key] == lockID {
			delete(f.locks, key)
			return int64(1), nil
		}
		return int64(0), nil
	default:
		return nil, nil
	}
}

func (f *antiDoubleClickFakeRedis) setLock(key, owner string) {
	f.lockMu.Lock()
	defer f.lockMu.Unlock()
	f.locks[key] = owner
}

func (f *antiDoubleClickFakeRedis) hasLock(key string) bool {
	f.lockMu.Lock()
	defer f.lockMu.Unlock()
	_, ok := f.locks[key]
	return ok
}
