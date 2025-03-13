package queue

import (
	"context"
	"github.com/zeromicro/go-zero/core/service"
)

type Client interface {
	service.Service

	Context() context.Context
	SetProcessor(eventListenerHandlerMap map[Event]ListenerHandlerMap)
	Push(job *Job) error
}
