package worker

import (
	"testing"
)

func TestQueueFIFOOrder(t *testing.T) {
	q := NewQueue(10)

	ids := []string{"first", "second", "third"}
	for _, id := range ids {
		q.Enqueue(id)
	}

	for _, want := range ids {
		select {
		case got := <-q.Chan():
			if got != want {
				t.Errorf("want %s, got %s", want, got)
			}
		default:
			t.Fatalf("expected %q in queue but channel was empty", want)
		}
	}
}

func TestQueueBuffersUpToCapacity(t *testing.T) {
	q := NewQueue(3)
	q.Enqueue("a")
	q.Enqueue("b")
	q.Enqueue("c")

	if n := len(q.ch); n != 3 {
		t.Errorf("want 3 buffered items, got %d", n)
	}
}
