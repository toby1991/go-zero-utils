package bizredis

import (
	"context"
	red "github.com/go-redis/redis/v8"
	"github.com/toby1991/go-zero-utils/cacher"
)

type RedisScripter interface {
	ScriptLoad(script string) (string, error)                                                          // ScriptLoad is the implementation of redis script load command.
	ScriptLoadCtx(ctx context.Context, script string) (string, error)                                  // ScriptLoadCtx is the implementation of redis script load command.
	ScriptRun(script *Script, keys []string, args ...any) (any, error)                                 // ScriptRun is the implementation of *redis.Script run command.
	ScriptRunCtx(ctx context.Context, script *Script, keys []string, args ...any) (val any, err error) // ScriptRunCtx is the implementation of *redis.Script run command.
}

type (

	// Script is an alias of redis.Script.
	Script = red.Script
)

// RedisNode interface represents a redis node.
type RedisNode interface {
	red.Cmdable
}

func (c *redisClient) getRedis() (RedisNode, error) {
	return c.client, nil
}

// NewScript returns a new Script instance.
func NewScript(script string) *Script {
	return red.NewScript(script)
}

// ScriptLoad is the implementation of redis script load command.
func (c *redisClient) ScriptLoad(script string) (string, error) {
	return c.ScriptLoadCtx(context.Background(), script)
}

// ScriptLoadCtx is the implementation of redis script load command.
func (c *redisClient) ScriptLoadCtx(ctx context.Context, script string) (string, error) {
	return c.client.ScriptLoad(ctx, script).Result()
}

// ScriptRun is the implementation of *redis.Script run command.
func (c *redisClient) ScriptRun(script *Script, keys []string, args ...any) (any, error) {
	for i, key := range keys {
		keys[i] = cacher.NewKey(key, c.Prefix()).Prefixed()
	}

	return c.ScriptRunCtx(context.Background(), script, keys, args...)
}

// ScriptRunCtx is the implementation of *redis.Script run command.
func (c *redisClient) ScriptRunCtx(ctx context.Context, script *Script, keys []string, args ...any) (val any, err error) {
	for i, key := range keys {
		keys[i] = cacher.NewKey(key, c.Prefix()).Prefixed()
	}

	err = c.brk.DoWithAcceptable(func() error {
		conn, err := c.getRedis()
		if err != nil {
			return err
		}

		val, err = script.Run(ctx, conn, keys, args...).Result()
		return err
	}, acceptable)
	return
}
func acceptable(err error) bool {
	return err == nil || err == red.Nil || err == context.Canceled
}
