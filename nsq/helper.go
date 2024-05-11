package nsq

import (
	"encoding/json"
	faktory "github.com/contribsys/faktory/client"
	"github.com/nsqio/go-nsq"
	"github.com/toby1991/go-zero-utils/queue"
	"time"
)

type helper struct {
	message *nsq.Message
	job     *queue.Job
}

func HelperFor(message *nsq.Message) (*helper, error) {

	var job queue.Job
	if err := json.Unmarshal(message.Body, &job); err != nil {
		return nil, err
	}

	return &helper{message: message, job: &job}, nil
}

func (h *helper) Job() *queue.Job {
	return h.job
}

func (h *helper) Jid() string {
	//return string(hash.Md5(h.message.ID[:]))
	return string(h.message.ID[:])
}

// Channel = job.Type
func (h *helper) JobType() string {
	return h.Job().Type
}

func (h *helper) Custom(key string) (value interface{}, ok bool) {
	val, ok := h.Job().Custom[key]
	return val, ok
}

func (h *helper) Bid() string {
	//TODO implement me
	return string(h.message.ID[:])
}

func (h *helper) CallbackBid() string {
	//TODO implement me
	panic("implement me")
}

func (h *helper) Batch(f func(*faktory.Batch) error) error {
	//TODO implement me
	panic("implement me")
}

func (h *helper) With(f func(*faktory.Client) error) error {
	//TODO implement me
	panic("implement me")
}

func (h *helper) TrackProgress(percent int, desc string, reserveUntil *time.Time) error {
	//TODO implement me
	panic("implement me")
}
