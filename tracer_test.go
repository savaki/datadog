package datadog_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/savaki/datadog"
	"github.com/tj/assert"
)

func TestMultiLogger(t *testing.T) {
	count := 0
	target := datadog.LoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
		count++
	})

	logger := datadog.MultiLogger(target, target)
	logger.Log(nil, log.String("hello", "world"))
	assert.Equal(t, 2, count)
}

func TestTracer_LogFields(t *testing.T) {
	var received []log.Field

	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
		datadog.WithBaggageItem("a", "b"),
		datadog.WithLoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
			received = fields
		}),
	)
	defer tracer.Close()

	field := log.String("hello", "world")
	tracer.LogFields(field)
	assert.Len(t, received, 1)
	assert.Equal(t, field, received[0])
}

func TestWithBaggage(t *testing.T) {
	calls := 0

	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
		datadog.WithBaggageItem("hello", "world"),
		datadog.WithLoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
			ctx.ForeachBaggageItem(func(key, value string) bool {
				assert.Equal(t, "hello", key)
				assert.Equal(t, "world", value)
				calls++
				return true
			})
		}),
	)
	defer tracer.Close()

	a := tracer.StartSpan("a")
	a.LogFields()

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	b.LogFields()
	b.Finish()

	a.Finish()
	assert.EqualValues(t, 2, calls)
}

func TestLive(t *testing.T) {
	tracer, _ := datadog.New("blah")
	defer tracer.Close()

	parent := tracer.StartSpan("parent")
	time.Sleep(time.Millisecond * 100)

	child := tracer.StartSpan("child", opentracing.ChildOf(parent.Context()))
	time.Sleep(time.Millisecond * 50)
	child.SetTag("hello", "world")
	time.Sleep(time.Millisecond * 25)
	child.Finish()

	time.Sleep(time.Millisecond * 50)
	parent.Finish()

	tracer.Flush()
	tracer.Close()

	time.Sleep(time.Second)
}

func TestBinaryCarrier(t *testing.T) {
	tracer, _ := datadog.New("blah", datadog.WithNop())
	defer tracer.Close()

	span := tracer.StartSpan("parent")
	span.SetBaggageItem("hello", "world")

	carrier := bytes.NewBuffer(nil)

	// inject
	err := tracer.Inject(span.Context(), opentracing.Binary, carrier)
	assert.Nil(t, err)

	// extract
	sc, err := tracer.Extract(opentracing.Binary, carrier)
	assert.Nil(t, err)

	child := tracer.StartSpan("child", opentracing.ChildOf(sc))
	assert.Equal(t, "world", child.BaggageItem("hello"))
}

func TestTextMapCarrier(t *testing.T) {
	tracer, _ := datadog.New("blah", datadog.WithNop())
	defer tracer.Close()

	span := tracer.StartSpan("parent")
	span.SetBaggageItem("hello", "world")
	carrier := opentracing.TextMapCarrier{}

	// inject
	err := tracer.Inject(span.Context(), opentracing.TextMap, carrier)
	assert.Nil(t, err)

	// extract
	sc, err := tracer.Extract(opentracing.TextMap, carrier)
	assert.Nil(t, err)

	child := tracer.StartSpan("child", opentracing.ChildOf(sc))
	assert.Equal(t, "world", child.BaggageItem("hello"))
}

func TestHTTPHeadersCarrier(t *testing.T) {
	tracer, _ := datadog.New("blah", datadog.WithNop())
	defer tracer.Close()

	span := tracer.StartSpan("parent")
	span.SetBaggageItem("hello", "world")
	carrier := http.Header{}

	// inject
	err := tracer.Inject(span.Context(), opentracing.HTTPHeaders, carrier)
	assert.Nil(t, err)

	// extract
	sc, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	assert.Nil(t, err)

	child := tracer.StartSpan("child", opentracing.ChildOf(sc))
	assert.Equal(t, "world", child.BaggageItem("hello"))
}
