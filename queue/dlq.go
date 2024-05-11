package queue

type Dlqer interface {
	RequeueDeadJob(job *Job) error
}
