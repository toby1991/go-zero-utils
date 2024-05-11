package nsq

// docker exec -it faktory_container_name redis-cli -s /var/lib/faktory/db/redis.sock
type NsqConf struct {
	Sender SenderConf
	Worker WorkerConf
}

type SenderConf struct {
	NsqdAddrs []string // []string{"127.0.0.1:4150"}
}

type WorkerConf struct {
	NsqLookupdAddrs []string // []string{"127.0.0.1:4160"}

	MaxInFlight int `json:",default=50"`

	PullFromQueuesWithPriority map[string]int `json:",default={\"default\":1}"` // {"critical":3, "default":2, "bulk":1}
}
