package datadog

import (
	"testing"

	"github.com/tj/assert"
)

func TestWithBufSize(t *testing.T) {
	const size = 1024
	options := options{}
	WithBufSize(size).Apply(&options)
	assert.Equal(t, size, options.threshold)
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
