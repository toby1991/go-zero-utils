package nsq

import (
	"github.com/nsqio/go-nsq"
	"github.com/toby1991/go-zero-utils/queue"
)

type ConsumerPool struct {
	nsqLookupdHttpAddresses []string
	conf                    *nsq.Config
	consumers               []*nsq.Consumer
}

func newConsumerPool(nsqLookupdHttpAddresses []string, conf *nsq.Config) *ConsumerPool {
	return &ConsumerPool{
		nsqLookupdHttpAddresses: nsqLookupdHttpAddresses,
		conf:                    conf,
		consumers:               make([]*nsq.Consumer, 0),
	}
}

func (c *ConsumerPool) RegisterProcessor(topic string, channel string, processor queue.JobProcessor, concurrency int, dlq queue.Dlqer) error {
	consumer, err := nsq.NewConsumer(topic, channel, c.conf)
	if err != nil {
		return err
	}

	// Set the Handler for messages received by this Consumer. Can be called multiple times.
	// See also AddConcurrentHandlers.
	if concurrency <= 0 {
		concurrency = 1
	}
	consumer.AddConcurrentHandlers(newMessageHandler(processor, dlq), concurrency)

	// Use nsqlookupd to discover nsqd instances.
	// See also ConnectToNSQD, ConnectToNSQDs, ConnectToNSQLookupds.
	if err := consumer.ConnectToNSQLookupds(c.nsqLookupdHttpAddresses); err != nil {
		return err
	}

	c.consumers = append(c.consumers, consumer)

	return nil
}

func (c *ConsumerPool) Stop() {
	for _, consumer := range c.consumers {
		consumer.Stop()
	}
}
