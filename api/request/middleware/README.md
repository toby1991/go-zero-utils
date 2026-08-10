# HTTP 请求中间件

## `ThrottleMiddleware`

`ThrottleMiddleware` 是 Laravel 风格的 HTTP 请求固定窗口限流：认证成功后、进入业务
handler 前，每个请求消耗一次配额。它按 `AuthContext.AuthId`、HTTP method 和 cleaned
static URL path 计数；query、body 和下游实际 provider 的处理结果都不是 key 维度，也不会
回滚已消耗的配额。

Lua 脚本会原子执行 `INCR`，且只在计数首次变为 `1` 时调用 `EXPIRE`。后续请求绝不能续期，
否则固定窗口会悄悄变成滑动窗口。

必须将它安装在写入 `AuthContext` 的认证 middleware 之后。缺少认证上下文时返回 `401`，
超过窗口配额时返回 `429`。Redis 故障默认 fail-open，保持与 `AntiDoubleClickMiddleware`
一致；成本或安全边界要求严格限流时，请显式使用 `WithThrottleFailClosed()`，此时 Redis
故障返回 `503`。

`AuthContext.AuthId` 只包含数字。若 user、admin 或 agent 等身份域可能拥有相同 ID，请使用
`WithThrottlePrincipalScope`；不同独立限流器还可通过 `WithThrottleKeyPrefix` 使用互不冲突的
Redis namespace。

```go
// TokenEx：同一 tokenex-user 对同一路径每天最多进入 handler 3 次。
throttle := middleware.NewThrottleMiddleware(
	redis,
	3,
	86400,
	middleware.WithThrottlePrincipalScope("tokenex-user"),
	middleware.WithThrottleFailClosed(),
)

// 保证 AuthMiddleware 在前，然后再包业务 handler。
handler := throttle.Handle(next)
```
