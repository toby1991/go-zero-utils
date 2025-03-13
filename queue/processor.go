package queue

type ListenerHandler func(helper Helper, args ...interface{}) error

type Event = string
type Listener = string
type ListenerHandlerMap = map[Listener]ListenerHandler
