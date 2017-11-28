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

type TraceGroups [][]*Trace

// Len is the number of elements in the collection.
func (t TraceGroups) Len() int {
	return len(t)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (t TraceGroups) Less(i, j int) bool {
	return t[i][0].TraceID < t[j][0].TraceID
}

// Swap swaps the elements with indexes i and j.
func (t TraceGroups) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
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

		sort.Sort(TraceGroups(groups))
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
