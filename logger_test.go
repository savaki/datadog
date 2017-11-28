package datadog

import (
	"bytes"
	"io/ioutil"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go/log"
	"github.com/tj/assert"
)

func TestLoggerContext(t *testing.T) {
	ctx := logContext{
		service: "service",
		baggage: map[string]string{"hello": "world"},
		tags:    map[string]interface{}{"key": "value"},
	}

	baggageCount := 0
	ctx.ForeachBaggageItem(func(k, v string) bool {
		baggageCount++
		assert.Equal(t, "hello", k)
		assert.Equal(t, "world", v)
		return true
	})

	tagCount := 0
	ctx.ForeachTag(func(k string, v interface{}) bool {
		tagCount++
		assert.Equal(t, "key", k)
		assert.Equal(t, "value", v)
		return true
	})

	assert.Equal(t, ctx.service, ctx.Service())
	assert.Equal(t, 1, tagCount)
	assert.Equal(t, 1, tagCount)
}

func TestNewLogger(t *testing.T) {
	ctx := logContext{
		service: "service",
		baggage: map[string]string{"hello": "world"},
		tags:    map[string]interface{}{"key": "value"},
	}

	tm, err := time.Parse(time.RFC3339, "2017-11-28T08:21:10-08:00")
	assert.Nil(t, err)

	buffer := bytes.NewBuffer(nil)
	logger := newLogger(buffer, func() time.Time { return tm })
	logger.Log(ctx, log.String("message", "log message"), log.Int("count", 123))

	data, err := ioutil.ReadAll(buffer)
	assert.Nil(t, err)
	assert.Equal(t, "2017-11-28T08:21:10-08:00 log message message=log message service=service hello=world key=value\n", string(data))
}
