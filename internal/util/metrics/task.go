package metrics

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// taskDurationBuckets cover tasks from 100 milliseconds up to 10 minutes.
// prometheus.DefBuckets end at 10 seconds, which is too short for a refresh
// of the updates or the inventory. Few buckets keep the number of series low.
var taskDurationBuckets = []float64{0.1, 0.5, 1, 5, 10, 60, 300, 600}

var (
	taskErrorsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: Namespace,
		Subsystem: "task",
		Name:      "errors_total",
		Help:      "Number of background task runs that failed.",
	}, []string{"task", "retryable"})

	taskDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: Namespace,
		Subsystem: "task",
		Name:      "duration_seconds",
		Help:      "Duration of the background task runs in seconds.",
		Buckets:   taskDurationBuckets,
	}, []string{"task"})

	taskLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "task",
		Name:      "last_success_timestamp_seconds",
		Help:      "Unix time of the last successful background task run. Reset to the current time whenever the task starts on this member.",
	}, []string{"task"})

	taskInterval = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Subsystem: "task",
		Name:      "interval_seconds",
		Help:      "Interval of the background task in seconds.",
	}, []string{"task"})
)

var (
	tasksMu sync.Mutex
	tasks   = map[string]*Task{}
)

func init() {
	registry.MustRegister(taskCollector{})
}

// Task records the metrics of one background task.
//
// The series of a task carry the label "task" with its name. They are
// exposed only while the component of the task is enabled. The gauges are
// exposed only while the task runs on this member. On a cluster, only the
// leader runs the background tasks.
//
// Use NewTask. The zero value is not usable.
type Task struct {
	component *Component
	running   atomic.Bool

	errorsHard      prometheus.Counter
	errorsRetryable prometheus.Counter
	duration        prometheus.Histogram
	lastSuccess     prometheus.Gauge
	interval        prometheus.Gauge
}

// NewTask returns the metrics of the task with the given name. The component
// gates the metrics. Calling it again with the same name returns the same
// task. It panics if the component is nil or differs from the first call.
func NewTask(c *Component, name string) *Task {
	if c == nil {
		panic("metrics: NewTask requires a component")
	}

	tasksMu.Lock()
	defer tasksMu.Unlock()

	t, ok := tasks[name]
	if ok {
		if t.component != c {
			panic(fmt.Sprintf("metrics: task %q is already registered with component %q", name, t.component.Name()))
		}

		return t
	}

	t = &Task{
		component: c,

		// Create the error series up front, so they start at zero.
		errorsHard:      taskErrorsTotal.WithLabelValues(name, "false"),
		errorsRetryable: taskErrorsTotal.WithLabelValues(name, "true"),
		duration:        taskDuration.WithLabelValues(name).(prometheus.Histogram),
		lastSuccess:     taskLastSuccess.WithLabelValues(name),
		interval:        taskInterval.WithLabelValues(name),
	}
	tasks[name] = t

	return t
}

// Started marks the task as running on this member with the given interval.
// It resets the last success to now so alerts can detect tasks that never succeed.
func (t *Task) Started(interval time.Duration) {
	t.interval.Set(interval.Seconds())
	t.lastSuccess.SetToCurrentTime()
	t.running.Store(true)
}

// Stopped marks the task as no longer running on this member. Its gauges are
// no longer exposed. The counters and the histogram keep their values.
func (t *Task) Stopped() {
	t.running.Store(false)
}

// Observe records a run of the task that began at start and ended with err.
//
// Canceled runs are omitted because shutdown or loss of leadership can interrupt them.
func (t *Task) Observe(ctx context.Context, start time.Time, err error) {
	if ctx.Err() != nil {
		return
	}

	// The last success is state, not a counter. Keep it current while the
	// component is disabled. Enabling the component then shows no stale value.
	if err == nil {
		t.lastSuccess.SetToCurrentTime()
	}

	if !t.component.Enabled() {
		return
	}

	t.duration.Observe(time.Since(start).Seconds())

	if err == nil {
		return
	}

	if IsRetryable(err) {
		t.errorsRetryable.Inc()
		return
	}

	t.errorsHard.Inc()
}

// taskCollector exposes the series of each task according to its component
// and its running state. A gated collector per task is not possible: all
// tasks share the same metric names.
type taskCollector struct{}

// Describe implements prometheus.Collector.
func (taskCollector) Describe(ch chan<- *prometheus.Desc) {
	taskErrorsTotal.Describe(ch)
	taskDuration.Describe(ch)
	taskLastSuccess.Describe(ch)
	taskInterval.Describe(ch)
}

// Collect implements prometheus.Collector.
func (taskCollector) Collect(ch chan<- prometheus.Metric) {
	tasksMu.Lock()
	defer tasksMu.Unlock()

	for _, t := range tasks {
		if !t.component.Enabled() {
			continue
		}

		ch <- t.errorsHard
		ch <- t.errorsRetryable
		ch <- t.duration

		if t.running.Load() {
			ch <- t.lastSuccess
			ch <- t.interval
		}
	}
}
