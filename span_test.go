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
	"github.com/savaki/datadog/ext"
	"github.com/tj/assert"
)

func captureLogs(target map[string]interface{}) datadog.LoggerFunc {
	return func(logContext datadog.LogContext, fields ...log.Field) {
		logContext.ForeachBaggageItem(func(k, v string) bool {
			target[k] = v
			return true
		})
		logContext.ForeachTag(func(k string, v interface{}) bool {
			target[k] = v
			return true
		})
		for _, f := range fields {
			target[f.Key()] = f.Value()
		}
	}
}

func TestBaggage(t *testing.T) {
	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
	)
	defer tracer.Close()
	a := tracer.StartSpan("a")
	defer a.Finish()
	a.SetBaggageItem("hello", "world")

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	defer b.Finish()

	assert.EqualValues(t, "world", b.BaggageItem("hello"))
}

func TestSpan_SetBaggageItem(t *testing.T) {
	baggage := map[string]interface{}{}
	fn := datadog.LoggerFunc(captureLogs(baggage))

	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
		datadog.WithLogger(fn),
	)
	defer tracer.Close()

	a := tracer.StartSpan("a")
	defer a.Finish()

	a.SetBaggageItem("key", "value")
	a.LogFields()

	assert.Equal(t, map[string]interface{}{"key": "value"}, baggage)
}

func TestSpan_SetTag(t *testing.T) {
	t.Run("string tag", func(t *testing.T) {
		tags := map[string]interface{}{}
		fn := datadog.LoggerFunc(captureLogs(tags))

		tracer, _ := datadog.New("blah",
			datadog.WithNop(),
			datadog.WithLogger(fn),
		)
		defer tracer.Close()

		a := tracer.StartSpan("a")
		defer a.Finish()

		a.SetTag("hello", "world")
		a.LogFields()

		assert.Equal(t, map[string]interface{}{"hello": "world"}, tags)
	})
}

func TestSpan(t *testing.T) {
	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
	)
	defer tracer.Close()

	a := tracer.StartSpan("a")
	defer a.Finish()

	b := tracer.StartSpan("b", opentracing.ChildOf(a.Context()))
	defer b.Finish()
}

func TestOpentracing(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		tracer, _ := datadog.New("blah", datadog.WithNop())
		defer tracer.Close()

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

		tracer, _ := datadog.New(apiKey,
			datadog.WithNop(),
		)
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
	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
	)
	defer tracer.Close()

	for i := 0; i < t.N; i++ {
		a := tracer.StartSpan("a")
		a.Finish()
	}
}

func BenchmarkConcurrentTracing(b *testing.B) {
	tracer, _ := datadog.New("blah",
		datadog.WithNop(),
	)
	defer tracer.Close()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		go func() {
			span := tracer.StartSpan("parent", datadog.Resource("/"))
			defer span.Finish()

			for i := 0; i < 10; i++ {
				tracer.StartSpan("child", opentracing.ChildOf(span.Context())).Finish()
			}
		}()
	}
}

func TestInject(t *testing.T) {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		t.SkipNow()
	}

	tracer, _ := datadog.New(apiKey, datadog.WithNop())
	defer tracer.Close()

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

func TestResource(t *testing.T) {
	name := "foo"
	opts := &opentracing.StartSpanOptions{}
	datadog.Resource(name).Apply(opts)
	assert.Equal(t, name, opts.Tags[ext.Resource])
}

func TestType(t *testing.T) {
	name := "foo"
	opts := &opentracing.StartSpanOptions{}
	datadog.Type(name).Apply(opts)
	assert.Equal(t, name, opts.Tags[ext.Type])
}

func TestService(t *testing.T) {
	name := "foo"
	opts := &opentracing.StartSpanOptions{}
	datadog.Service(name).Apply(opts)
	assert.Equal(t, name, opts.Tags[ext.Service])
}
