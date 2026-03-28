package worker

// Queue is a thread-safe FIFO job queue.
type Queue struct {
	ch chan string
}

// NewQueue creates a Queue with the given buffer capacity.
func NewQueue(capacity int) *Queue {
	return &Queue{ch: make(chan string, capacity)}
}

// Enqueue adds a job ID to the queue (non-blocking; caller must not overflow).
func (q *Queue) Enqueue(id string) {
	q.ch <- id
}

// Chan returns the receive channel for workers to consume.
func (q *Queue) Chan() <-chan string {
	return q.ch
}
