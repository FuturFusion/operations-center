package queue

import "testing"

type Item[T any] struct {
	Value T
	Err   error
}

// Pop removes the next item from the queue and returns it.
func Pop[T any](t *testing.T, queue *[]Item[T]) (T, error) {
	t.Helper()

	if len(*queue) == 0 {
		t.Fatal("queue already drained")
	}

	ret, err := (*queue)[0].Value, (*queue)[0].Err
	updatedQueue := (*queue)[1:]
	*queue = updatedQueue

	return ret, err
}
