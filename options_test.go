package datadog

import (
	"testing"
	"time"

	"github.com/tj/assert"
)

func TestWithFlushInterval(t *testing.T) {
	const d = time.Second
	options := options{}
	WithFlushInterval(d).Apply(&options)
	assert.Equal(t, d, options.flushInterval)
}

func TestWithBufSize(t *testing.T) {
	const size = 1024
	options := options{}
	WithBufSize(size).Apply(&options)
	assert.Equal(t, size, options.bufSize)
}

func TestWithHost(t *testing.T) {
	const host = "host"
	options := options{}
	WithHost(host).Apply(&options)
	assert.Equal(t, host, options.host)
}

func TestWithPort(t *testing.T) {
	const port = "8080"
	options := options{}
	WithPort(port).Apply(&options)
	assert.Equal(t, port, options.port)
}
