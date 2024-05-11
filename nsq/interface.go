package nsq

import (
	"context"
	"github.com/toby1991/go-zero-utils/queue"
	"github.com/zeromicro/go-zero/core/service"
)

type NsqClient interface {
	service.Service

	Context() context.Context
	SetProcessor(jobTopicChannelMapWithProcessor map[Topic]ChannelProcessorMap)
	Push(job *queue.Job) error
}
