package datadog

import (
	"fmt"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/tj/assert"
)

func TestError(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		err := fmt.Errorf("boom")
		stack := extractStackTrace(err)
		assert.NotZero(t, stack)
	})

	t.Run("github.com/pkg/errors", func(t *testing.T) {
		err := errors.Wrap(io.EOF, "boom")
		stack := extractStackTrace(err)
		assert.NotZero(t, stack)
	})
}
