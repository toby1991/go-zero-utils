package faktory

import (
	"context"
	"github.com/toby1991/go-zero-utils/queue"
	"github.com/zeromicro/go-zero/core/service"
)

type FaktoryClient interface {
	service.Service

	Context() context.Context
	SetProcessor(jobNameProcessorMap map[string]queue.JobProcessor)
	Push(job *queue.Job) error
}
