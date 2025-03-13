package faktory

import (
	"context"
	"github.com/jinzhu/copier"
	"github.com/toby1991/go-zero-utils/queue"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
)
import faktory "github.com/contribsys/faktory/client"
import worker "github.com/contribsys/faktory_worker_go"

type faktoryClient struct {
	_conf                   FaktoryConf
	senderPool              *faktory.Pool
	workerMgr               *worker.Manager
	eventListenerMap        map[string]queue.ListenerHandler
	eventListenerHandlerMap map[queue.Event]queue.ListenerHandlerMap
	ctx                     context.Context
	cancel                  context.CancelFunc
}

func (c *faktoryClient) Start() {
	c.processing(context.Background(), c.eventListenerHandlerMap)
}

func (c *faktoryClient) Stop() {
	c.cancel()
}

func NewFaktory(conf FaktoryConf) *faktoryClient {
	if len(conf.Url) <= 0 {
		return nil
	}

	// FAKTORY_PROVIDER=FOO_URL
	// FOO_URL=tcp://:mypassword@faktory.example.com:7419
	if err := os.Setenv("FAKTORY_PROVIDER", "FOO_URL"); err != nil {
		panic(err)
	}
	if err := os.Setenv("FOO_URL", conf.Url); err != nil {
		panic(err)
	}

	// pool
	pool, err := faktory.NewPool(conf.Sender.PoolCapacity)
	if err != nil {
		panic(err)
	}

	// worker manager
	workerMgr := worker.NewManager()
	workerMgr.Concurrency = conf.Worker.Concurrency
	workerMgr.ProcessWeightedPriorityQueues(conf.Worker.PullFromQueuesWithPriority)

	return &faktoryClient{
		_conf:      conf,
		senderPool: pool,
		workerMgr:  workerMgr,
	}
}

func (c *faktoryClient) SetProcessor(eventListenerHandlerMap map[queue.Event]queue.ListenerHandlerMap) {
	c.eventListenerHandlerMap = eventListenerHandlerMap
}
func (c *faktoryClient) Context() context.Context {
	return c.ctx
}

// https://github.com/contribsys/faktory_worker_go#usage
func (c *faktoryClient) processing(ctx context.Context, eventListenerHandlerMap map[queue.Event]queue.ListenerHandlerMap) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	go func() {
		// Start processing jobs in background routine, this method does not return
		// unless an error is returned or cancel() is called
		c.workerMgr.RunWithContext(c.ctx)
	}()

	// register processor
	for event, listenerHandlerMap := range eventListenerHandlerMap {
		for listener, processor := range listenerHandlerMap {

			jobType := ToJobType(event, listener)

			// register job processor one by one
			newProcessor := processor
			c.workerMgr.Register(
				jobType,
				func(ctx context.Context, args ...interface{}) error {
					help := worker.HelperFor(ctx)
					logx.Infof("Working on job %s\n", help.Jid())
					return newProcessor(help, args...) // success then return nil as error, it will auto ack
				},
			)
		}
	}
	//
	//go func() {
	//	stopSignals := []os.Signal{
	//		syscall.SIGTERM,
	//		syscall.SIGINT,
	//	}
	//	stop := make(chan os.Signal, len(stopSignals))
	//	for _, s := range stopSignals {
	//		signal.Notify(stop, s)
	//	}
	//
	//	for {
	//		select {
	//		case <-c.ctx.Done():
	//			return
	//		case <-stop:
	//			c.cancel()
	//		}
	//	}
	//}()
	//
	//<-c.ctx.Done()
}

func (c *faktoryClient) Push(job *queue.Job) error {
	return c.senderPool.With(func(cl *faktory.Client) error {
		// job := faktory.NewJob("SomeJob", 1, 2, 3)
		faktoryJob := faktory.NewJob("")
		err := copier.Copy(faktoryJob, job)
		if err != nil {
			return err
		}
		faktoryJob.Type = ToJobType(faktoryJob.Type, faktoryJob.Queue)
		return cl.Push(faktoryJob)
	})
}
