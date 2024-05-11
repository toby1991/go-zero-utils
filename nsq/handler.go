package nsq

import (
	"github.com/nsqio/go-nsq"
	"github.com/toby1991/go-zero-utils/queue"
	"github.com/zeromicro/go-zero/core/logx"
)

type messageHandler struct {
	processor queue.JobProcessor
	dlq       queue.Dlqer
}

// go/pkg/mod/github.com/nsqio/go-nsq@v1.1.0/consumer.go:1175
func (m *messageHandler) LogFailedMessage(message *nsq.Message) {
	help, err := HelperFor(message)
	if err != nil {
		logx.Error("dlq parse error: ", err)
		return
	}

	if err := m.dlq.RequeueDeadJob(help.Job()); err != nil {
		logx.Error("dlq error: ", err)
		return
	}
}

func newMessageHandler(processor queue.JobProcessor, dlq queue.Dlqer) *messageHandler {
	return &messageHandler{processor: processor, dlq: dlq}
}

func (m *messageHandler) HandleMessage(message *nsq.Message) error {
	help, err := HelperFor(message)
	if err != nil {
		return err
	}

	logx.Infof("Working on job %s\n", help.Jid())

	return m.processor(help, help.Job().Args...)
}
