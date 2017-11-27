package datadog

import "github.com/opentracing/opentracing-go/log"

// LogContext provides an abstraction for metadata to be passed to the Log
type LogContext interface {
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
	baggage map[string]string
	tags    map[string]interface{}
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
