package bizmemory

import (
	"github.com/toby1991/go-zero-utils/cacher"
)

type MemoryClient interface {
	cacher.BasicCacher
}
