package faktory

// docker exec -it faktory_container_name redis-cli -s /var/lib/faktory/db/redis.sock
type FaktoryConf struct {
	Url    string // tcp://:mypassword@faktory.example.com:7419
	Sender SenderConf
	Worker WorkerConf
}

type SenderConf struct {
	PoolCapacity int `json:",default=1"`
}

type WorkerConf struct {
	Concurrency                int            `json:",default=20"`              // worker pool = concurrency + 2 github.com/contribsys/faktory_worker_go@v1.6.0/manager.go:137
	PullFromQueuesWithPriority map[string]int `json:",default={\"default\":1}"` // {"critical":3, "default":2, "bulk":1}
}
