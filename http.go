package datadog

import (
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/opentracing/opentracing-go"
	"github.com/savaki/datadog/ext"
)

const (
	httpHeader = "X-Datadog-Trace"
)

var (
	errRequestFailed = errors.New("request failed")
)

var (
	writers = &sync.Pool{
		New: func() interface{} {
			return &responseWriter{}
		},
	}
)

type responseWriter struct {
	target     http.ResponseWriter
	statusCode int
}

func (r *responseWriter) Header() http.Header {
	return r.target.Header()
}

func (r *responseWriter) WriteHeader(statusCode int) {
	r.target.WriteHeader(statusCode)
	r.statusCode = statusCode
}

func (r *responseWriter) Write(data []byte) (int, error) {
	return r.target.Write(data)
}

func WrapHandler(h http.Handler, tracer *Tracer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rw := writers.Get().(*responseWriter)
		rw.target = w

		var span opentracing.Span

		var t opentracing.Tracer = tracer
		if t == nil {
			t = opentracing.GlobalTracer()
		}
		if sc, err := t.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header)); err == nil {
			span = t.StartSpan("http.request", opentracing.ChildOf(sc))
		} else {
			span = t.StartSpan("http.request")
		}
		defer span.Finish()

		h.ServeHTTP(rw, req)

		span.SetTag(ext.Type, TypeWeb)
		span.SetTag(ext.Resource, req.Method)
		span.SetTag(ext.HTTPMethod, req.Method)
		span.SetTag(ext.HTTPURL, req.URL.Path)
		span.SetTag(ext.HTTPCode, strconv.Itoa(rw.statusCode))
		if rw.statusCode >= 500 && rw.statusCode < 600 {
			span.SetTag(ext.Error, errRequestFailed)
		}

		func() {
			rw.statusCode = 0
			rw.target = nil
			writers.Put(rw)
		}()
	})
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func WrapRoundTripper(rt http.RoundTripper, tracer *Tracer) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var t opentracing.Tracer = tracer
		if t == nil {
			t = opentracing.GlobalTracer()
		}

		span := t.StartSpan(req.URL.String())
		ctx := opentracing.ContextWithSpan(req.Context(), span)
		defer span.Finish()

		req = req.WithContext(ctx)

		carrier := opentracing.HTTPHeadersCarrier(req.Header)
		if err := t.Inject(span.Context(), opentracing.HTTPHeaders, carrier); err != nil {
			return nil, err
		}

		return rt.RoundTrip(req)
	})
}
