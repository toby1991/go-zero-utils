package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
	"github.com/toby1991/go-zero-utils/bizredis"
)

func TestThrottleMiddlewareDoesNotRenewTTL(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 1, 60)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	allowed := httptest.NewRecorder()
	handler(allowed, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))
	rejected := httptest.NewRecorder()
	handler(rejected, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))
	if allowed.Code != http.StatusNoContent || rejected.Code != http.StatusTooManyRequests {
		t.Fatalf("statuses = %d, %d; want 204 then 429", allowed.Code, rejected.Code)
	}

	key := "api_request_throttle:1001:POST:/api/v1/orders"
	counter := redis.counter(key)
	if counter.count != 2 {
		t.Fatalf("count = %d, want 2; rejected requests must still consume the Lua counter", counter.count)
	}
	if counter.expireCalls != 1 {
		t.Fatalf("EXPIRE calls = %d, want 1; later requests must not renew the fixed window", counter.expireCalls)
	}
	if counter.ttlSeconds != 60 {
		t.Fatalf("ttl seconds = %d, want 60", counter.ttlSeconds)
	}
}

func TestThrottleMiddlewareAtomicallyEnforcesConcurrentRequests(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 7, 60)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	const total = 100
	var allowed atomic.Int64
	var rejected atomic.Int64
	var waitGroup sync.WaitGroup
	for range total {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			recorder := httptest.NewRecorder()
			handler(recorder, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))
			switch recorder.Code {
			case http.StatusNoContent:
				allowed.Add(1)
			case http.StatusTooManyRequests:
				rejected.Add(1)
			default:
				t.Errorf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		}()
	}
	waitGroup.Wait()

	if allowed.Load() != 7 {
		t.Fatalf("allowed = %d, want 7", allowed.Load())
	}
	if rejected.Load() != total-7 {
		t.Fatalf("rejected = %d, want %d", rejected.Load(), total-7)
	}
}

func TestThrottleMiddlewareSeparatesScopeAndPrefixNamespaces(t *testing.T) {
	redis := newThrottleFakeRedis()
	user := NewThrottleMiddleware(redis, 1, 60, WithThrottlePrincipalScope("user"))
	admin := NewThrottleMiddleware(redis, 1, 60, WithThrottlePrincipalScope("admin"))
	customPrefix := NewThrottleMiddleware(redis, 1, 60, WithThrottleKeyPrefix("other_throttle"), WithThrottlePrincipalScope("user"))

	for name, middleware := range map[string]*ThrottleMiddleware{
		"user":          user,
		"admin":         admin,
		"custom-prefix": customPrefix,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})(recorder, newThrottleRequest(http.MethodPost, "/api/v1/shared/action", 1001))
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	assertThrottleSeenKey(t, redis, "api_request_throttle:user:1001:POST:/api/v1/shared/action")
	assertThrottleSeenKey(t, redis, "api_request_throttle:admin:1001:POST:/api/v1/shared/action")
	assertThrottleSeenKey(t, redis, "other_throttle:user:1001:POST:/api/v1/shared/action")
}

func TestThrottleMiddlewareRejectsMissingAuthContext(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 1, 60)
	recorder := httptest.NewRecorder()
	calls := 0

	middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
	})(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil))

	if recorder.Code != http.StatusUnauthorized || calls != 0 {
		t.Fatalf("status=%d calls=%d, want 401 and no handler call", recorder.Code, calls)
	}
	assertThrottleResponseCode(t, recorder, "invalid_auth_context")
}

func TestThrottleMiddlewareFailsOpenOnRedisError(t *testing.T) {
	redis := newThrottleFakeRedis()
	redis.scriptErr = errors.New("redis unavailable")
	middleware := NewThrottleMiddleware(redis, 1, 60)

	assertThrottleRedisFailurePolicy(t, middleware, http.StatusNoContent, 1)
}

func TestThrottleMiddlewareFailsClosedOnRedisError(t *testing.T) {
	redis := newThrottleFakeRedis()
	redis.scriptErr = errors.New("redis unavailable")
	middleware := NewThrottleMiddleware(redis, 1, 60, WithThrottleFailClosed())

	assertThrottleRedisFailurePolicy(t, middleware, http.StatusServiceUnavailable, 0)
}

func TestThrottleMiddlewareNilRedisUsesConfiguredFailurePolicy(t *testing.T) {
	t.Run("fail open by default", func(t *testing.T) {
		assertThrottleRedisFailurePolicy(t, NewThrottleMiddleware(nil, 1, 60), http.StatusNoContent, 1)
	})
	t.Run("fail closed when configured", func(t *testing.T) {
		middleware := NewThrottleMiddleware(nil, 1, 60, WithThrottleFailClosed())
		assertThrottleRedisFailurePolicy(t, middleware, http.StatusServiceUnavailable, 0)
	})
}

func TestThrottleMiddlewareIgnoresQueryAndBodyAndPreservesBody(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 1, 60)
	body := `{"provider":"actual-provider-is-not-a-key-dimension"}`
	request := newThrottleRequest(http.MethodPost, "/api/v1/orders/?trace=not-in-key", 1001)
	request.Body = io.NopCloser(strings.NewReader(body))

	recorder := httptest.NewRecorder()
	middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != body {
			t.Fatalf("handler body = %q, want %q", raw, body)
		}
		w.WriteHeader(http.StatusNoContent)
	})(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	assertThrottleSeenKey(t, redis, "api_request_throttle:1001:POST:/api/v1/orders")
	for _, key := range redis.keys() {
		if strings.Contains(key, "trace=") || strings.Contains(key, "actual-provider-is-not-a-key-dimension") {
			t.Fatalf("throttle key leaked query or body: %q", key)
		}
	}
}

func TestThrottleMiddlewareRejectsAfterQuotaIsConsumed(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 1, 60)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	first := httptest.NewRecorder()
	handler(first, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))
	second := httptest.NewRecorder()
	handler(second, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))

	if first.Code != http.StatusNoContent || second.Code != http.StatusTooManyRequests {
		t.Fatalf("statuses = %d, %d; want 204 then 429", first.Code, second.Code)
	}
	assertThrottleResponseCode(t, second, "too_many_requests")
}

func TestThrottleMiddlewareConsumesBeforeHandlerAndDoesNotRefundHandlerFailure(t *testing.T) {
	redis := newThrottleFakeRedis()
	middleware := NewThrottleMiddleware(redis, 1, 60)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		// 下游 provider 失败不影响已经在 handler 前消耗的 HTTP 请求配额。
		http.Error(w, "provider unavailable", http.StatusBadGateway)
	})

	first := httptest.NewRecorder()
	handler(first, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))
	second := httptest.NewRecorder()
	handler(second, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))

	if first.Code != http.StatusBadGateway || second.Code != http.StatusTooManyRequests {
		t.Fatalf("statuses = %d, %d; want 502 then 429", first.Code, second.Code)
	}
	assertThrottleResponseCode(t, second, "too_many_requests")
}

func assertThrottleRedisFailurePolicy(t *testing.T, middleware *ThrottleMiddleware, wantStatus, wantCalls int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	calls := 0
	middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	})(recorder, newThrottleRequest(http.MethodPost, "/api/v1/orders", 1001))

	if recorder.Code != wantStatus || calls != wantCalls {
		t.Fatalf("status=%d calls=%d, want status=%d calls=%d", recorder.Code, calls, wantStatus, wantCalls)
	}
	if wantStatus == http.StatusServiceUnavailable {
		assertThrottleResponseCode(t, recorder, "throttle_unavailable")
	}
}

func assertThrottleResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["code"] != wantCode {
		t.Fatalf("response = %+v, want code %q", response, wantCode)
	}
}

func newThrottleRequest(method, target string, authID uint64) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	return requestcontext.NewRequestWithAuthContext(request, &requestcontext.AuthContext{AuthId: authID})
}

func assertThrottleSeenKey(t *testing.T, redis *throttleFakeRedis, key string) {
	t.Helper()
	for _, seenKey := range redis.keys() {
		if seenKey == key {
			return
		}
	}
	t.Fatalf("seen keys = %+v, want %q", redis.keys(), key)
}

type throttleCounter struct {
	count       int64
	ttlSeconds  int
	expireCalls int
}

type throttleFakeRedis struct {
	mu        sync.Mutex
	counters  map[string]throttleCounter
	seenKeys  map[string]struct{}
	scriptErr error
}

func newThrottleFakeRedis() *throttleFakeRedis {
	return &throttleFakeRedis{
		counters: make(map[string]throttleCounter),
		seenKeys: make(map[string]struct{}),
	}
}

func (f *throttleFakeRedis) ScriptLoad(script string) (string, error) {
	return "", nil
}

func (f *throttleFakeRedis) ScriptLoadCtx(ctx context.Context, script string) (string, error) {
	return "", nil
}

func (f *throttleFakeRedis) ScriptRun(script *bizredis.Script, keys []string, args ...any) (any, error) {
	return f.ScriptRunCtx(context.Background(), script, keys, args...)
}

// ScriptRunCtx 以同一把 mutex 模拟 Lua 脚本的原子 INCR 和首次 EXPIRE 语义。
func (f *throttleFakeRedis) ScriptRunCtx(ctx context.Context, script *bizredis.Script, keys []string, args ...any) (any, error) {
	if len(keys) != 1 || len(args) != 1 {
		return nil, errors.New("unexpected throttle script arguments")
	}
	rawArgs, ok := args[0].([]string)
	if !ok || len(rawArgs) != 1 {
		return nil, errors.New("unexpected throttle script arguments")
	}

	seconds, err := strconv.Atoi(rawArgs[0])
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.scriptErr != nil {
		return nil, f.scriptErr
	}

	key := keys[0]
	f.seenKeys[key] = struct{}{}
	counter := f.counters[key]
	counter.count++
	if counter.count == 1 {
		counter.ttlSeconds = seconds
		counter.expireCalls++
	}
	f.counters[key] = counter
	return counter.count, nil
}

func (f *throttleFakeRedis) counter(key string) throttleCounter {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counters[key]
}

func (f *throttleFakeRedis) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys := make([]string, 0, len(f.seenKeys))
	for key := range f.seenKeys {
		keys = append(keys, key)
	}
	return keys
}
