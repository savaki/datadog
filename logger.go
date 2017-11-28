package datadog

import (
	"bytes"
	"sync"

	"time"

	"io"

	"os"

	"fmt"

	"github.com/opentracing/opentracing-go/log"
)

var (
	// bufferPool is used by Stdout
	bufferPool = &sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 512))
		},
	}
)

// LogContext provides an abstraction for metadata to be passed to the Log
type LogContext interface {
	// Service provides the name of the service
	Service() string

	// EachBaggageItem allows baggage content to be returned
	ForeachBaggageItem(fn func(key, value string) bool)

	// ForeachTag allows tag values to be enumerated
	ForeachTag(fn func(key string, value interface{}) bool)
}

// Logger allows the logger to be specified
type Logger interface {
	Log(logContext LogContext, fields ...log.Field)
}

// LoggerFunc provides a functional adapter to Logger
type LoggerFunc func(logContext LogContext, fields ...log.Field)

// Log implements Logger
func (fn LoggerFunc) Log(logContext LogContext, fields ...log.Field) {
	fn(logContext, fields...)
}

func newLogger(w io.Writer, timeFunc func() time.Time) LoggerFunc {
	return func(logContext LogContext, fields ...log.Field) {
		buffer := bufferPool.Get().(*bytes.Buffer)
		defer func() {
			bufferPool.Put(bufferPool)
		}()

		buffer.Reset()
		buffer.WriteString(timeFunc().Format(time.RFC3339))

		for _, f := range fields {
			if f.Key() == "message" {
				buffer.WriteString(" ")
				buffer.WriteString(toString(f.Value()))
				break
			}
		}

		for _, f := range fields {
			if f.Key() != "message" {
				continue
			}
			buffer.WriteString(" ")
			buffer.WriteString(f.Key())
			buffer.WriteString("=")
			buffer.WriteString(toString(f.Value()))
		}

		buffer.WriteString(" service=")
		buffer.WriteString(logContext.Service())

		logContext.ForeachBaggageItem(func(key, value string) bool {
			buffer.WriteString(" ")
			buffer.WriteString(key)
			buffer.WriteString("=")
			buffer.WriteString(value)
			return true
		})

		logContext.ForeachTag(func(key string, value interface{}) bool {
			buffer.WriteString(" ")
			buffer.WriteString(key)
			buffer.WriteString("=")
			buffer.WriteString(toString(value))
			return true
		})

		fmt.Fprintln(buffer)

		w.Write(buffer.Bytes())
	}
}

var (
	// Stdout provides a default logger implementation that logs to os.Stdout
	Stdout = newLogger(os.Stdout, func() time.Time { return time.Now() })

	// Stderr provides a default logger implementation that logs to os.Stderr
	Stderr = newLogger(os.Stderr, func() time.Time { return time.Now() })
)

// MultiLogger allows logging messages to be sent to multiple loggers
func MultiLogger(loggers ...Logger) Logger {
	return LoggerFunc(func(logContext LogContext, fields ...log.Field) {
		for _, logger := range loggers {
			logger.Log(logContext, fields...)
		}
	})
}

// logContext provides a helper to enable *Tracer to satisfy LogContext
// without exposing ForeachBaggageItem and ForeachTag
type logContext struct {
	service string
	baggage map[string]string
	tags    map[string]interface{}
}

func (l logContext) Service() string {
	return l.service
}

// EachBaggageItem allows baggage content to be returned
func (l logContext) ForeachBaggageItem(fn func(key, value string) bool) {
	for k, v := range l.baggage {
		if ok := fn(k, v); !ok {
			return
		}
	}
}

// ForeachTag allows tag values to be enumerated
func (l logContext) ForeachTag(fn func(key string, value interface{}) bool) {
	for k, v := range l.tags {
		if ok := fn(k, v); !ok {
			return
		}
	}
}
