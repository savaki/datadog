package datadog_test

import (
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

	tracer := datadog.New("blah",
		datadog.WithBaggageItem("a", "b"),
		datadog.WithLoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
			received = fields
		}),
	)

	field := log.String("hello", "world")
	tracer.LogFields(field)
	assert.Len(t, received, 1)
	assert.Equal(t, field, received[0])
}

func TestWithBaggage(t *testing.T) {
	calls := 0

	tracer := datadog.New("blah",
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

	a := tracer.StartSpan("a")
	a.LogFields()

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	b.LogFields()
	b.Finish()

	a.Finish()
	assert.EqualValues(t, 2, calls)
}

func TestLive(t *testing.T) {
	tracer := datadog.New("blah")

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
