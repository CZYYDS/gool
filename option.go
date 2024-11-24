package gool

import "time"

type Option func(*ThreadPool)

func WithMinWorkers(min int64) Option {
	return func(pool *ThreadPool) {
		if min >= pool.MaxWorkers {
			pool.minWorkers = 0
			return
		}
		pool.minWorkers = min
	}
}
func WithMaxLifeTime(maxLifeTime time.Duration) Option {
	return func(pool *ThreadPool) {
		pool.MaxLifeTime = maxLifeTime
	}
}
func WithIdleTimeout(idleTimeout time.Duration) Option {
	return func(pool *ThreadPool) {
		pool.MaxIdleTime = idleTimeout
	}
}
