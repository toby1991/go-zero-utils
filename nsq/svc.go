package nsq

import (
	"context"
	"github.com/nsqio/go-nsq"
	"github.com/toby1991/go-zero-utils/queue"
	"time"
)

type Topic = string
type Channel = string
type ChannelProcessorMap = map[Channel]queue.JobProcessor
type nsqClient struct {
	_conf      NsqConf
	senderPool *ProducerPool
	workerPool *ConsumerPool

	jobTopicChannelMapWithProcessor map[Topic]ChannelProcessorMap // map[string]queue.JobProcessor
	ctx                             context.Context
	cancel                          context.CancelFunc
}

func NewNsq(conf NsqConf) *nsqClient {
	_nsqClient := &nsqClient{
		_conf: conf,
	}

	// config
	_conf := nsq.NewConfig()
	_conf.MaxInFlight = conf.Worker.MaxInFlight

	// producer
	var err error
	if _nsqClient.senderPool, err = newProducerPool(conf.Sender.NsqdAddrs, _conf); err != nil {
		panic(err)
	}

	// consumer
	_nsqClient.workerPool = newConsumerPool(conf.Worker.NsqLookupdAddrs, _conf)

	return _nsqClient
}

func (c *nsqClient) SetProcessor(jobTopicChannelMapWithProcessor map[Topic]ChannelProcessorMap) {
	c.jobTopicChannelMapWithProcessor = jobTopicChannelMapWithProcessor
}
func (c *nsqClient) Context() context.Context {
	return c.ctx
}
func (c *nsqClient) Start() {
	c.processing(context.Background(), c.jobTopicChannelMapWithProcessor)
}
func (c *nsqClient) Stop() {
	c.senderPool.Stop()
	c.workerPool.Stop()
	c.cancel()
}
func (c *nsqClient) Push(job *queue.Job) error {
	// Topic = job.Queue
	// Channel = job.Type
	// delay = job.At // 可能不准，会比设定时间多一点

	delay := time.Duration(0)
	if len(job.At) > 0 {
		//	job.At is time.RFC3339Nano string
		jobAt, err := time.Parse(time.RFC3339Nano, job.At)
		if err != nil {
			return err
		}
		delay = jobAt.Sub(time.Now())
	}

	jobJsonBytes, err := job.JsonBytes()
	if err != nil {
		return err
	}
	return c.senderPool.Publish(job.Queue, delay, jobJsonBytes)
}

func (c *nsqClient) processing(ctx context.Context, jobTopicChannelMapWithProcessor map[Topic]ChannelProcessorMap) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	// dlq
	_dlq := newDlq(c.senderPool)

	// register processor
	for topic, channelMapWithProcessor := range jobTopicChannelMapWithProcessor {
		for channel, processor := range channelMapWithProcessor {

			// register job processor one by one
			newProcessor := processor

			// Topic = job.Queue
			// Channel = job.Type
			// delay = job.At // 可能不准，会比设定时间多一点
			concurrency, ok := c._conf.Worker.PullFromQueuesWithPriority[channel]
			if !ok {
				concurrency = 1
			}
			if err := c.workerPool.RegisterProcessor(topic, channel, newProcessor, concurrency, _dlq); err != nil {
				panic(err)
			}

			// topic 同时注册到 global-dlq, concurrency 为 1
			if err := c.workerPool.RegisterProcessor(topic+TOPIC_DLQ_SUFFIX, channel, newProcessor, 1, _dlq); err != nil {
				panic(err)
			}
		}

	}
}
