package datadog

import (
	"fmt"
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

func TestWithTimeFunc(t *testing.T) {
	const now = int64(12345)
	options := options{}
	WithTimeFunc(func() int64 { return now }).Apply(&options)
	assert.Equal(t, now, options.timeFunc())
}

func TestWithTransporter(t *testing.T) {
	err := fmt.Errorf("argle bargle")
	fn := transporterFunc(func(groups [][]*Trace) error {
		return err
	})
	options := options{}
	WithTransporter(fn).Apply(&options)
	assert.Equal(t, err, options.transporter.Publish(nil))
}
