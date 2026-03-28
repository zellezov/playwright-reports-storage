package worker

import (
	"sync"
)

// Pool manages a set of worker goroutines draining a Queue.
type Pool struct {
	queue     *Queue
	processor *Processor
	wg        sync.WaitGroup
}

// NewPool creates a Pool but does not start workers yet.
func NewPool(q *Queue, p *Processor) *Pool {
	return &Pool{queue: q, processor: p}
}

// Start launches n worker goroutines.
func (p *Pool) Start(n int) {
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for id := range p.queue.Chan() {
				p.processor.Process(id)
			}
		}()
	}
}

// Stop signals all workers to finish and blocks until they do.
// Closing the channel is the idiomatic Go way to broadcast "no more work":
// every worker's `for id := range channel` loop exits cleanly when the channel
// is closed and drained, so in-flight jobs complete before this returns.
func (p *Pool) Stop() {
	close(p.queue.ch)
	p.wg.Wait()
}
