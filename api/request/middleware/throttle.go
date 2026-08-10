package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
	"github.com/toby1991/go-zero-utils/bizredis"
	"github.com/toby1991/go-zero/core/logx"
	"github.com/toby1991/go-zero/rest/httpx"
)

const (
	defaultThrottleTimes   = 1
	defaultThrottleSeconds = 60

	// DefaultThrottleKeyPrefix 是 HTTP 请求限流计数器的默认 Redis key 前缀。
	DefaultThrottleKeyPrefix = "api_request_throttle"
)

var throttleScript = bizredis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
    redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return count
`)

// ThrottleOption 调整请求限流 middleware 的 Redis key 空间和故障策略。
type ThrottleOption func(*throttleConfig)

type throttleConfig struct {
	keyPrefix      string
	principalScope string
	failClosed     bool
}

// WithThrottleKeyPrefix 设置 Redis 计数器 key 前缀。
//
// 同一个 Redis namespace 内存在多套无关的 HTTP 限流时，调用方应使用不同 prefix，
// 让它们的计数器彼此独立。
func WithThrottleKeyPrefix(prefix string) ThrottleOption {
	return func(config *throttleConfig) {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" {
			config.keyPrefix = prefix
		}
	}
}

// WithThrottlePrincipalScope 设置身份作用域，例如 "user"、"admin"、"agent"。
//
// AuthContext.AuthId 只有数字，不携带 principal 类型。不同身份域可能使用相同的
// AuthId，所以共用 Redis namespace、method 和 path 时必须显式设置 scope，避免错误共享配额。
func WithThrottlePrincipalScope(scope string) ThrottleOption {
	return func(config *throttleConfig) {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			config.principalScope = scope
		}
	}
}

// WithThrottleFailClosed 使 Redis 不可用时返回 503，而不是放行请求。
//
// 默认策略与 AntiDoubleClickMiddleware 一致：Redis 故障时 fail-open，保证可用性。
// 对配额本身就是安全或成本边界的接口，调用方应显式启用此选项。
func WithThrottleFailClosed() ThrottleOption {
	return func(config *throttleConfig) {
		config.failClosed = true
	}
}

// ThrottleMiddleware 对已认证 HTTP 请求执行 Laravel 风格的固定窗口限流。
//
// 每个进入 middleware 的请求都会在认证完成、进入业务 handler 前消耗一次配额；
// 它只使用 AuthContext.AuthId、HTTP method 和 cleaned static URL path 建 key，
// 不读取 query/body，也不根据下游实际 provider 的成败回滚计数。
// 因此它不是业务幂等、provider 配额或 Redis 锁的替代品。
type ThrottleMiddleware struct {
	redis          bizredis.RedisScripter
	times          int
	seconds        int
	keyPrefix      string
	principalScope string
	failClosed     bool
}

// NewThrottleMiddleware 构造 HTTP 请求限流 middleware。
//
// times 是固定窗口 seconds 内允许进入 handler 的请求数。非正值分别回退为 1 次和 60 秒，
// 防止无意中配置出永不生效或无过期时间的 Redis 计数器。默认 Redis 故障 fail-open；
// 需要严格拒绝时使用 WithThrottleFailClosed。
func NewThrottleMiddleware(redis bizredis.RedisScripter, times, seconds int, opts ...ThrottleOption) *ThrottleMiddleware {
	if times <= 0 {
		times = defaultThrottleTimes
	}
	if seconds <= 0 {
		seconds = defaultThrottleSeconds
	}

	config := throttleConfig{keyPrefix: DefaultThrottleKeyPrefix}
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}

	return &ThrottleMiddleware{
		redis:          redis,
		times:          times,
		seconds:        seconds,
		keyPrefix:      config.keyPrefix,
		principalScope: config.principalScope,
		failClosed:     config.failClosed,
	}
}

// Handle 在认证 middleware 之后、业务 handler 之前消耗当前请求的限流配额。
func (m *ThrottleMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authID, ok := throttleAuthenticatedRequestID(r.Context())
		if !ok {
			writeThrottleJSON(r.Context(), w, http.StatusUnauthorized, "invalid_auth_context", "invalid auth context")
			return
		}

		method := strings.ToUpper(strings.TrimSpace(r.Method))
		routePath := cleanThrottlePath(r.URL.Path)
		if m.redis == nil {
			m.handleRedisFailure(r.Context(), w, next, r, method, routePath, errors.New("redis client is nil"))
			return
		}

		count, err := m.increment(r.Context(), authID, method, routePath)
		if err != nil {
			m.handleRedisFailure(r.Context(), w, next, r, method, routePath, err)
			return
		}
		if count > int64(m.times) {
			writeThrottleJSON(r.Context(), w, http.StatusTooManyRequests, "too_many_requests", "too many requests")
			return
		}

		next(w, r)
	}
}

func (m *ThrottleMiddleware) increment(ctx context.Context, authID uint64, method, routePath string) (int64, error) {
	key, err := buildThrottleKey(m.keyPrefix, m.principalScope, authID, method, routePath)
	if err != nil {
		return 0, err
	}

	response, err := m.redis.ScriptRunCtx(ctx, throttleScript, []string{key}, []string{strconv.Itoa(m.seconds)})
	if err != nil {
		return 0, err
	}
	count, ok := response.(int64)
	if !ok || count < 1 {
		return 0, fmt.Errorf("unexpected throttle script response: %v", response)
	}
	return count, nil
}

func (m *ThrottleMiddleware) handleRedisFailure(ctx context.Context, w http.ResponseWriter, next http.HandlerFunc, r *http.Request, method, routePath string, err error) {
	logx.WithContext(ctx).Errorf("throttle increment failed for %s %s: %v", method, routePath, err)
	if m.failClosed {
		writeThrottleJSON(ctx, w, http.StatusServiceUnavailable, "throttle_unavailable", "request throttle unavailable")
		return
	}
	next(w, r)
}

func buildThrottleKey(keyPrefix, principalScope string, authID uint64, method, routePath string) (string, error) {
	keyPrefix = strings.TrimSpace(keyPrefix)
	if keyPrefix == "" {
		return "", errors.New("key prefix is required")
	}
	if authID == 0 {
		return "", errors.New("auth id is required")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return "", errors.New("method is required")
	}
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return "", errors.New("route path is required")
	}

	principalScope = strings.TrimSpace(principalScope)
	if principalScope != "" {
		return fmt.Sprintf("%s:%s:%d:%s:%s", keyPrefix, principalScope, authID, method, routePath), nil
	}
	return fmt.Sprintf("%s:%d:%s:%s", keyPrefix, authID, method, routePath), nil
}

func throttleAuthenticatedRequestID(ctx context.Context) (authID uint64, ok bool) {
	defer func() {
		if recover() != nil {
			authID = 0
			ok = false
		}
	}()

	authContext := requestcontext.GetAuthContext(ctx)
	if authContext == nil || authContext.AuthId == 0 {
		return 0, false
	}
	return authContext.AuthId, true
}

func cleanThrottlePath(routePath string) string {
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return "/"
	}
	if !strings.HasPrefix(routePath, "/") {
		routePath = "/" + routePath
	}
	return path.Clean(routePath)
}

func writeThrottleJSON(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	httpx.WriteJsonCtx(ctx, w, status, map[string]string{
		"code":    code,
		"message": message,
	})
}
