package utils

import "sync"

type DynamicLimiter struct {
	mu          sync.Mutex
	maxParallel int
	active      int
	waiting     int
	cond        *sync.Cond
}

func NewDynamicLimiter(limit int) *DynamicLimiter {
	dl := &DynamicLimiter{maxParallel: limit}
	dl.cond = sync.NewCond(&dl.mu)
	return dl
}

// Acquire reserves a slot AND enforces the limit change immediately.
func (d *DynamicLimiter) Acquire() {
	d.mu.Lock()
	for d.active >= d.maxParallel {
		d.cond.Wait()
	}
	d.active++
	d.mu.Unlock()
}

// Release frees a slot and wakes waiters.
func (d *DynamicLimiter) Release() {
	d.mu.Lock()
	d.active--
	d.cond.Broadcast() // wake all to re-check
	d.mu.Unlock()
}

// SetLimit changes the limit and forces extra active workers to pause if needed.
func (d *DynamicLimiter) SetLimit(limit int) {
	d.mu.Lock()
	d.maxParallel = limit
	d.cond.Broadcast() // wake everyone to re-check
	d.mu.Unlock()
}

func (d *DynamicLimiter) Wait() {
	d.mu.Lock()
	for d.active > 0 {
		d.cond.Wait()
	}
	d.mu.Unlock()
}

// WaitIfOverLimit makes already-acquired goroutines pause if over the limit.
// Uses a waiting counter to prevent deadlock by ensuring at least one thread can proceed.
func (d *DynamicLimiter) WaitIfOverLimit() {
	d.mu.Lock()
	d.waiting++

	for (d.active - (d.waiting - 1)) > d.maxParallel {
		d.cond.Wait()
	}

	d.waiting--
	d.mu.Unlock()
}
