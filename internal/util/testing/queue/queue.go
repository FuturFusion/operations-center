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

// PopErr removes the next item from the queue and returns only the error
// from it.
func PopErr[T any](t testingT, queue *[]Item[T]) error {
	t.Helper()

	if len(*queue) == 0 {
		t.Fatal("queue already drained")
	}

	_, err := (*queue)[0].Value, (*queue)[0].Err
	updatedQueue := (*queue)[1:]
	*queue = updatedQueue

	return err
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

type Errs []error

// Pop removes the first item from the error queue and returns it.
func (e *Errs) Pop(t *testing.T) error {
	t.Helper()

	if len(*e) == 0 {
		t.Fatal("queue already drained")
	}

	err := (*e)[0]
	updatedErrs := (*e)[1:]
	*e = updatedErrs

	return err
}
