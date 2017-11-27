// Copyright 2017 Matt Ho
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package datadog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/savaki/datadog/ext"
)

var (
	spanPool = sync.Pool{
		New: func() interface{} {
			return &Span{}
		},
	}
)

// Span references a dapper Span
type Span struct {
	traceID       uint64
	spanID        uint64
	parentSpanID  uint64
	tracer        *Tracer
	baggage       map[string]string
	tags          map[string]interface{}
	operationName string
	startedAt     int64
	hasError      int32
	resource      string
	typ           string
	err           error
}

func (s *Span) release() {
	for key := range s.baggage {
		delete(s.baggage, key)
	}
	for key := range s.tags {
		delete(s.tags, key)
	}

	s.traceID = 0
	s.spanID = 0
	s.parentSpanID = 0
	s.tracer = nil
	s.operationName = ""
	s.startedAt = 0
	s.hasError = 0
	s.resource = ""
	s.typ = ""
	s.err = nil

	spanPool.Put(s)
}

// ForeachBaggageItem implements SpanContext and LogContext
func (s *Span) ForeachBaggageItem(fn func(k, v string) bool) {
	for k, v := range s.baggage {
		if ok := fn(k, v); !ok {
			return
		}
	}
}

// ForeachTag implements LogContext
func (s *Span) ForeachTag(fn func(k string, value interface{}) bool) {
	for k, v := range s.tags {
		if ok := fn(k, v); !ok {
			return
		}
	}
}

// Sets the end timestamp and finalizes *Span state.
//
// With the exception of calls to Context() (which are always allowed),
// Finish() must be the last call made to any span instance, and to do
// otherwise leads to undefined behavior.
func (s *Span) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{})
}

// FinishWithOptions is like Finish() but with explicit control over
// timestamps and log data.
func (s *Span) FinishWithOptions(opts opentracing.FinishOptions) {
	defer s.release()

	var finishedAt int64
	if opts.FinishTime.IsZero() {
		finishedAt = s.tracer.now()
	} else {
		finishedAt = opts.FinishTime.UnixNano()
	}

	s.tracer.transport.Push(s, s.tracer.service, finishedAt)
	if s.tracer.logSpans {
		s.LogFields(log.String("operationName", s.operationName))
	}
}

// Context() yields the SpanContext for this *Span. Note that the return
// value of Context() is still valid after a call to Span.Finish(), as is
// a call to Span.Context() after a call to Span.Finish().
func (s *Span) Context() opentracing.SpanContext {
	return s
}

type pp struct {
	w io.Writer
}

func (p pp) Write(data []byte) (int, error) {
	return p.w.Write(data)
}

// Width returns the value of the width option and whether it has been set.
func (p pp) Width() (wid int, ok bool) {
	return 0, false
}

// Precision returns the value of the precision option and whether it has been set.
func (p pp) Precision() (precision int, ok bool) {
	return 0, false
}

// Flag reports whether the flag c, a character, has been set.
func (p pp) Flag(c int) bool {
	return false
}

// Sets or changes the operation name.
func (s *Span) SetOperationName(operationName string) opentracing.Span {
	s.operationName = operationName
	return s
}

func (s *Span) setError(err error) {
	if err == nil {
		return
	}

	atomic.StoreInt32(&s.hasError, 1)

	s.tags[ext.ErrorMsg] = err.Error()
	s.tags[ext.ErrorType] = reflect.TypeOf(err).String()

	switch v := err.(type) {
	case fmt.Formatter:
		buffer := bytes.NewBuffer(nil)
		v.Format(pp{w: buffer}, 'v')
		s.tags[ext.ErrorStack] = string(buffer.String())

	default:
		stack := debug.Stack()
		s.tags[ext.ErrorStack] = string(stack)
	}
}

// Adds a tag to the span.
//
// If there is a pre-existing tag set for `key`, it is overwritten.
//
// Tag values can be numeric types, strings, or bools. The behavior of
// other tag value types is undefined at the OpenTracing level. If a
// tracing system does not know how to handle a particular value type, it
// may ignore the tag, but shall not panic.
func (s *Span) SetTag(key string, value interface{}) opentracing.Span {
	if s.tags == nil {
		s.tags = map[string]interface{}{}
	}

	switch key {
	case ext.Error:
		err, ok := value.(error)
		if !ok {
			err = fmt.Errorf("error set")
		}
		s.setError(err)
		return s

	case ext.Resource:
		v, ok := value.(string)
		if ok {
			s.resource = v
		}
		return s

	case ext.Type:
		v, ok := value.(string)
		if ok {
			s.typ = v
		}
		return s
	}

	if err, ok := value.(error); ok {
		s.setError(err)
	}

	s.tags[key] = value

	return s
}

// LogFields is an efficient and type-checked way to record key:value
// logging data about a Span, though the programming interface is a little
// more verbose than LogKV(). Here's an example:
//
//    span.LogFields(
//        log.String("event", "soft error"),
//        log.String("type", "cache timeout"),
//        log.Int("waited.millis", 1500))
//
// Also see Span.FinishWithOptions() and FinishOptions.BulkLogData.
func (s *Span) LogFields(fields ...log.Field) {
	for _, f := range fields {
		if err, ok := f.Value().(error); ok {
			s.setError(err)
		}
	}
	s.tracer.logger.Log(s, fields...)
}

func toField(key string, v interface{}) log.Field {
	switch value := v.(type) {
	case error:
		return log.Error(value)
	case bool:
		return log.Bool(key, value)
	case string:
		return log.String(key, value)
	case float32:
		return log.Float32(key, value)
	case float64:
		return log.Float64(key, value)
	case int:
		return log.Int(key, value)
	case int32:
		return log.Int32(key, value)
	case int64:
		return log.Int64(key, value)
	case uint32:
		return log.Uint32(key, value)
	case uint64:
		return log.Uint64(key, value)
	case fmt.Stringer:
		return log.String(key, value.String())
	default:
		return log.Object(key, value)
	}
}

// LogKV is a concise, readable way to record key:value logging data about
// a Span, though unfortunately this also makes it less efficient and less
// type-safe than LogFields(). Here's an example:
//
//    span.LogKV(
//        "event", "soft error",
//        "type", "cache timeout",
//        "waited.millis", 1500)
//
// For LogKV (as opposed to LogFields()), the parameters must appear as
// key-value pairs, like
//
//    span.LogKV(key1, val1, key2, val2, key3, val3, ...)
//
// The keys must all be strings. The values may be strings, numeric types,
// bools, Go error instances, or arbitrary structs.
//
// (Note to implementors: consider the log.InterleavedKVToFields() helper)
func (s *Span) LogKV(alternatingKeyValues ...interface{}) {
	if s == nil {
		return
	}

	n := len(alternatingKeyValues) / 2
	fields := make([]log.Field, 0, n)

	for i := 0; i < n; i++ {
		k := alternatingKeyValues[i*2]
		key, ok := k.(string)
		if !ok {
			continue
		}

		v := alternatingKeyValues[i*2+1]
		if v == nil {
			continue
		}

		if err, ok := v.(error); ok {
			s.setError(err)
		}

		fields = append(fields, toField(key, v))
	}

	s.tracer.logger.Log(s, fields...)
}

// SetBaggageItem sets a key:value pair on this *Span and its *SpanContext
// that also propagates to descendants of this *Span.
//
// SetBaggageItem() enables powerful functionality given a full-stack
// opentracing integration (e.g., arbitrary application data from a mobile
// app can make it, transparently, all the way into the depths of a storage
// system), and with it some powerful costs: use this feature with care.
//
// IMPORTANT NOTE #1: SetBaggageItem() will only propagate baggage items to
// *future* causal descendants of the associated Span.
//
// IMPORTANT NOTE #2: Use this thoughtfully and with care. Every key and
// value is copied into every local *and remote* child of the associated
// Span, and that can add up to a lot of network and cpu overhead.
//
// Returns a reference to this *Span for chaining.
func (s *Span) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	if s.baggage == nil {
		s.baggage = map[string]string{}
	}
	s.baggage[restrictedKey] = value

	return s
}

// Gets the value for a baggage item given its key. Returns the empty string
// if the value isn't found in this *Span.
func (s *Span) BaggageItem(restrictedKey string) string {
	return s.baggage[restrictedKey]
}

// Provides access to the Tracer that created this *Span.
func (s *Span) Tracer() opentracing.Tracer {
	return s.tracer
}

// Deprecated: use LogFields or LogKV
func (s *Span) LogEvent(event string) {
	fmt.Fprintln(os.Stderr, "Span.LogEvent is deprecated. Use LogFields or LogKV")
}

// Deprecated: use LogFields or LogKV
func (s *Span) LogEventWithPayload(event string, payload interface{}) {
	fmt.Fprintln(os.Stderr, "Span.LogEventWithPayload is deprecated. Use LogFields or LogKV")
}

// Deprecated: use LogFields or LogKV
func (s *Span) Log(data opentracing.LogData) {
	fmt.Fprintln(os.Stderr, "Span.Log is deprecated. Use LogFields or LogKV")
}

type identity struct {
	TraceID uint64            `json:"t,string"`
	SpanID  uint64            `json:"s,string"`
	Baggage map[string]string `json:"b,omitempty"`
}

func (s *Span) MarshalJSON() ([]byte, error) {
	return json.Marshal(identity{
		TraceID: s.traceID,
		SpanID:  s.spanID,
		Baggage: s.baggage,
	})
}

func (s *Span) UnmarshalJSON(data []byte) error {
	var id identity
	if err := json.Unmarshal(data, &id); err != nil {
		return ErrInvalidToken
	}

	span := spanPool.Get().(*Span)
	span.traceID = id.TraceID
	span.parentSpanID = id.SpanID
	span.baggage = id.Baggage
	*s = *span
	return nil
}

var (
	// ErrInvalidToken if the token provided cannot be unmarshaled
	ErrInvalidToken = errors.New("invalid token")
)

type StartSpanOption interface {
	Apply(opts *opentracing.StartSpanOptions)
}

type spanOptionFunc func(*opentracing.StartSpanOptions)

func (fn spanOptionFunc) Apply(opts *opentracing.StartSpanOptions) {
	fn(opts)
}

// Resource specifies the resource being accessed
func Resource(r string) StartSpanOption {
	return spanOptionFunc(func(opts *opentracing.StartSpanOptions) {
		if opts.Tags == nil {
			opts.Tags = map[string]interface{}{}
		}
		opts.Tags[ext.Resource] = r
	})
}

// Type specifies the type of service: web, rpc, cache, db, etc
func Type(t string) StartSpanOption {
	return spanOptionFunc(func(opts *opentracing.StartSpanOptions) {
		if opts.Tags == nil {
			opts.Tags = map[string]interface{}{}
		}
		opts.Tags[ext.Type] = t
	})
}

// Service overrides the name of the service
func Service(s string) StartSpanOption {
	return spanOptionFunc(func(opts *opentracing.StartSpanOptions) {
		if opts.Tags == nil {
			opts.Tags = map[string]interface{}{}
		}
		opts.Tags[ext.Service] = s
	})
}
