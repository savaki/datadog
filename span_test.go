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

package datadog_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/savaki/datadog"
	"github.com/tj/assert"
)

func TestBaggage(t *testing.T) {
	tracer := datadog.New("blah")
	a := tracer.StartSpan("a")
	defer a.Finish()
	a.SetBaggageItem("hello", "world")

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	defer b.Finish()

	assert.EqualValues(t, "world", b.BaggageItem("hello"))
}

func TestSpan_SetBaggageItem(t *testing.T) {
	baggage := map[string]string{}
	fn := datadog.LoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
		ctx.ForeachBaggageItem(func(key, value string) bool {
			baggage[key] = value
			return true
		})
		assert.Len(t, fields, 0)
	})

	tracer := datadog.New("blah", datadog.WithLogger(fn))
	a := tracer.StartSpan("a")
	defer a.Finish()

	a.SetBaggageItem("key", "value")
	a.LogFields()

	assert.Equal(t, map[string]string{"key": "value"}, baggage)
}

func TestSpan_SetTag(t *testing.T) {
	tags := map[string]interface{}{}
	fn := datadog.LoggerFunc(func(ctx datadog.LogContext, fields ...log.Field) {
		ctx.ForeachTag(func(key string, value interface{}) bool {
			tags[key] = value
			return true
		})
	})

	tracer := datadog.New("blah", datadog.WithLogger(fn))
	a := tracer.StartSpan("a")
	defer a.Finish()

	a.SetTag("hello", "world")
	a.LogFields()

	assert.Equal(t, map[string]interface{}{"hello": "world"}, tags)
}

func TestSpan(t *testing.T) {
	tracer := datadog.New("blah")
	a := tracer.StartSpan("a")
	defer a.Finish()

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	defer b.Finish()
}

func TestOpentracing(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		tracer := datadog.New("blah")
		opentracing.SetGlobalTracer(tracer)

		a, ctx := opentracing.StartSpanFromContext(context.Background(), "a")
		defer a.Finish()

		b, ctx := opentracing.StartSpanFromContext(ctx, "b")
		defer b.Finish()
	})

	t.Run("http", func(t *testing.T) {
		apiKey := os.Getenv("API_KEY")
		if apiKey == "" {
			t.SkipNow()
		}

		tracer := datadog.New(apiKey)
		defer tracer.Close()

		req := httptest.NewRequest(http.MethodGet, "http://localhost", nil)
		recorder := httptest.NewRecorder()
		recorder.WriteHeader(http.StatusOK)

		opentracing.SetGlobalTracer(tracer)
		span := opentracing.StartSpan("http")
		span.SetTag("http.request", req)
		span.SetTag("http.Response", recorder.Result())
		time.Sleep(time.Millisecond * 250)
		span.Finish()
	})
}

func BenchmarkSpan(t *testing.B) {
	tracer := datadog.New("blah")

	for i := 0; i < t.N; i++ {
		a := tracer.StartSpan("a")
		a.Finish()
	}
}

func TestInject(t *testing.T) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		t.SkipNow()
	}

	tracer := datadog.New(apiKey)

	t.Run("Binary", func(t *testing.T) {
		local := tracer.StartSpan("Binary - local")
		local.SetBaggageItem("hello", "world")
		local.LogFields(log.String("message", "local message"))

		time.Sleep(time.Millisecond * 250)

		// Inject -> Extract
		buf := bytes.NewBuffer(nil)
		assert.Nil(t, tracer.Inject(local.Context(), opentracing.Binary, buf))

		remoteContext, err := tracer.Extract(opentracing.Binary, buf)
		assert.Nil(t, err)

		// create remote span
		remote := tracer.StartSpan("remote", opentracing.SpanReference{
			Type:              opentracing.ChildOfRef,
			ReferencedContext: remoteContext,
		})
		time.Sleep(time.Millisecond * 250)
		remote.SetBaggageItem("a", "b")
		remote.LogFields(log.String("message", "remote message"))
		remote.Finish()
		time.Sleep(time.Millisecond * 250)

		local.Finish()
	})

	t.Run("TextMap", func(t *testing.T) {
		local := tracer.StartSpan("TextMap - local")
		local.SetBaggageItem("hello", "world")
		local.LogFields(log.String("message", "local message"))

		time.Sleep(time.Millisecond * 250)

		// Inject -> Extract
		carrier := opentracing.TextMapCarrier{}
		assert.Nil(t, tracer.Inject(local.Context(), opentracing.TextMap, carrier))

		remoteContext, err := tracer.Extract(opentracing.TextMap, carrier)
		assert.Nil(t, err)

		// create remote span
		remote := tracer.StartSpan("remote", opentracing.SpanReference{
			Type:              opentracing.ChildOfRef,
			ReferencedContext: remoteContext,
		})
		time.Sleep(time.Millisecond * 250)
		remote.SetBaggageItem("a", "b")
		remote.LogFields(log.String("message", "remote message"))
		remote.Finish()
		time.Sleep(time.Millisecond * 250)

		local.Finish()
	})

	t.Run("HTTPHeaders", func(t *testing.T) {
		local := tracer.StartSpan("HTTPHeaders - local")
		local.SetBaggageItem("hello", "world")
		local.LogFields(log.String("message", "local message"))

		time.Sleep(time.Millisecond * 250)

		// Inject -> Extract
		header := http.Header{}
		assert.Nil(t, tracer.Inject(local.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(header)))

		remoteContext, err := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(header))
		assert.Nil(t, err)

		// create remote span
		remote := tracer.StartSpan("remote", opentracing.SpanReference{
			Type:              opentracing.ChildOfRef,
			ReferencedContext: remoteContext,
		})
		time.Sleep(time.Millisecond * 250)
		remote.SetBaggageItem("a", "b")
		remote.LogFields(log.String("message", "remote message"))
		remote.Finish()
		time.Sleep(time.Millisecond * 250)

		local.Finish()
	})

	time.Sleep(time.Second * 3)
}
