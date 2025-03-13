package nsq

import (
	"github.com/nsqio/go-nsq"
	"sync"
	"time"
)

type ProducerPool struct {
	nsqdAddresses []string
	conf          *nsq.Config
	producers     []*nsq.Producer
	sync.Mutex        // 互斥锁保护以下字段
	index         int // 当前使用的producer的索引
}

func newProducerPool(nsqdAddresses []string, conf *nsq.Config) (*ProducerPool, error) {
	var producers []*nsq.Producer
	for _, addr := range nsqdAddresses {
		producer, err := nsq.NewProducer(addr, conf)
		if err != nil {
			return nil, err
		}

		err = producer.Ping()
		if err != nil {
			producer.Stop()
			return nil, err
		}

		producers = append(producers, producer)
	}

	return &ProducerPool{
		nsqdAddresses: nsqdAddresses,
		conf:          conf,
		producers:     producers,
	}, nil
}

func (p *ProducerPool) Publish(topic string, delay time.Duration, message []byte) error {
	p.Lock()
	producer := p.producers[p.index]
	p.index = (p.index + 1) % len(p.producers)
	p.Unlock()

	if delay > 0 {
		return producer.DeferredPublish(topic, delay, message)
	}
	return producer.Publish(topic, message)
}

func (p *ProducerPool) Stop() {
	for _, producer := range p.producers {
		producer.Stop()
	}
}
