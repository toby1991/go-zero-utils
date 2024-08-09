package cacher

import "time"

func DurationFromNow(future time.Time) time.Duration {
	return future.Sub(time.Now())
}
