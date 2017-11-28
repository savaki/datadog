# datadog

[![GoDoc](https://godoc.org/github.com/savaki/datadog?status.svg)](https://godoc.org/github.com/savaki/datadog)
[![TravisCI](https://travis-ci.org/savaki/datadog.svg?branch=master)](https://travis-ci.org/savaki/datadog.svg?branch=master)

datadog opentracing implementation.  For more information on opentracing and
the terminology, take a look at the [OpenTracing Project](http://opentracing.io/).

Requirements:

* Go 1.7
* docker-dd-agent 

## Installation

```bash
go get github.com/savaki/datadog
```


#### Singleton initialization

The simplest starting point is to register datadog as the default
tracer.

```go
package main

import (
	"github.com/opentracing/opentracing-go"
	"github.com/savaki/datadog"
)

func main() {
	tracer, _ := datadog.New("service")
	defer tracer.Close()

	opentracing.SetGlobalTracer(tracer)

	// ...
}
```

Alternately, you can manage the reference to Tracer explicitly.

By default, the datadog tracer will connect to the datadog agent at localhost:8126

#### Specify the host and port of the datadog agent

```go
func main() {
	tracer, _ := datadog.New("service", 
		datadog.Host("10.0.0.1"), 
		datadog.Port("8200"), 
	) 
	defer tracer.Close()
}
```

#### From within an ECS container, specify a docker-dd-agent running on the host

```go
func main() {
	tracer, _ := datadog.New("service", datadog.WithECS())
	defer tracer.Close()
}
```

#### Starting an empty trace by creating a "root span"

It's always possible to create a "root" `Span` with no parent or other causal
reference.

```go
    func xyz() {
        ...
        sp := opentracing.StartSpan("operation_name")
        defer sp.Finish()
        ...
    }
```

#### Creating a (child) Span given an existing (parent) Span

```go
    func xyz(parentSpan opentracing.Span, ...) {
        ...
        sp := opentracing.StartSpan(
            "operation_name",
            opentracing.ChildOf(parentSpan.Context()))
        defer sp.Finish()
        ...
    }
```

#### Serializing over http

As a convenience, the datadog tracer provides a couple of convenience methods
above the standard opentracing Inject/Extract.  

Instrument an http client with the datadog tracer. 

```go
client := &http.Client{
	Transport: datadog.WrapRoundTripper(http.DefaultRoundTripper, tracer),
}
```

Instrument an http server with the datadog tracer. 

```go
var h http.Handler = ....
h = datadog.WrapHandler(h, tracer)
```

