package faktory

import (
	"context"
	"github.com/zeromicro/go-zero/core/logx"
	"os"
)
import faktory "github.com/contribsys/faktory/client"
import worker "github.com/contribsys/faktory_worker_go"

type JobProcessor func(helper worker.Helper, args ...interface{}) error

type faktoryClient struct {
	_conf               FaktoryConf
	senderPool          *faktory.Pool
	workerMgr           *worker.Manager
	jobNameProcessorMap map[string]JobProcessor
	ctx                 context.Context
	cancel              context.CancelFunc
}

func (c *faktoryClient) Start() {
	c.processing(context.Background(), c.jobNameProcessorMap)
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

func (c *faktoryClient) SetProcessor(jobNameProcessorMap map[string]JobProcessor) {
	c.jobNameProcessorMap = jobNameProcessorMap
}
func (c *faktoryClient) Context() context.Context {
	return c.ctx
}

// https://github.com/contribsys/faktory_worker_go#usage
func (c *faktoryClient) processing(ctx context.Context, jobNameProcessorMap map[string]JobProcessor) {
	c.ctx, c.cancel = context.WithCancel(ctx)

	go func() {
		// Start processing jobs in background routine, this method does not return
		// unless an error is returned or cancel() is called
		c.workerMgr.RunWithContext(c.ctx)
	}()

	// register processor
	for jobName, processor := range jobNameProcessorMap {
		// register job processor one by one
		newProcessor := processor
		c.workerMgr.Register(
			jobName,
			func(ctx context.Context, args ...interface{}) error {
				help := worker.HelperFor(ctx)
				logx.Infof("Working on job %s\n", help.Jid())
				return newProcessor(help, args...) // success then return nil as error, it will auto ack
			},
		)
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

func (c *faktoryClient) Push(job *faktory.Job) error {
	return c.senderPool.With(func(cl *faktory.Client) error {
		// job := faktory.NewJob("SomeJob", 1, 2, 3)
		return cl.Push(job)
	})
}
