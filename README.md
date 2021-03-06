# datadog

[![GoDoc](https://godoc.org/github.com/savaki/datadog?status.svg)](https://godoc.org/github.com/savaki/datadog)
[![Build Status](https://travis-ci.org/savaki/datadog.svg?branch=master)](https://travis-ci.org/savaki/datadog)

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
		datadog.WithHost("10.0.0.1"), 
		datadog.WithPort("8200"), 
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

#### Specifying Datadog Resource

Unlike the opentracing default, datadog has a notion of resource.  Resource can be
defined in two ways:

```go
span := opentracing.StartSpan("operation_name", datadog.Resource("/"))
```

```go
span := opentracing.StartSpan("operation_name")
span.SetTag(ext.Resource, "/foo")
```

#### Specifying Datadog service type

Datadog has a number of default service, types: web, cache, db, and rpc.  By default,
the datadog tracer defaults the type to web.  To override this you can do either of
the following:

```go
span := opentracing.StartSpan("operation_name", datadog.Type(datadog.TypeRPC))
```

```go
span := opentracing.StartSpan("operation_name")
span.SetTag(ext.Type, "custom")
```

#### Handling Errors

Errors can be propagated to datadog in a couple ways:

***Via Tag:***

```go
span := opentracing.StartSpan("operation_name")
span.SetTag(ext.Error, error)
```

***Via LogFields:***

```go
span := opentracing.StartSpan("operation_name")
span.LogFields(log.Error(err))
```

***Via LogKV:***

```go
span := opentracing.StartSpan("operation_name")
span.LogKV("err", err)
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

## Logging

By default, datadog logs to os.Stdout via the ```datadog.Stdout``` logger.  

#### Using a custom logger

You can use a custom logger by specifying WithLogger when you construct 
the datadog tracer.

For example:

```go

func main() {
	tracer, _ := datadog.New("service", datadog.WithLogger(datadog.Stderr))
	defer tracer.Close()
}
```

## Todo 

- [ ] allow direct posts of traces to datadog e.g. agent-free tracer
- [ ] integration datadog log api
- [ ] allow direct posts of logs to datadog
