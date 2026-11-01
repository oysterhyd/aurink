package mock

import "sync"

// Immediate is a contract.Dispatcher that runs each function at once.
type Immediate struct{}

// Post runs fn.
func (Immediate) Post(fn func()) { fn() }

// Queued is a contract.Dispatcher that holds functions until Flush, for tests
// of callback threads.
type Queued struct {
	mu    sync.Mutex
	queue []func()
}

// Post queues fn.
func (q *Queued) Post(fn func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = append(q.queue, fn)
}

// Flush runs what is queued, in order, including what those functions post.
func (q *Queued) Flush() {
	for {
		q.mu.Lock()
		if len(q.queue) == 0 {
			q.mu.Unlock()
			return
		}
		fn := q.queue[0]
		q.queue = q.queue[1:]
		q.mu.Unlock()
		fn()
	}
}
