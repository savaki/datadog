package datadog

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/ugorji/go/codec"
)

type traceEncoder interface {
	io.Reader
	Encode(traces [][]*Trace) error
	ContentType() string
}

type msgpackEncoder struct {
	buffer  *bytes.Buffer
	encoder *codec.Encoder
}

func (e *msgpackEncoder) Encode(traces [][]*Trace) error {
	e.buffer.Reset()
	return e.encoder.Encode(traces)
}

func (e *msgpackEncoder) Read(data []byte) (int, error) {
	return e.buffer.Read(data)
}

func (e *msgpackEncoder) ContentType() string {
	return "application/msgpack"
}

type jsonEncoder struct {
	buffer  *bytes.Buffer
	encoder *json.Encoder
}

func (e *jsonEncoder) Encode(traces [][]*Trace) error {
	e.buffer.Reset()
	return e.encoder.Encode(traces)
}

func (e *jsonEncoder) Read(data []byte) (int, error) {
	return e.buffer.Read(data)
}

func (e *jsonEncoder) ContentType() string {
	return "application/json"
}
