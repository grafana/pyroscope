package ratelimit

import "time"

// Limiter implements a simple token-bucket rate limiter.
// Tokens are replenished over time.
type Limiter struct {
	rate    float64
	tokens  float64
	updated time.Time
	// For testing purposes.
	sleep func(time.Duration)
	now   func() time.Time
}

func NewLimiter(rate float64) *Limiter {
	return &Limiter{
		rate:   rate,
		tokens: rate,
		sleep:  time.Sleep,
		now:    time.Now,
	}
}

func (l *Limiter) Wait(n int) {
	for {
		now := l.now()
		elapsed := now.Sub(l.updated).Seconds()
		l.updated = now
		l.tokens += elapsed * l.rate
		if l.tokens > l.rate {
			l.tokens = l.rate
		}
		if l.tokens >= float64(n) {
			l.tokens -= float64(n)
			return
		}
		missing := float64(n) - l.tokens
		delay := time.Duration(missing/l.rate*1e9) * time.Nanosecond
		l.sleep(delay)
	}
}
