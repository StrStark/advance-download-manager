package core

import (
	"context"
	"sync"
	"time"
)

// Limiter is a token bucket whose rate can be changed while in use.
// A rate of 0 means unlimited.
type Limiter struct {
	mu     sync.Mutex
	rate   float64 // bytes per second
	tokens float64
	last   time.Time
}

func NewLimiter(bytesPerSec int64) *Limiter {
	return &Limiter{rate: float64(bytesPerSec), last: time.Now()}
}

func (l *Limiter) SetRate(bytesPerSec int64) {
	l.mu.Lock()
	l.rate = float64(bytesPerSec)
	l.tokens = 0
	l.last = time.Now()
	l.mu.Unlock()
}

// WaitN blocks until n bytes may be transferred.
func (l *Limiter) WaitN(ctx context.Context, n int) error {
	l.mu.Lock()
	if l.rate <= 0 {
		l.mu.Unlock()
		return nil
	}
	t := time.Now()
	l.tokens += t.Sub(l.last).Seconds() * l.rate
	l.last = t
	// Allow at most ~200ms of burst so limits feel responsive.
	if burst := l.rate / 5; l.tokens > burst {
		l.tokens = burst
	}
	l.tokens -= float64(n)
	wait := time.Duration(0)
	if l.tokens < 0 {
		wait = time.Duration(-l.tokens / l.rate * float64(time.Second))
	}
	l.mu.Unlock()
	if wait == 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
