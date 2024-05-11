package bizredis

type BizRedisConf struct {
	Host     string
	Port     int
	Password string `json:",optional"`
	Db       int
	Prefix   string `json:",optional"`
}
