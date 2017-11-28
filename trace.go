package datadog

import "sync"

var (
	traceGroupPool = &sync.Pool{
		New: func() interface{} {
			return make([][]*Trace, 0, 256)
		},
	}

	traceSlicePool = &sync.Pool{
		New: func() interface{} {
			return make([]*Trace, 0, 32)
		},
	}
)

type Trace struct {
	TraceID      uint64            `json:"trace_id"`
	SpanID       uint64            `json:"span_id"`
	Name         string            `json:"name"`
	Resource     string            `json:"resource"`
	Service      string            `json:"service"`
	Type         string            `json:"type"`
	Start        int64             `json:"start"`
	Duration     int64             `json:"duration"`
	ParentSpanID uint64            `json:"parent_id"`
	Error        int32             `json:"error"`
	Meta         map[string]string `json:"meta,omitempty"`
}

func (t *Trace) reset() *Trace {
	t.TraceID = 0
	t.SpanID = 0
	t.Name = ""
	t.Resource = ""
	t.Service = ""
	t.Type = ""
	t.Start = 0
	t.Duration = 0
	t.ParentSpanID = 0
	t.Error = 0

	for k := range t.Meta {
		delete(t.Meta, k)
	}

	return t
}

func release(groups [][]*Trace) {
	for i := len(groups) - 1; i >= 0; i-- {
		traces := groups[i]
		for j := len(traces) - 1; j >= 0; j-- {
			tracePool.Put(traces[j].reset())

			traces[j] = nil
		}
		traceSlicePool.Put(traces[:0])

		groups[i] = nil
	}
	traceGroupPool.Put(groups[:0])
}

type queue struct {
	sync.Mutex
	transporter Transporter
	traces      map[uint64][]*Trace
	threshold   int
	n           int
}

func newQueue(threshold int, transporter Transporter) *queue {
	return &queue{
		transporter: transporter,
		traces:      map[uint64][]*Trace{},
		threshold:   threshold,
	}
}

func (q *queue) publish() {
	if q.n == 0 {
		return
	}

	groups := traceGroupPool.Get().([][]*Trace)
	for key, item := range q.traces {
		delete(q.traces, key)
		groups = append(groups, item)
	}

	q.transporter.Publish(groups)
	q.n = 0
}

func (q *queue) Push(t *Trace) {
	q.Lock()
	defer q.Unlock()

	slice, ok := q.traces[t.TraceID]
	if !ok {
		slice = traceSlicePool.Get().([]*Trace)
	}

	q.traces[t.TraceID] = append(slice, t)
	q.n++
	if q.n == q.threshold {
		q.publish()
	}
}

func (q *queue) Flush() {
	q.Lock()
	defer q.Unlock()

	q.publish()
}
