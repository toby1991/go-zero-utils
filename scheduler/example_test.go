package scheduler

import (
	"context"
	"time"
)

func ExampleNew_daily() {
	redis := &fakeRedisScripter{}

	service := New(
		"account.daily_reconciliation",
		DailyAt(0, 10, 0),
		redis,
		func(runCtx context.Context) error {
			// 这里只做触发适配：构造 proto request，然后调用 owner logic。
			// 真正的业务幂等和对账正确性必须留在 logic/DB 里。
			return nil
		},
		WithTimeout(30*time.Minute),
	)

	_ = service
}
