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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

const (
	// EnvAgentHost specifies optional environment property to specify datadog agent host
	EnvAgentHost = "DATADOG_AGENT_HOST"

	// EnvAgentPort specifies optional environment property to specify datadog agent port
	EnvAgentPort = "DATADOG_AGENT_PORT"
)

const (
	// FlushInterval contains the default length of time between flushes
	FlushInterval = time.Second * 3

	// DefaultBufSize contains the number of Spans to cache
	DefaultThreshold = 8192
)

const (
	// TypeWeb indicates span handled by web server
	TypeWeb = "web"

	// TypeRPC indicates span handled by rpc server
	TypeRPC = "rpc"

	// TypeDB indicates span handles a DB connection
	TypeDB = "db"

	// TypeCache indicates span handles a Cache
	TypeCache = "cache"
)

// Tracer is a simple, thin interface for Span creation and SpanContext
// propagation.
type Tracer struct {
	// service holds the default name of the service being run
	service string

	logger  Logger
	baggage map[string]string

	// now contains the function to calculate when now is
	now func() int64

	// logSpans indicates that spans should be recorded to the logger
	logSpans bool

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	queue  *queue
}

// Create, start, and return a new Span with the given `operationName` and
// incorporate the given StartSpanOption `opts`. (Note that `opts` borrows
// from the "functional options" pattern, per
// http://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis)
//
// A Span with no SpanReference options (e.g., opentracing.ChildOf() or
// opentracing.FollowsFrom()) becomes the root of its own trace.
//
// Examples:
//
//     var tracer opentracing.Tracer = ...
//
//     // The root-span case:
//     sp := tracer.StartSpan("GetFeed")
//
//     // The vanilla child span case:
//     sp := tracer.StartSpan(
//         "GetFeed",
//         opentracing.ChildOf(parentSpan.Context()))
//
//     // All the bells and whistles:
//     sp := tracer.StartSpan(
//         "GetFeed",
//         opentracing.ChildOf(parentSpan.Context()),
//         opentracing.Tag{"user_agent", loggedReq.UserAgent},
//         opentracing.StartTime(loggedReq.Timestamp),
//     )
//
func (t *Tracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	span := spanPool.Get().(*Span)
	span.tracer = t
	span.startedAt = t.now()
	span.operationName = operationName

	options := opentracing.StartSpanOptions{}
	for _, opt := range opts {
		opt.Apply(&options)
	}

loop:
	for _, ref := range options.References {
		switch ref.Type {
		case opentracing.ChildOfRef, opentracing.FollowsFromRef:
			if parent, ok := ref.ReferencedContext.(*Span); ok {
				span.traceID = parent.traceID
				span.parentSpanID = parent.spanID
				span.spanID = nextID()
			}
			break loop
		}
	}

	if span.traceID == 0 {
		span.traceID = nextID()
		span.spanID = nextID()
	}

	// set root baggage
	for k, v := range t.baggage {
		span.SetBaggageItem(k, v)
	}

	// set baggage from parent span
	for _, ref := range options.References {
		ref.ReferencedContext.ForeachBaggageItem(func(k, v string) bool {
			span.SetBaggageItem(k, v)
			return true
		})
	}

	for k, v := range options.Tags {
		span.SetTag(k, v)
	}

	return span
}

// Inject() takes the `sm` SpanContext instance and injects it for
// propagation within `carrier`. The actual type of `carrier` depends on
// the value of `format`.
//
// OpenTracing defines a common set of `format` values (see BuiltinFormat),
// and each has an expected carrier type.
//
// Other packages may declare their own `format` values, much like the keys
// used by `context.Context` (see
// https://godoc.org/golang.org/x/net/context#WithValue).
//
// Example usage (sans error handling):
//
//     carrier := opentracing.HTTPHeadersCarrier(httpReq.Header)
//     err := tracer.Inject(
//         span.Context(),
//         opentracing.HTTPHeaders,
//         carrier)
//
// NOTE: All opentracing.Tracer implementations MUST support all
// BuiltinFormats.
//
// Implementations may return opentracing.ErrUnsupportedFormat if `format`
// is not supported by (or not known by) the implementation.
//
// Implementations may return opentracing.ErrInvalidCarrier or any other
// implementation-specific error if the format is supported but injection
// fails anyway.
//
// See Tracer.Extract().
func (t *Tracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	span, ok := sm.(*Span)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}

	if carrier == nil {
		return opentracing.ErrInvalidCarrier
	}

	data, err := span.MarshalJSON()
	if err != nil {
		return opentracing.ErrSpanContextCorrupted
	}

	if format == opentracing.Binary {
		w, ok := carrier.(io.Writer)
		if !ok {
			return opentracing.ErrInvalidCarrier
		}
		io.WriteString(w, string(data))

	} else if format == opentracing.TextMap || format == opentracing.HTTPHeaders {
		m, ok := carrier.(opentracing.TextMapWriter)
		if !ok {
			return opentracing.ErrInvalidCarrier
		}
		m.Set(httpHeader, string(data))

	} else {
		return opentracing.ErrUnsupportedFormat
	}

	return nil
}

func (t *Tracer) push(span *Span, finishedAt int64) {
	if finishedAt == 0 {
		finishedAt = t.now()
	}

	service := span.service
	if service == "" {
		service = t.service
	}
	resource := span.resource
	if resource == "" {
		resource = span.operationName
	}
	typ := span.typ
	if typ == "" {
		typ = TypeWeb
	}

	trace := tracePool.Get().(*Trace)
	trace.TraceID = span.traceID
	trace.SpanID = span.spanID
	trace.Name = span.operationName
	trace.Resource = resource
	trace.Service = service
	trace.Type = typ
	trace.Start = span.startedAt
	trace.Duration = finishedAt - span.startedAt
	trace.ParentSpanID = span.parentSpanID
	trace.Error = span.hasError

	if trace.Meta != nil {
		for k, v := range span.baggage {
			trace.Meta[k] = v
		}
		for k, v := range span.tags {
			trace.Meta[k] = v
		}
	}

	t.queue.Push(trace)
}

// LogFields allows logging outside the scope of a span
func (t *Tracer) LogFields(fields ...log.Field) {
	ctx := logContext{baggage: t.baggage}
	t.logger.Log(ctx, fields...)
}

func extract(data []byte) (*Span, error) {
	span := spanPool.Get().(*Span)
	if err := json.Unmarshal(data, span); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	return span, nil
}

func extractBinary(carrier interface{}) (*Span, error) {
	r, ok := carrier.(io.Reader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	return extract(data)
}

func extractTextMap(carrier interface{}) (*Span, error) {
	var span *Span

	m, ok := carrier.(opentracing.TextMapReader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}

	fn := func(k, v string) error {
		if k == httpHeader {
			v, err := extract([]byte(v))
			if err == nil {
				return opentracing.ErrSpanContextCorrupted
			}
			span = v
		}
		return nil
	}

	if err := m.ForeachKey(fn); err != nil {
		return nil, err
	}

	if span == nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}

	return span, nil
}

func extractHTTPHeaders(carrier interface{}) (*Span, error) {
	var span *Span

	m, ok := carrier.(opentracing.HTTPHeadersCarrier)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}

	fn := func(k, v string) error {
		if k == httpHeader {
			v, err := extract([]byte(v))
			if err != nil {
				return opentracing.ErrSpanContextCorrupted
			}
			span = v
		}
		return nil
	}

	if err := m.ForeachKey(fn); err != nil {
		return nil, err
	}

	if span == nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}

	return span, nil
}

// Extract() returns a SpanContext instance given `format` and `carrier`.
//
// OpenTracing defines a common set of `format` values (see BuiltinFormat),
// and each has an expected carrier type.
//
// Other packages may declare their own `format` values, much like the keys
// used by `context.Context` (see
// https://godoc.org/golang.org/x/net/context#WithValue).
//
// Example usage (with StartSpan):
//
//
//     carrier := opentracing.HTTPHeadersCarrier(httpReq.Header)
//     clientContext, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
//
//     // ... assuming the ultimate goal here is to resume the trace with a
//     // server-side Span:
//     var serverSpan opentracing.Span
//     if err == nil {
//         span = tracer.StartSpan(
//             rpcMethodName, ext.RPCServerOption(clientContext))
//     } else {
//         span = tracer.StartSpan(rpcMethodName)
//     }
//
//
// NOTE: All opentracing.Tracer implementations MUST support all
// BuiltinFormats.
//
// Return values:
//  - A successful Extract returns a SpanContext instance and a nil error
//  - If there was simply no SpanContext to extract in `carrier`, Extract()
//    returns (nil, opentracing.ErrSpanContextNotFound)
//  - If `format` is unsupported or unrecognized, Extract() returns (nil,
//    opentracing.ErrUnsupportedFormat)
//  - If there are more fundamental problems with the `carrier` object,
//    Extract() may return opentracing.ErrInvalidCarrier,
//    opentracing.ErrSpanContextCorrupted, or implementation-specific
//    errors.
//
// See Tracer.Inject().
func (t *Tracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	var span *Span
	var err error

	if format == opentracing.Binary {
		span, err = extractBinary(carrier)
		if err != nil {
			return nil, err
		}

	} else if format == opentracing.TextMap {
		span, err = extractTextMap(carrier)
		if err != nil {
			return nil, err
		}

	} else if format == opentracing.HTTPHeaders {
		span, err = extractHTTPHeaders(carrier)
		if err != nil {
			return nil, err
		}

	} else {
		return nil, fmt.Errorf("unhandled format, %v", format)
	}

	return span, nil
}

func (t *Tracer) Close() error {
	t.cancel()
	<-t.done

	if closer, ok := interface{}(t.now).(io.Closer); ok {
		closer.Close()
	}

	return nil
}

func (t *Tracer) Flush() {
	t.queue.Flush()
}

func (t *Tracer) run() {
	defer close(t.done)

	timer := time.NewTicker(FlushInterval)
	defer timer.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return

		case <-timer.C:
			t.queue.Flush()
		}
	}
}

// Options contains configuration parameters
type options struct {
	host        string
	port        string
	logger      Logger
	baggage     map[string]string
	threshold   int
	transporter Transporter
	timeFunc    func() int64
	logSpans    bool
	debug       io.Writer
}

// Option defines a functional configuration
type Option interface {
	Apply(*options)
}

type optionFunc func(*options)

func (fn optionFunc) Apply(opt *options) {
	fn(opt)
}

// WithLogger allows the logger to be configured
func WithLogger(logger Logger) Option {
	return optionFunc(func(opt *options) {
		opt.logger = logger
	})
}

// WithLogger allows the logger to be configured
func WithLoggerFunc(logger LoggerFunc) Option {
	return WithLogger(logger)
}

// WithBaggageItem allows root baggage to be specified that will propagate to all spans
func WithBaggageItem(key, value string) Option {
	return optionFunc(func(opts *options) {
		if opts.baggage == nil {
			opts.baggage = map[string]string{}
		}
		opts.baggage[key] = value
	})
}

// WithBufSize configures the numbers of Spans to cache
func WithBufSize(n int) Option {
	return optionFunc(func(opt *options) {
		opt.threshold = n
	})
}

// WithHost configures the target host
func WithHost(host string) Option {
	return optionFunc(func(opts *options) {
		opts.host = host
	})
}

// WithPort configures a target host
func WithPort(port string) Option {
	return optionFunc(func(opts *options) {
		opts.port = port
	})
}

func WithECSHost() Option {
	return optionFunc(func(opts *options) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
		defer cancel()

		url := "http://169.254.169.254/latest/meta-data/local-ipv4"
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.WithContext(ctx)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to connect to instance metadata host, %v\n", url)
			return
		}
		defer resp.Body.Close()

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, "unable to read host info from instance metadata")
			return
		}

		opts.host = strings.TrimSpace(string(data))
	})
}

// WithTimeFunc allows for custom calculations of time.Now().UnixNano()
func WithTimeFunc(fn func() int64) Option {
	return optionFunc(func(opts *options) {
		opts.timeFunc = fn
	})
}

func WithTransporter(transporter Transporter) Option {
	return optionFunc(func(opts *options) {
		opts.transporter = transporter
	})
}

// WithNop is provided primarily for testing.  No traces will be published to datadog.
func WithNop() Option {
	return optionFunc(func(opts *options) {
		opts.transporter = nopTransporter
	})
}

func WithLogSpans() Option {
	return optionFunc(func(opts *options) {
		opts.logSpans = true
	})
}

func getOrElse(envKey, defaultValue string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}

	return defaultValue
}

// New constructs a new datadog tracer for the specified service.
// See https://docs.datadoghq.com/tracing/api/
func New(service string, opts ...Option) *Tracer {
	options := &options{
		host:      getOrElse(EnvAgentHost, "localhost"),
		port:      getOrElse(EnvAgentPort, "8126"),
		threshold: DefaultThreshold,
		logger:    LoggerFunc(func(logContext LogContext, fields ...log.Field) {}),
		timeFunc:  func() int64 { return time.Now().UnixNano() },
		debug:     ioutil.Discard,
	}
	for _, opt := range opts {
		opt.Apply(options)
	}

	transporter := options.transporter
	if transporter == nil {
		transporter = newTransporter(options.debug, options.host, options.port)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tracer := &Tracer{
		service:  service,
		logger:   options.logger,
		baggage:  options.baggage,
		now:      options.timeFunc,
		logSpans: options.logSpans,
		queue:    newQueue(options.threshold, transporter),
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}

	go tracer.run()

	return tracer
}
