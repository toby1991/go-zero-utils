package nsq

import (
	"fmt"
	"github.com/toby1991/go-zero-utils/queue"
	"github.com/zeromicro/go-zero/core/logx"
)

const TOPIC_DLQ_SUFFIX = "-dlq"

type dlq struct {
	producerPool *ProducerPool
}

func newDlq(producerPool *ProducerPool) *dlq {
	return &dlq{producerPool: producerPool}
}

func (d *dlq) RequeueDeadJob(job *queue.Job) error {
	jobJsonBytes, err := job.JsonBytes()
	if err != nil {
		return err
	}

	logx.Alert(fmt.Sprintf("go-zero-utils: RequeueDeadJob: %s", string(jobJsonBytes)))

	return d.producerPool.Publish(job.Queue+TOPIC_DLQ_SUFFIX, 0, jobJsonBytes)
}
