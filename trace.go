package datadog

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

func (t *Trace) release() {
	for k := range t.Meta {
		delete(t.Meta, k)
	}

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

	tracePool.Put(t)
}

type traceMap map[uint64][]*Trace

func (tm traceMap) push(t *Trace) {
	if tm == nil || t == nil {
		return
	}

	traces, ok := tm[t.TraceID]
	if !ok {
		traces = []*Trace{}
	}

	tm[t.TraceID] = append(traces, t)
}

func (tm traceMap) array() [][]*Trace {
	var traces [][]*Trace
	for _, items := range tm {
		traces = append(traces, items)
	}
	return traces
}

func (tm traceMap) release() {
	for key, traces := range tm {
		for i := len(traces) - 1; i >= 0; i-- {
			trace := traces[i]
			trace.release()
			traces[i] = nil
		}
		delete(tm, key)
	}
}
