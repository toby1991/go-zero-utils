package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	requestcontext "github.com/toby1991/go-zero-utils/api/request/context"
	"github.com/toby1991/go-zero-utils/bizredis"
	"github.com/toby1991/go-zero/core/logx"
	"github.com/toby1991/go-zero/rest/httpx"
)

const (
	defaultAntiDoubleClickLockTTLSeconds = 3
	DefaultAntiDoubleClickLockKeyPrefix  = "api_anti_double_click_lock"
)

var ErrAntiDoubleClickInProgress = errors.New("api anti double click lock is already in progress")

// AntiDoubleClickOption 调整防重复点击锁的 key 维度。
type AntiDoubleClickOption func(*antiDoubleClickConfig)

type antiDoubleClickConfig struct {
	keyPrefix      string
	principalScope string
}

// WithAntiDoubleClickKeyPrefix 设置 Redis lock key 前缀。
//
// 同一个 Redis namespace 中如果存在多套互不相关的 API 防重复点击逻辑，
// 使用不同 prefix 可以把锁空间彻底隔开。
func WithAntiDoubleClickKeyPrefix(prefix string) AntiDoubleClickOption {
	return func(config *antiDoubleClickConfig) {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" {
			config.keyPrefix = prefix
		}
	}
}

// WithAntiDoubleClickPrincipalScope 设置身份作用域，例如 "user"、"admin"、"agent"。
//
// 这个 scope 会写进 Redis lock key。它不是从 AuthContext 自动推断的，调用方必须按
// 当前接口前面的 authContext middleware 明确传入。原因很简单：AuthContext.AuthId
// 只有数字，没有 principal 类型。同一个项目里 admin/user/agent 这类认证 middleware
// 如果都写入了相同数字 AuthId，又共用相同 method+path 和 Redis namespace，不加 scope
// 就会互相挡请求。别把“数字相同”误认为“同一个业务身份”，那是 bug 的温床。
func WithAntiDoubleClickPrincipalScope(scope string) AntiDoubleClickOption {
	return func(config *antiDoubleClickConfig) {
		scope = strings.TrimSpace(scope)
		if scope != "" {
			config.principalScope = scope
		}
	}
}

// AntiDoubleClickMiddleware 对已认证的静态 HTTP 接口做 in-flight 单飞保护。
//
// 它只处理已经进入该 middleware 的请求；哪些接口进入这里应由 go-zero .api 的
// @server middleware 分组决定。锁 key 使用 AuthContext.AuthId、HTTP method 和
// cleaned request path，不读取 query/body，也不解析动态 route template。
//
// 注意：AuthContext.AuthId 只是数字，不包含 user/admin/agent 这类 principal 类型。
// 如果同一个服务、同一个 Redis namespace 下有多套认证 principal 共用相同 method+path，
// 必须通过 WithAntiDoubleClickPrincipalScope 或 WithAntiDoubleClickKeyPrefix 分域。
type AntiDoubleClickMiddleware struct {
	redis          bizredis.RedisScripter
	lockTTLSeconds int
	keyPrefix      string
	principalScope string
}

// NewAntiDoubleClickMiddleware 构造静态 path 防重复点击 middleware。
//
// 默认 Redis key 前缀为 DefaultAntiDoubleClickLockKeyPrefix，默认不带 principal scope，
// 因而保持历史 key 形状：prefix:authID:METHOD:/path。混用 admin/user/agent 等身份域时，
// 调用方应该显式传 WithAntiDoubleClickPrincipalScope，避免相同 AuthId 互相挡请求。
func NewAntiDoubleClickMiddleware(redis bizredis.RedisScripter, lockTTLSeconds int, opts ...AntiDoubleClickOption) *AntiDoubleClickMiddleware {
	if lockTTLSeconds <= 0 {
		lockTTLSeconds = defaultAntiDoubleClickLockTTLSeconds
	}
	config := antiDoubleClickConfig{
		keyPrefix: DefaultAntiDoubleClickLockKeyPrefix,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&config)
		}
	}

	return &AntiDoubleClickMiddleware{
		redis:          redis,
		lockTTLSeconds: lockTTLSeconds,
		keyPrefix:      config.keyPrefix,
		principalScope: config.principalScope,
	}
}

func (m *AntiDoubleClickMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authID, ok := authenticatedRequestID(r.Context())
		if !ok {
			writeAntiDoubleClickJSON(r.Context(), w, http.StatusUnauthorized, "invalid_auth_context", "invalid auth context")
			return
		}
		if m.redis == nil {
			logx.WithContext(r.Context()).Error("anti-double-click redis client is nil")
			next(w, r)
			return
		}

		routePath := cleanAntiDoubleClickPath(r.URL.Path)
		method := strings.ToUpper(strings.TrimSpace(r.Method))
		lock, err := acquireAntiDoubleClickLock(r.Context(), m.redis, m.keyPrefix, m.principalScope, authID, method, routePath, m.lockTTLSeconds)
		if errors.Is(err, ErrAntiDoubleClickInProgress) {
			writeAntiDoubleClickJSON(r.Context(), w, http.StatusTooManyRequests, "duplicate_request", "duplicate request, retry later")
			return
		}
		if err != nil {
			logx.WithContext(r.Context()).Errorf("anti-double-click acquire failed for %s %s: %v", method, routePath, err)
			next(w, r)
			return
		}
		defer func() {
			released, err := lock.ReleaseCtx(r.Context())
			if err != nil {
				logx.WithContext(r.Context()).Errorf("anti-double-click release failed for %s %s: %v", method, routePath, err)
				return
			}
			if !released {
				logx.WithContext(r.Context()).Errorf("anti-double-click lock was not released for %s %s", method, routePath)
			}
		}()

		next(w, r)
	}
}

func buildAntiDoubleClickLockKey(keyPrefix, principalScope string, authID uint64, method, routePath string) (string, error) {
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

func acquireAntiDoubleClickLock(ctx context.Context, redis bizredis.RedisScripter, keyPrefix, principalScope string, authID uint64, method, routePath string, ttlSeconds int) (*bizredis.RedisLock, error) {
	key, err := buildAntiDoubleClickLockKey(keyPrefix, principalScope, authID, method, routePath)
	if err != nil {
		return nil, err
	}
	if ttlSeconds <= 0 {
		ttlSeconds = defaultAntiDoubleClickLockTTLSeconds
	}

	lock := bizredis.NewRedisLock(redis, key)
	lock.SetExpire(ttlSeconds)
	locked, err := lock.AcquireCtx(ctx)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrAntiDoubleClickInProgress
	}
	return lock, nil
}

func authenticatedRequestID(ctx context.Context) (authID uint64, ok bool) {
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

func cleanAntiDoubleClickPath(routePath string) string {
	routePath = strings.TrimSpace(routePath)
	if routePath == "" {
		return "/"
	}
	if !strings.HasPrefix(routePath, "/") {
		routePath = "/" + routePath
	}
	return path.Clean(routePath)
}

func writeAntiDoubleClickJSON(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	httpx.WriteJsonCtx(ctx, w, status, map[string]string{
		"code":    code,
		"message": message,
	})
}
