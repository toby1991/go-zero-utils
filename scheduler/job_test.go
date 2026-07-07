package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
