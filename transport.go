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
			return &Trace{
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
			return make([]*Trace, 0, DefaultBufSize)
		},
	}
)

func (tr *transport) publish(ctx context.Context, payload []*Trace) error {
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

	if err := encoder.Encode(tm.array()); err != nil {
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
	traces    []*Trace
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
		traces:    make([]*Trace, bufSize),
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

	payload := payloadPool.Get().([]*Trace)
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

	t := tracePool.Get().(*Trace)
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
