package faktory

import (
	"context"
	faktory "github.com/contribsys/faktory/client"
	"github.com/zeromicro/go-zero/core/service"
)

type FaktoryClient interface {
	service.Service

	Context() context.Context
	SetProcessor(jobNameProcessorMap map[string]JobProcessor)
	Push(job *faktory.Job) error
}
