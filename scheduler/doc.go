// Package scheduler adapts scheduled jobs into go-zero service.Service values.
//
// Usage in a service entry:
//
//	svcGroup.Add(scheduler.New(
//		"account.daily_reconciliation",
//		scheduler.DailyAt(0, 10, 0),
//		ctx.BizRedis,
//		func(runCtx context.Context) error {
//			_, err := logic.NewRunDailyReconciliationLogic(runCtx, ctx).RunDailyReconciliation(req)
//			return err
//		},
//		scheduler.WithTimeout(30*time.Minute),
//	))
//
// Required arguments are intentionally positional: name, schedule, Redis, and
// handler. If a caller forgets one of them, the compiler catches it instead of
// letting a half-built struct reach runtime.
//
// The Redis lock is held until TTL expiry. The wrapper does not release it when
// the handler returns. This follows the scheduler pattern used in cutleeks:
// lock TTL represents the execution window, not only the maximum handler
// runtime. This prevents multiple service instances, staggered startups, or
// quick handler completion from running the same logical window more than once.
//
// The schedule owns the lock TTL. Every follows interval-expire and uses its
// interval as the lock window. DailyAt and WeeklyAt follow boundary-expire: the
// lock window is slightly shorter than the natural day/week so stale lock state
// cannot block the next boundary.
//
// The handler should only bridge into the owner service logic. Business
// idempotency and final correctness must stay in the logic and database.
package scheduler
