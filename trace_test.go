package datadog

import (
	"sort"
	"testing"

	"github.com/tj/assert"
)

func TestTraceReset(t *testing.T) {
	trace := &Trace{
		TraceID:      123,
		SpanID:       123,
		Name:         "blah",
		Resource:     "blah",
		Service:      "blah",
		Type:         "blah",
		Start:        123,
		Duration:     123,
		ParentSpanID: 123,
		Error:        1,
		Meta: map[string]string{
			"hello": "world",
		},
	}

	trace.reset()

	assert.Len(t, trace.Meta, 0)
	trace.Meta = nil

	zero := &Trace{}
	assert.EqualValues(t, zero, trace)
}

func TestRelease(t *testing.T) {
	trace := &Trace{
		TraceID: 123,
	}
	groups := [][]*Trace{
		{
			trace,
		},
	}
	release(groups)

	zero := &Trace{}
	assert.EqualValues(t, zero, trace)
}

func TestQueue(t *testing.T) {
	t1 := &Trace{TraceID: 1}
	t2 := &Trace{TraceID: 2}
	t3 := &Trace{TraceID: 3}

	t.Run("publishes traces when threshold is reached", func(t *testing.T) {
		var groups [][]*Trace
		q := newQueue(3, transporterFunc(func(g [][]*Trace) error {
			groups = g
			return nil
		}))

		q.Push(t1)
		assert.Len(t, groups, 0)

		q.Push(t2)
		assert.Len(t, groups, 0)

		q.Push(t3)
		assert.Len(t, groups, 3)

		sort.Slice(groups, func(i, j int) bool {
			return groups[i][0].TraceID < groups[j][0].TraceID
		})
		assert.EqualValues(t, [][]*Trace{{t1}, {t2}, {t3}}, groups)
	})

	t.Run("flush forces publish to be called", func(t *testing.T) {
		var groups [][]*Trace
		q := newQueue(3, transporterFunc(func(g [][]*Trace) error {
			groups = g
			return nil
		}))

		q.Push(t1)
		assert.Len(t, groups, 0)

		q.Flush()
		assert.Len(t, groups, 1)

		groups = nil
		q.Flush()
		assert.Len(t, groups, 0)
	})
}

func BenchmarkQueue(t *testing.B) {
	q := newQueue(DefaultThreshold, nopTransporter)

	for i := 0; i < t.N; i++ {
		t := tracePool.Get().(*Trace)
		t.TraceID = uint64(i)

		q.Push(t)
	}
}
