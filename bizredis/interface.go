package bizredis

import "github.com/go-redis/redis/v8"

type RedisClient interface {
	Client() *redis.Client
	BasicCacher
	RedisScripter
}
