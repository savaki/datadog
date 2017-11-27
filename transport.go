package datadog

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"

	"github.com/ugorji/go/codec"
)

var (
	mh codec.MsgpackHandle
)

var (
	tracePool = &sync.Pool{
		New: func() interface{} {
			return &trace{
				Meta: map[string]string{},
			}
		},
	}

	encoderPool = &sync.Pool{
		New: func() interface{} {
			buffer := bytes.NewBuffer(make([]byte, 0, DefaultBufSize))
			return &msgpackEncoder{
				buffer:  buffer,
				encoder: codec.NewEncoder(buffer, &mh),
			}
		},
	}

	payloadPool = &sync.Pool{
		New: func() interface{} {
			return make([]*trace, 0, DefaultBufSize)
		},
	}
)

type trace struct {
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

func (t *trace) release() {
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

type traceMap map[uint64][]*trace

func (tm traceMap) push(t *trace) {
	if tm == nil || t == nil {
		return
	}

	traces, ok := tm[t.TraceID]
	if !ok {
		traces = []*trace{}
	}

	tm[t.TraceID] = append(traces, t)
}

func (tm traceMap) Array() [][]*trace {
	var traces [][]*trace
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

func (tr *transport) publish(ctx context.Context, payload []*trace) error {
	defer tr.wg.Done()

	defer func() {
		for i := len(payload) - 1; i >= 0; i-- {
			payload[i] = nil
		}
		payload = payload[:0]
		payloadPool.Put(payload)
	}()

	tm := traceMap{}
	defer tm.release()

	for _, item := range payload {
		tm.push(item)
	}

	encoder := encoderPool.Get().(traceEncoder)
	defer func() {
		encoderPool.Put(encoder)
	}()

	if err := encoder.Encode(tm.Array()); err != nil {
		return err
	}

	for attempts := 0; attempts < 5; attempts++ {
		if err := tr.submitter(ctx, encoder.ContentType(), encoder); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	return nil
}

type transport struct {
	sync.Mutex
	wg        *sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	traces    []*trace
	offset    int
	bufSize   int
	submitter func(ctx context.Context, contentType string, r io.Reader) error
	interval  time.Duration
}

func newTransport(bufSize int, interval time.Duration, submitter func(ctx context.Context, contentType string, r io.Reader) error) *transport {
	ctx, cancel := context.WithCancel(context.Background())

	t := &transport{
		wg:        &sync.WaitGroup{},
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		traces:    make([]*trace, bufSize),
		bufSize:   bufSize,
		submitter: submitter,
		interval:  interval,
	}

	go t.run()

	return t
}

func (tr *transport) run() {
	defer close(tr.done)

	t := time.NewTicker(tr.interval)
	defer t.Stop()

	for {
		select {
		case <-tr.ctx.Done():
			return
		case <-t.C:
			tr.Flush()
		}
	}
}

func (tr *transport) Close() error {
	tr.cancel()
	<-tr.done
	tr.wg.Wait()
	return nil
}

func (tr *transport) flush() {
	if tr.offset == 0 {
		return
	}

	payload := payloadPool.Get().([]*trace)
	payload = append(payload, tr.traces[0:tr.offset]...)

	for i := 0; i < tr.offset; i++ {
		tr.traces[i] = nil
	}

	tr.wg.Add(1)
	go tr.publish(tr.ctx, payload)

	tr.offset = 0
}

func (tr *transport) Flush() {
	tr.Lock()
	defer tr.Unlock()

	tr.flush()
}

func (tr *transport) Push(span *Span, service string, nowInEpochNano int64) {
	tr.Lock()
	defer tr.Unlock()

	resource := span.resource
	if resource == "" {
		resource = "-"
	}

	typ := span.typ
	if typ == "" {
		typ = TypeWeb
	}

	t := tracePool.Get().(*trace)
	t.TraceID = span.traceID
	t.SpanID = span.spanID
	t.Name = span.operationName
	t.Resource = resource
	t.Service = service
	t.Type = typ
	t.Start = span.startedAt
	t.Duration = nowInEpochNano - span.startedAt
	t.ParentSpanID = span.parentSpanID
	t.Error = span.hasError

	for k, v := range span.baggage {
		t.Meta[k] = v
	}
	for k, v := range span.tags {
		t.Meta[k] = v
	}

	tr.traces[tr.offset] = t
	tr.offset++
	if tr.offset == tr.bufSize {
		tr.flush()
	}
}
