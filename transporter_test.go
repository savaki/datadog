package datadog

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/tj/assert"
)

func TestWithRetry(t *testing.T) {
	fn := transporterFunc(func(groups [][]*Trace) error {
		return io.EOF
	})

	fn = withRetry(fn, 3, time.Millisecond*5)
	assert.NotNil(t, fn(nil))
}

func TestNewTransporter(t *testing.T) {
	t.Run("nop when host or port is zero", func(t *testing.T) {
		buffer := bytes.NewBuffer(nil)
		tr := newTransporter(buffer, "", "")
		assert.NotNil(t, tr)
		assert.NotZero(t, buffer.String(), "expected an error message")
	})

	t.Run("nop when host or port is zero", func(t *testing.T) {
		buffer := bytes.NewBuffer(nil)
		tr := newTransporter(buffer, "_", "_ _")
		assert.NotNil(t, tr)
		assert.NotZero(t, buffer.String(), "expected an error message")
	})
}
