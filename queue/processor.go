package queue

type JobProcessor func(helper Helper, args ...interface{}) error
