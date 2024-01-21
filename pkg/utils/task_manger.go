package utils

import (
	"sync"
)

// TaskManager manages tasks with concurrency control.
type TaskManager struct {
	maxConcurrent int
	semaphore     chan struct{}
	wg            sync.WaitGroup
}

// NewTaskManager creates a new TaskManager with a specified concurrency limit.
func NewTaskManager(maxConcurrent int) *TaskManager {
	return &TaskManager{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

// Add signals the beginning of a task. It should be called before the task starts.
func (tm *TaskManager) Add() {
	tm.wg.Add(1)
	tm.semaphore <- struct{}{} // Acquire a token (will block if limit is reached)
}

// Done signals the completion of a task. It should be called after the task ends.
func (tm *TaskManager) Done() {
	<-tm.semaphore // Release the token
	tm.wg.Done()
}

// Wait waits for all tasks to complete.
func (tm *TaskManager) Wait() {
	tm.wg.Wait()
}
