// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package obs

import (
	"crypto/rand"
	"encoding/binary"
	"time"
)

// RunID identifies one invocation of ship-it. It is 26 Crockford base32
// characters: a 48-bit millisecond timestamp followed by 80 random bits, the
// layout popularised by ULID, so IDs sort by creation time.
type RunID string

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewRunID returns a new, time-ordered run identifier.
func NewRunID() RunID { return newRunIDAt(time.Now()) }

func newRunIDAt(t time.Time) RunID {
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[:8], uint64(t.UnixMilli())<<16)
	_, _ = rand.Read(raw[6:]) // crypto/rand.Read never fails since Go 1.24
	// 128 bits encoded 5 bits at a time; the first character carries 3 bits.
	var out [26]byte
	hi := binary.BigEndian.Uint64(raw[:8])
	lo := binary.BigEndian.Uint64(raw[8:])
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&0x1F]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return RunID(out[:])
}
