package scheduler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/toby1991/go-zero-utils/bizredis"
)

func TestJobRunsWithEveryScheduleAndRedisLock(t *testing.T) {
	redis := &fakeRedisScripter{}
	ran := make(chan struct{}, 1)
	svc := New(
		"account.test",
		Every(5*time.Millisecond),
		redis,
		func(context.Context) error {
			select {
			case ran <- struct{}{}:
			default:
			}
			return nil
		},
		WithTimeout(time.Second),
	)

	started := startService(t, svc)
	defer stopService(t, svc, started)

	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("job did not run")
	}

	redis.mu.Lock()
	defer redis.mu.Unlock()
	if got, want := redis.acquireKeys[0], "scheduler:account.test"; got != want {
		t.Fatalf("lock key = %s, want %s", got, want)
	}
	if redis.releaseCount != 0 {
		t.Fatalf("release count = %d, want 0; scheduler locks must expire naturally", redis.releaseCount)
	}
	if !redis.locked {
		t.Fatal("redis lock was released; scheduler lock should stay held until ttl")
	}
}

func TestJobSkipsWhenRedisLockIsHeld(t *testing.T) {
	redis := &fakeRedisScripter{locked: true}
	ran := make(chan struct{}, 1)
	svc := New(
		"account.locked",
		Every(5*time.Millisecond),
		redis,
		func(context.Context) error {
			ran <- struct{}{}
			return nil
		},
		WithTimeout(time.Second),
	)

	started := startService(t, svc)
	defer stopService(t, svc, started)

	select {
	case <-ran:
		t.Fatal("job ran even though redis lock was held")
	case <-time.After(30 * time.Millisecond):
	}
}

func TestJobTimeoutCancelsHandlerContext(t *testing.T) {
	redis := &fakeRedisScripter{}
	cancelled := make(chan error, 1)
	svc := New(
		"account.timeout",
		Every(time.Millisecond),
		redis,
		func(ctx context.Context) error {
			<-ctx.Done()
			cancelled <- ctx.Err()
			return ctx.Err()
		},
		WithTimeout(5*time.Millisecond),
	)

	started := startService(t, svc)
	defer stopService(t, svc, started)

	select {
	case err := <-cancelled:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("handler context error = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handler context was not cancelled by timeout")
	}
}

func TestJobStopBeforeStartNoops(t *testing.T) {
	redis := &fakeRedisScripter{}
	svc := New(
		"account.stop",
		Every(time.Hour),
		redis,
		func(context.Context) error {
			t.Fatal("handler should not run")
			return nil
		},
	)
	svc.Stop()
}

func TestJobStartReturnsWhenAlreadyStarted(t *testing.T) {
	svc := New(
		"account.started",
		Every(time.Hour),
		&fakeRedisScripter{},
		func(context.Context) error { return nil },
	).(*jobService)
	svc.started.Store(true)

	done := make(chan struct{})
	go func() {
		svc.Start()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return when service was already started")
	}
}

func TestJobStartReturnsWhenSchedulerSetupFails(t *testing.T) {
	svc := &jobService{
		job: job{
			name: "account.bad_schedule",
			schedule: schedule{
				jobDefinition: gocron.DurationJob(0),
				lockDuration:  time.Hour,
			},
			redis:   &fakeRedisScripter{},
			handler: func(context.Context) error { return nil },
			lockTTL: time.Hour,
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	svc.Start()

	select {
	case <-svc.done:
	default:
		t.Fatal("Start did not close done after scheduler setup failure")
	}
}

func TestJobStopReturnsWhenShutdownTimesOut(t *testing.T) {
	startedHandler := make(chan struct{})
	unblockHandler := make(chan struct{})
	svc := New(
		"account.shutdown_timeout",
		Every(time.Millisecond),
		&fakeRedisScripter{},
		func(context.Context) error {
			select {
			case startedHandler <- struct{}{}:
			default:
			}
			<-unblockHandler
			return nil
		},
		WithStopTimeout(time.Millisecond),
	)

	started := startService(t, svc)
	defer func() {
		close(unblockHandler)
		<-started
	}()

	select {
	case <-startedHandler:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	stopped := make(chan struct{})
	go func() {
		svc.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after shutdown timeout")
	}
}

func TestNewAppliesOptionalSettings(t *testing.T) {
	redis := &fakeRedisScripter{}
	location := time.FixedZone("account-test", 8*60*60)
	stopTimeout := 123 * time.Millisecond

	svc := New(
		"account.options",
		DailyAt(0, 10, 0),
		redis,
		func(context.Context) error { return nil },
		WithLocation(location),
		WithStopTimeout(stopTimeout),
	)

	jobSvc := svc.(*jobService)
	if jobSvc.job.location != location {
		t.Fatalf("location = %v, want %v", jobSvc.job.location, location)
	}
	if jobSvc.job.StopTimeout != stopTimeout {
		t.Fatalf("stop timeout = %v, want %v", jobSvc.job.StopTimeout, stopTimeout)
	}
	if _, err := jobSvc.newScheduler(); err != nil {
		t.Fatalf("new scheduler with explicit location: %v", err)
	}
}

func TestRunLockedReturnsAcquireError(t *testing.T) {
	redisErr := errors.New("redis unavailable")
	svc := New(
		"account.acquire_error",
		Every(time.Hour),
		&fakeRedisScripter{acquireErr: redisErr},
		func(context.Context) error {
			t.Fatal("handler should not run when lock acquire fails")
			return nil
		},
	)

	err := svc.(*jobService).runLocked(context.Background())
	if !errors.Is(err, redisErr) {
		t.Fatalf("runLocked error = %v, want redis error", err)
	}
	if !strings.Contains(err.Error(), "scheduler:account.acquire_error") {
		t.Fatalf("runLocked error = %q, want lock key", err.Error())
	}
}

func TestRunLockedConvertsHandlerPanicToError(t *testing.T) {
	svc := New(
		"account.panic",
		Every(time.Hour),
		&fakeRedisScripter{},
		func(context.Context) error {
			panic("boom")
		},
	)

	err := svc.(*jobService).runLocked(context.Background())
	if err == nil {
		t.Fatal("runLocked returned nil, want panic error")
	}
	if !strings.Contains(err.Error(), "scheduler job panic: boom") {
		t.Fatalf("runLocked error = %q, want panic message", err.Error())
	}
}

func TestRunOnceAcceptsNilParentContext(t *testing.T) {
	ran := make(chan struct{}, 1)
	svc := New(
		"account.nil_parent",
		Every(time.Hour),
		&fakeRedisScripter{},
		func(ctx context.Context) error {
			if ctx == nil {
				t.Fatal("handler context is nil")
			}
			ran <- struct{}{}
			return nil
		},
	)

	svc.(*jobService).runOnce(nil)

	select {
	case <-ran:
	default:
		t.Fatal("handler did not run")
	}
}

func TestSafeRunReturnsHandlerError(t *testing.T) {
	handlerErr := errors.New("handler failed")
	svc := New(
		"account.handler_error",
		Every(time.Hour),
		&fakeRedisScripter{},
		func(context.Context) error {
			return handlerErr
		},
	)

	err := svc.(*jobService).safeRun(context.Background())
	if !errors.Is(err, handlerErr) {
		t.Fatalf("safeRun error = %v, want handler error", err)
	}
}

func TestNewPanicsWhenRequiredArgsMissing(t *testing.T) {
	redis := &fakeRedisScripter{}
	tests := []struct {
		name string
		run  func()
	}{
		{
			name: "name",
			run: func() {
				New("", Every(time.Hour), redis, func(context.Context) error { return nil })
			},
		},
		{
			name: "schedule",
			run: func() {
				New("account.missing_schedule", nil, redis, func(context.Context) error { return nil })
			},
		},
		{
			name: "redis",
			run: func() {
				New("account.missing_redis", Every(time.Hour), nil, func(context.Context) error { return nil })
			},
		},
		{
			name: "handler",
			run: func() {
				New("account.missing_handler", Every(time.Hour), redis, nil)
			},
		},
		{
			name: "lock ttl",
			run: func() {
				validateJob(job{
					name:     "account.zero_ttl",
					schedule: Every(time.Hour),
					redis:    redis,
					handler:  func(context.Context) error { return nil },
					lockTTL:  0,
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered == nil {
					t.Fatal("New did not panic")
				}
			}()
			tt.run()
		})
	}
}

func TestEveryUsesIntervalLockTTL(t *testing.T) {
	if got, want := Every(5*time.Minute).lockWindow(), 5*time.Minute; got != want {
		t.Fatalf("every lock ttl = %v, want %v", got, want)
	}
}

func TestDailyAtUsesBoundaryLockTTL(t *testing.T) {
	if got, want := DailyAt(0, 10, 0).lockWindow(), 24*time.Hour-boundaryLockDrift; got != want {
		t.Fatalf("daily lock ttl = %v, want %v", got, want)
	}
}

func TestWeeklyAtUsesBoundaryLockTTL(t *testing.T) {
	if got, want := WeeklyAt(time.Monday, 0, 10, 0).lockWindow(), 7*24*time.Hour-boundaryLockDrift; got != want {
		t.Fatalf("weekly lock ttl = %v, want %v", got, want)
	}
}

func TestTTLSecondsRoundsUp(t *testing.T) {
	if got, want := ttlSeconds(1500*time.Millisecond), 2; got != want {
		t.Fatalf("ttl seconds = %d, want %d", got, want)
	}
	if got, want := ttlSeconds(0), 1; got != want {
		t.Fatalf("ttl seconds = %d, want %d", got, want)
	}
}

func startService(t *testing.T, svc interface{ Start() }) <-chan struct{} {
	t.Helper()
	started := make(chan struct{})
	go func() {
		close(started)
		svc.Start()
	}()
	<-started
	return started
}

func stopService(t *testing.T, svc interface{ Stop() }, started <-chan struct{}) {
	t.Helper()
	<-started
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("service did not stop")
	}
}

type fakeRedisScripter struct {
	mu           sync.Mutex
	locked       bool
	acquireErr   error
	acquireKeys  []string
	releaseCount int
}

var _ bizredis.RedisScripter = (*fakeRedisScripter)(nil)

func (r *fakeRedisScripter) ScriptRun(script *bizredis.Script, keys []string, args ...any) (any, error) {
	return r.ScriptRunCtx(context.Background(), script, keys, args...)
}

func (r *fakeRedisScripter) ScriptRunCtx(_ context.Context, _ *bizredis.Script, keys []string, args ...any) (any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(keys) != 1 {
		return nil, errors.New("unexpected key count")
	}
	values, _ := args[0].([]string)
	if len(values) == 2 {
		r.acquireKeys = append(r.acquireKeys, keys[0])
		if r.acquireErr != nil {
			return nil, r.acquireErr
		}
		if r.locked {
			return nil, nil
		}
		r.locked = true
		return "OK", nil
	}
	if len(values) == 1 {
		r.locked = false
		r.releaseCount++
		return int64(1), nil
	}
	return nil, errors.New("unexpected redis script args")
}

func (r *fakeRedisScripter) ScriptLoadCtx(context.Context, string) (string, error) {
	return "", nil
}

func (r *fakeRedisScripter) ScriptLoad(string) (string, error) {
	return "", nil
}
