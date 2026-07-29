package nsq

import (
	"testing"

	"github.com/toby1991/go-zero-utils/queue"
)

func TestNsqClientSetProcessor(t *testing.T) {
	processorMap := map[queue.Event]queue.ListenerHandlerMap{
		"goods.updated": {
			"refresh-cache": func(queue.Helper, ...interface{}) error {
				return nil
			},
		},
	}

	client := &nsqClient{}
	client.SetProcessor(processorMap)

	if len(client.eventListenerHandlerMap) != 1 {
		t.Fatalf("processor event count = %d, want 1", len(client.eventListenerHandlerMap))
	}
	if _, ok := client.eventListenerHandlerMap["goods.updated"]["refresh-cache"]; !ok {
		t.Fatal("processor map does not contain goods.updated/refresh-cache")
	}
}

func TestNsqClientPushRejectsInvalidSchedule(t *testing.T) {
	client := &nsqClient{}
	job := queue.NewJob("goods.updated", "refresh-cache")
	job.At = "not-a-rfc3339-timestamp"

	err := client.Push(job)

	if err == nil {
		t.Fatal("Push() error = nil, want invalid schedule error")
	}
}
