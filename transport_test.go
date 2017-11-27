package datadog

import (
	"context"
	"io"
	"testing"
	"time"
)

func BenchmarkTransportPush(t *testing.B) {
	tr := newTransport(DefaultBufSize, time.Second*20, func(ctx context.Context, contentType string, r io.Reader) error { return nil })
	defer tr.Close()

	span := &Span{}
	now := time.Now().UnixNano()
	service := "service"

	for i := 0; i < t.N; i++ {
		tr.Push(span, service, now)
	}
}

func TestTransportPush(t *testing.T) {
	tr := newTransport(DefaultBufSize, time.Second*20, func(ctx context.Context, contentType string, r io.Reader) error { return nil })
	defer tr.Close()

	span := &Span{}
	now := time.Now().UnixNano()
	service := "service"

	tr.Push(span, service, now)
}
