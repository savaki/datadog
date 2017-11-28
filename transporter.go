package datadog

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/ugorji/go/codec"
)

var (
	mh codec.MsgpackHandle
)

var (
	tracePool = &sync.Pool{
		New: func() interface{} {
			return &Trace{
				Meta: map[string]string{},
			}
		},
	}

	encoderPool = &sync.Pool{
		New: func() interface{} {
			return newMsgpackEncoder()
		},
	}
)

type Transporter interface {
	Publish(groups [][]*Trace) error
}

type transporterFunc func(groups [][]*Trace) error

func (fn transporterFunc) Publish(groups [][]*Trace) error {
	return fn(groups)
}

var (
	nopTransporter = transporterFunc(func(groups [][]*Trace) error {
		release(groups)
		return nil
	})
)

func newTransporter(output io.Writer, host, port string) transporterFunc {
	if host == "" || port == "" {
		fmt.Fprintln(output, "host and port not set.  Traces will not be sent")
		return nopTransporter
	}

	u := fmt.Sprintf("http://%v:%v/v0.3/traces", host, port)
	if _, err := http.NewRequest(http.MethodGet, u, nil); err != nil {
		fmt.Fprintf(output, "Datadog - invalid host:port, %v:%v.  Traces will not be sent\n", host, port)
		return nopTransporter
	}

	return func(groups [][]*Trace) error {
		encoder := encoderPool.Get().(traceEncoder)
		if err := encoder.Encode(groups); err != nil {
			return errors.Wrapf(err, "unable to encode trace groups")
		}

		req, _ := http.NewRequest(http.MethodPut, u, encoder)
		req.Header.Set("Content-Type", encoder.ContentType())

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return errors.Wrapf(err, "PUT %v failed", u)
		}
		defer resp.Body.Close()
		io.Copy(output, resp.Body)

		return nil
	}
}

func withRetry(t Transporter, retries int, delay time.Duration) transporterFunc {
	return func(groups [][]*Trace) error {
		for attempt := 0; attempt < retries; attempt++ {
			if err := t.Publish(groups); err == nil {
				return nil
			}

			time.Sleep(delay)
		}
		return errors.Errorf("unable to post traces to datadog after %v attempts", retries)
	}
}
