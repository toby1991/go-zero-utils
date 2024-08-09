package bizredis

import (
	"context"
	"github.com/go-redis/redis/v8"
	"github.com/toby1991/go-zero-utils/cacher"
	"github.com/zeromicro/go-zero/core/breaker"
	"github.com/zeromicro/go-zero/core/logx"
	"strconv"
	"time"
)

type redisClient struct {
	_conf  BizRedisConf
	client *redis.Client
	prefix string
	brk    breaker.Breaker
}

func (c *redisClient) Client() *redis.Client {
	return c.client
}

func (c *redisClient) Prefix() string {
	return c.prefix
}
func (c *redisClient) Has(key string) bool {
	k := cacher.NewKey(key, c.Prefix())

	exists, err := c.client.Exists(context.Background(), k.Prefixed()).Result()
	if err != nil {
		logx.Error(err)
		return false
	}
	if exists <= 0 {
		return false
	}

	return true
}
func (c *redisClient) Get(key string, defaultValue ...interface{}) interface{} {
	k := cacher.NewKey(key, c.Prefix())

	if !c.Has(k.Raw()) {
		// @todo Event CacheMissed
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return nil
	}
	valStr, err := c.client.Get(context.Background(), k.Prefixed()).Result()
	if err != nil {
		logx.Error(err)
		return err
	}

	// @todo Event CacheHit
	return valStr
}
func (c *redisClient) Pull(key string, defaultValue ...interface{}) interface{} {
	k := cacher.NewKey(key, c.Prefix())

	val := c.Get(k.Raw(), defaultValue...)
	if val == nil {
		return nil
	}

	c.Forget(k.Raw())

	return val
}
func (c *redisClient) Put(key string, value interface{}, future time.Time) bool {
	k := cacher.NewKey(key, c.Prefix())

	_, err := c.client.Set(context.Background(), k.Prefixed(), value, cacher.DurationFromNow(future)).Result()
	if err != nil {
		logx.Error(err)
		return false
	}

	return true

	// @todo Event KeyWritten
}
func (c *redisClient) Add(key string, value interface{}, future time.Time) bool {
	k := cacher.NewKey(key, c.Prefix())

	// if expired return false
	ttl, err := c.client.TTL(context.Background(), k.Prefixed()).Result()
	if err != nil {
		logx.Error(err)
		return false
	}
	if ttl > 0 {
		return false
	}

	// if exists return false
	if c.Has(k.Raw()) {
		return false
	}

	result := c.Put(k.Raw(), value, future)

	// @todo Event KeyWritten
	return result
}
func (c *redisClient) Increment(key string, value int64) (incremented int64, success bool) {
	k := cacher.NewKey(key, c.Prefix())

	incremented, err := c.client.IncrBy(context.Background(), k.Prefixed(), value).Result()
	if err != nil {
		logx.Error(err)
		return 0, false
	}

	return incremented, true
}
func (c *redisClient) Decrement(key string, value int64) (decremented int64, success bool) {
	k := cacher.NewKey(key, c.Prefix())

	decremented, err := c.client.DecrBy(context.Background(), k.Prefixed(), value).Result()
	if err != nil {
		logx.Error(err)
		return 0, false
	}

	return decremented, true
}
func (c *redisClient) Forever(key string, value interface{}) bool {
	k := cacher.NewKey(key, c.Prefix())

	_, err := c.client.Set(context.Background(), k.Prefixed(), value, 0).Result()
	if err != nil {
		logx.Error(err)
		return false
	}

	// @todo Event KeyWritten
	return true
}
func (c *redisClient) Forget(key string) bool {
	k := cacher.NewKey(key, c.Prefix())

	result, err := c.client.Del(context.Background(), k.Prefixed()).Result()
	if err != nil {
		logx.Error(err)
		return false
	}
	if result <= 0 {
		return false
	}

	// @todo Event KeyForget
	return true
}
func (c *redisClient) Close() error {
	return c.client.Close()
}

func NewRedis(conf BizRedisConf) *redisClient {
	return &redisClient{
		_conf: conf,
		client: redis.NewClient(&redis.Options{
			Addr:     conf.Host + ":" + strconv.FormatInt(int64(conf.Port), 10),
			Password: conf.Password,
			DB:       conf.Db,
		}),
		prefix: conf.Prefix,
		brk:    breaker.NewBreaker(),
	}
}
