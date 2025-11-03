package queue

import "testing"

type Item[T any] struct {
	Value T
	Err   error
}

type testingT interface {
	Helper()
	Fatal(args ...any)
}

var _ testingT = &testing.T{}

// Pop removes the next item from the queue and returns it.
func Pop[T any](t testingT, queue *[]Item[T]) (T, error) {
	t.Helper()

	if len(*queue) == 0 {
		t.Fatal("queue already drained")
	}

	ret, err := (*queue)[0].Value, (*queue)[0].Err
	updatedQueue := (*queue)[1:]
	*queue = updatedQueue

	return ret, err
}

// PopRetainLast removes the next item from the queue and returns it.
// The last item is not removed and therefore returned on every subsequent call.
func PopRetainLast[T any](t testingT, queue *[]Item[T]) (T, error) {
	t.Helper()

	if len(*queue) == 0 {
		t.Fatal("empty queue")
	}

	ret, err := (*queue)[0].Value, (*queue)[0].Err
	if len(*queue) > 1 {
		updatedQueue := (*queue)[1:]
		*queue = updatedQueue
	}

	return ret, err
}
