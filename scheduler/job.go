package scheduler

import (
	"context"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/toby1991/go-zero-utils/bizredis"
	"github.com/toby1991/go-zero/core/logx"
	"github.com/toby1991/go-zero/core/service"
)

const (
	defaultStopTimeout = 10 * time.Second
	lockKeyPrefix      = "scheduler"

	// boundaryLockDrift is used by boundary-expire schedules.
	// 规则：Redis 锁表示一个自然调度窗口，执行后不主动释放；锁窗口略短于
	// 周期，避免旧窗口残锁挡住下一次自然边界触发。
	// 解决的问题：多实例错峰启动、进程重启、handler 很快结束时，同一窗口
	// 被重复执行；同时保证下一天/下一周边界有时间重新抢锁。
	boundaryLockDrift = 10 * time.Minute
)

// Handler runs when the schedule fires and this instance acquires the Redis lock.
// 它只承载触发后的函数调用；业务判断、幂等、DB 最终正确性必须留在 owner logic 里。
type Handler func(context.Context) error

// Option changes non-required scheduler behavior.
type Option func(*job)

type job struct {
	name     string
	schedule Schedule
	location *time.Location

	handler Handler

	// lockTTL > 0 enables a Redis mutex at scheduler:<name>.
	// 这个锁只防止多实例同时跑同名任务；业务重复执行仍由 logic/DB 兜底。
	lockTTL time.Duration
	redis   bizredis.RedisScripter

	// timeout bounds one handler execution.
	Timeout     time.Duration
	StopTimeout time.Duration
}

// Schedule is the trigger definition accepted by New.
type Schedule interface {
	definition() gocron.JobDefinition
	lockWindow() time.Duration
}

type schedule struct {
	jobDefinition gocron.JobDefinition
	lockDuration  time.Duration
}

func (s schedule) definition() gocron.JobDefinition {
	return s.jobDefinition
}

func (s schedule) lockWindow() time.Duration {
	return s.lockDuration
}

// Every triggers a job at a fixed interval.
// Its Redis lock is held for the interval and then expires naturally.
func Every(interval time.Duration) Schedule {
	return schedule{
		jobDefinition: gocron.DurationJob(interval),
		lockDuration:  interval,
	}
}

// DailyAt triggers once per UTC day unless WithLocation overrides the scheduler location.
// It follows boundary-expire: the Redis lock gates the natural daily window,
// is not released after handler completion, and expires slightly before the
// next daily boundary so the next window can acquire the lock on time.
func DailyAt(hour, minute, second uint) Schedule {
	return schedule{
		jobDefinition: gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(hour, minute, second))),
		lockDuration:  24*time.Hour - boundaryLockDrift,
	}
}

// WeeklyAt triggers once per week on weekday at the given local time.
// The local time is UTC unless WithLocation is set. It follows the same
// boundary-expire rule as DailyAt, but for a weekly natural window.
func WeeklyAt(weekday time.Weekday, hour, minute, second uint) Schedule {
	return schedule{
		jobDefinition: gocron.WeeklyJob(
			1,
			gocron.NewWeekdays(weekday),
			gocron.NewAtTimes(gocron.NewAtTime(hour, minute, second)),
		),
		lockDuration: 7*24*time.Hour - boundaryLockDrift,
	}
}

// WithLocation sets the timezone used to interpret DailyAt and WeeklyAt.
func WithLocation(location *time.Location) Option {
	return func(job *job) {
		job.location = location
	}
}

// WithTimeout bounds one handler execution.
// Keep this shorter than the schedule lock window. The Redis lock is not
// released when the handler returns; it expires naturally at that window.
func WithTimeout(timeout time.Duration) Option {
	return func(job *job) {
		job.Timeout = timeout
	}
}

// WithStopTimeout bounds scheduler shutdown.
func WithStopTimeout(timeout time.Duration) Option {
	return func(job *job) {
		job.StopTimeout = timeout
	}
}

// New returns a go-zero service.Service for one scheduled job.
// Required arguments are positional so missing name/schedule/Redis/handler
// fail at compile time. The schedule owns the Redis lock window. The Redis lock
// key is scheduler:<name>, and the lock is intentionally left to expire by TTL
// instead of being released after handler completion. This prevents duplicate
// executions inside the same scheduling window when multiple service instances
// start at different times.
func New(name string, schedule Schedule, redis bizredis.RedisScripter, handler Handler, options ...Option) service.Service {
	job := job{
		name:     name,
		schedule: schedule,
		redis:    redis,
		handler:  handler,
	}
	if schedule != nil {
		job.lockTTL = schedule.lockWindow()
	}
	for _, option := range options {
		if option != nil {
			option(&job)
		}
	}
	validateJob(job)
	return &jobService{
		job:  job,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

type jobService struct {
	job job

	stop chan struct{}
	done chan struct{}

	started  atomic.Bool
	stopOnce sync.Once
}

func (s *jobService) Start() {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	defer close(s.done)

	cr, err := s.newScheduler()
	if err != nil {
		logx.Errorw("scheduler job setup failed", logx.Field("job", s.job.name), logx.Field("error", err))
		return
	}

	cr.Start()
	<-s.stop

	stopTimeout := s.job.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	if err := cr.ShutdownWithContext(ctx); err != nil {
		logx.Errorw("scheduler job shutdown failed", logx.Field("job", s.job.name), logx.Field("error", err))
	}
}

func (s *jobService) Stop() {
	if !s.started.Load() {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stop)
		<-s.done
	})
}

func (s *jobService) newScheduler() (gocron.Scheduler, error) {
	location := s.job.location
	if location == nil {
		location = time.UTC
	}
	cr, err := gocron.NewScheduler(gocron.WithLocation(location))
	if err != nil {
		return nil, err
	}
	_, err = cr.NewJob(
		s.job.schedule.definition(),
		gocron.NewTask(func(ctx context.Context) {
			s.runOnce(ctx)
		}),
		gocron.WithName(s.job.name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return nil, err
	}
	return cr, nil
}

func (s *jobService) runOnce(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	ctx := parent
	cancel := func() {}
	if s.job.Timeout > 0 {
		ctx, cancel = context.WithTimeout(parent, s.job.Timeout)
	}
	defer cancel()

	startedAt := time.Now()
	if err := s.runLocked(ctx); err != nil {
		logx.Errorw("scheduler job run failed",
			logx.Field("job", s.job.name),
			logx.Field("duration", time.Since(startedAt).String()),
			logx.Field("error", err),
		)
	}
}

func (s *jobService) runLocked(ctx context.Context) (err error) {
	lock := bizredis.NewRedisLock(s.job.redis, s.lockKey())
	lock.SetExpire(ttlSeconds(s.job.lockTTL))
	locked, err := lock.AcquireCtx(ctx)
	if err != nil {
		return fmt.Errorf("acquire scheduler lock %s: %w", s.lockKey(), err)
	}
	if !locked {
		return nil
	}

	// 不主动 Release。锁 TTL 表示本任务的执行窗口；让它自然过期可避免
	// 多实例在同一窗口内因为某个实例提前完成而重复执行。
	return s.safeRun(ctx)
}

func (s *jobService) safeRun(ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("scheduler job panic: %v\n%s", recovered, string(debug.Stack()))
		}
	}()
	return s.job.handler(ctx)
}

func (s *jobService) lockKey() string {
	return fmt.Sprintf("%s:%s", lockKeyPrefix, s.job.name)
}

func validateJob(job job) {
	if strings.TrimSpace(job.name) == "" {
		panic("scheduler job name is required")
	}
	if job.schedule == nil {
		panic("scheduler job schedule is required")
	}
	if job.handler == nil {
		panic("scheduler job handler is required")
	}
	if job.redis == nil {
		panic("scheduler job redis is required")
	}
	if job.lockTTL <= 0 {
		panic("scheduler job lock ttl must be greater than 0")
	}
}

func ttlSeconds(ttl time.Duration) int {
	seconds := int(math.Ceil(ttl.Seconds()))
	if seconds < 1 {
		return 1
	}
	return seconds
}
