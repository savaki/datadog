package datadog

import (
	"testing"

	"github.com/tj/assert"
)

func TestLoggerContext(t *testing.T) {
	ctx := logContext{
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

	assert.Equal(t, 1, baggageCount)
	assert.Equal(t, 1, tagCount)
}
