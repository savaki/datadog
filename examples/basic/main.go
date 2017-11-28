package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/savaki/datadog"
)

func main() {
	tracer, _ := datadog.New("service")
	defer tracer.Close()

	opentracing.SetGlobalTracer(tracer)

	span := opentracing.StartSpan("sample")
	defer span.Finish()
}
