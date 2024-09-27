// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package bitmask

import (
	"bytes"
)

// Bit is a type constraint that includes all unsigned integers.
type Bit interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64

	// StringValue returns the string-ified value for the bit.
	StringValue() string
}

// Bitmask is a type constraint that can be used as a bitmask.
type Bitmask[T Bit] interface {
	Bit

	// MaxValue returns the maximum, valid bit that may be set in the mask.
	MaxValue() T
}

// Has returns true if (a & b) > 0.
func Has[B Bit, M Bitmask[B]](a, b M) bool {
	return (a & b) > 0
}

// Set returns a new mask of a | b.
func Set[B Bit, M Bitmask[B]](a, b M) M {
	return a | b
}

// Unset returns a new mask of a &^ b.
func Unset[B Bit, M Bitmask[B]](a, b M) M {
	return a &^ b
}

// String returns the stringified version of the mask.
func String[B Bit, M Bitmask[B]](a M) string {
	var (
		b       B
		w       bytes.Buffer
		m       = a.MaxValue()
		writeFn = write[B, M]
	)

	if f, ok := ((any)(a)).(interface{ Write(*bytes.Buffer, M) }); ok {
		// If the bitmask implements Write(*bytes.Buffer, M, M), then use it.
		// Otherwise use write(*bytes.Buffer, M, M).
		writeFn = f.Write
	}

	for i := 0; ; i++ {
		if b = 1 << i; b > m {
			break
		}
		if Has(a, M(b)) {
			writeFn(&w, M(b))
		}
	}

	return w.String()
}

func write[B Bit, M Bitmask[B]](w *bytes.Buffer, b M) {
	if w.Len() > 0 {
		w.WriteString("And")
	}
	w.WriteString(b.StringValue())
}
