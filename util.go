package datadog

import (
	"crypto/rand"
	"encoding/binary"
	"sync/atomic"
)

var (
	idCounter   uint64
	idIncrement uint64
)

func init() {
	// Set spanIDCounter and spanIDIncrement to random values.  nextSpanID will
	// return an arithmetic progression using these values, skipping zero.  We set
	// the LSB of spanIDIncrement to 1, so that the cycle length is 2^64.
	binary.Read(rand.Reader, binary.LittleEndian, &idCounter)
	binary.Read(rand.Reader, binary.LittleEndian, &idIncrement)
	idIncrement |= 1
}

// nextID returns a new ID.  It will never return zero.
func nextID() uint64 {
	var id uint64
	for id == 0 {
		id = atomic.AddUint64(&idCounter, idIncrement)
	}
	return id
}
