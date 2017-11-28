package datadog

import (
	"encoding/json"
	"testing"

	"github.com/tj/assert"
	"github.com/ugorji/go/codec"
)

func TestMsgpackEncoder(t *testing.T) {
	groups := [][]*Trace{
		{
			{
				TraceID: 123,
			},
		},
	}
	encoder := newMsgpackEncoder()
	assert.Nil(t, encoder.Encode(groups))

	var found [][]*Trace
	assert.Nil(t, codec.NewDecoder(encoder, &mh).Decode(&found))
	assert.EqualValues(t, groups, found)
}

func TestJsonEncoder(t *testing.T) {
	groups := [][]*Trace{
		{
			{
				TraceID: 123,
			},
		},
	}
	encoder := newJsonEncoder()
	assert.Nil(t, encoder.Encode(groups))

	var found [][]*Trace
	assert.Nil(t, json.NewDecoder(encoder).Decode(&found))
	assert.EqualValues(t, groups, found)

	assert.EqualValues(t, "application/json", encoder.ContentType())
}
