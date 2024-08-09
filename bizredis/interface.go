package bizredis

import (
	"github.com/go-redis/redis/v8"
	"github.com/toby1991/go-zero-utils/cacher"
)

type RedisClient interface {
	Client() *redis.Client
	cacher.BasicCacher
	RedisScripter
}
