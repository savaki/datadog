package datadog

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/ugorji/go/codec"
)

type traceEncoder interface {
	io.Reader
	Encode(groups [][]*Trace) error
	ContentType() string
}

type msgpackEncoder struct {
	buffer  *bytes.Buffer
	encoder *codec.Encoder
}

func newMsgpackEncoder() *msgpackEncoder {
	buffer := bytes.NewBuffer(make([]byte, 0, 4096))
	return &msgpackEncoder{
		buffer:  buffer,
		encoder: codec.NewEncoder(buffer, &mh),
	}
}

func (e *msgpackEncoder) Encode(groups [][]*Trace) error {
	e.buffer.Reset()
	return e.encoder.Encode(groups)
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

func newJsonEncoder() *jsonEncoder {
	buffer := bytes.NewBuffer(make([]byte, 0, 4096))
	return &jsonEncoder{
		buffer:  buffer,
		encoder: json.NewEncoder(buffer),
	}
}

func (e *jsonEncoder) Encode(groups [][]*Trace) error {
	e.buffer.Reset()
	return e.encoder.Encode(groups)
}

func (e *jsonEncoder) Read(data []byte) (int, error) {
	return e.buffer.Read(data)
}

func (e *jsonEncoder) ContentType() string {
	return "application/json"
}
