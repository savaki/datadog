package datadog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"io"

	"net"

	"github.com/opentracing/opentracing-go/log"
	"github.com/savaki/datadog"
	"github.com/savaki/datadog/ext"
	"github.com/tj/assert"
)

func TestWrapHandler(t *testing.T) {
	t.Run("200", func(t *testing.T) {
		tags := map[string]interface{}{}

		tracer := datadog.New("blah",
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				logContext.ForeachTag(func(key string, value interface{}) bool {
					tags[key] = value
					return true
				})
			}),
		)

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
			ext.Type:       datadog.TypeWeb,
			ext.Resource:   http.MethodGet,
			ext.HTTPMethod: http.MethodGet,
			ext.HTTPCode:   "200",
			ext.HTTPURL:    "/",
		}, tags)
	})

	t.Run("500 Internal Server Error", func(t *testing.T) {
		count := int32(0)
		tags := map[string]interface{}{}

		tracer := datadog.New("blah",
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				logContext.ForeachTag(func(key string, value interface{}) bool {
					tags[key] = value
					return true
				})
				count++
			}),
		)

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
		assert.NotNil(t, tags[ext.Error], "expected error to be set since status code 500")
	})

	t.Run("from client", func(t *testing.T) {
		l, err := net.Listen("tcp", "localhost:0")
		assert.Nil(t, err)
		defer l.Close()

		tracer := datadog.New("blah",
			datadog.WithLogSpans(),
			datadog.WithLoggerFunc(func(logContext datadog.LogContext, fields ...log.Field) {
				fmt.Println("eek")
			}),
		)

		fn := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			for k, values := range req.Header {
				fmt.Println(k, values)
			}
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
	})
}
