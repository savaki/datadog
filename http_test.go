package datadog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"io"

	"net"

	"sync/atomic"

	"github.com/opentracing/opentracing-go/log"
	"github.com/savaki/datadog"
	"github.com/savaki/datadog/ext"
	"github.com/tj/assert"
)

func TestWrapHandler(t *testing.T) {
	t.Run("200", func(t *testing.T) {
		tags := map[string]interface{}{}

		tracer, _ := datadog.New("blah",
			datadog.WithNop(),
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				logContext.ForeachTag(func(key string, value interface{}) bool {
					tags[key] = value
					return true
				})
			}),
		)
		defer tracer.Close()

		fn := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "ok")
		})
		h := datadog.WrapHandler(fn, tracer)

		// When
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		// Then
		assert.EqualValues(t, map[string]interface{}{
			ext.Resource:   "/",
			ext.Type:       "web",
			ext.HTTPMethod: http.MethodGet,
			ext.HTTPCode:   "200",
			ext.HTTPURL:    "/",
		}, tags)
	})

	t.Run("500 Internal Server Error", func(t *testing.T) {
		count := int32(0)
		tags := map[string]interface{}{}

		tracer, _ := datadog.New("blah",
			datadog.WithNop(),
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				logContext.ForeachTag(func(key string, value interface{}) bool {
					tags[key] = value
					return true
				})
				count++
			}),
		)
		defer tracer.Close()

		fn := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		h := datadog.WrapHandler(fn, tracer)

		// When
		req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		// Then
		assert.EqualValues(t, 1, count)
		assert.NotNil(t, tags[ext.ErrorMsg], "expected error to be set since status code 500")
		assert.NotNil(t, tags[ext.ErrorStack], "expected error to be set since status code 500")
		assert.NotNil(t, tags[ext.ErrorType], "expected error to be set since status code 500")
	})

	t.Run("from client", func(t *testing.T) {
		l, err := net.Listen("tcp", "localhost:0")
		assert.Nil(t, err)
		defer l.Close()

		count := int32(0)
		tracer, _ := datadog.New("blah",
			datadog.WithNop(),
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				atomic.AddInt32(&count, 1)
			}),
		)
		defer tracer.Close()

		fn := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "ok")
		})
		h := datadog.WrapHandler(fn, tracer)
		go http.Serve(l, h)

		url := fmt.Sprintf("http://%v", l.Addr())
		req, err := http.NewRequest(http.MethodGet, url, nil)
		assert.Nil(t, err)

		rt := datadog.WrapRoundTripper(http.DefaultTransport, tracer)
		_, err = rt.RoundTrip(req)
		assert.Nil(t, err)
		assert.EqualValues(t, 2, atomic.LoadInt32(&count))
	})
}
