// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package bitmask_test

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/bitmask"
)

type mask8 uint8

const (
	mask8Aay mask8 = 1 << iota
	mask8Bee
	mask8Cee

	_mask8ValMax = 1 << (iota - 1)
)

func (m mask8) MaxValue() mask8 {
	return _mask8ValMax
}

func (m mask8) StringValue() string {
	switch m {
	case mask8Aay:
		return "Aay"
	case mask8Bee:
		return "Bee"
	case mask8Cee:
		return "Cee"
	}
	return ""
}

type mask32 uint64

const (
	mask32Aay mask32 = 1 << iota
	mask32Bee
	mask32Cee

	_mask32ValMax = 1 << (iota - 1)
)

func (m mask32) MaxValue() mask32 {
	return _mask32ValMax
}

func (m mask32) StringValue() string {
	switch m {
	case mask32Aay:
		return "1"
	case mask32Bee:
		return "2"
	case mask32Cee:
		return "4"
	}
	return ""
}

func (m mask32) Write(w *bytes.Buffer, b mask32) {
	if w.Len() > 0 {
		w.WriteString("|")
	}
	fmt.Fprintf(w, "%d", b)
}

type bit64 uint64
type mask64 bit64

const (
	bit64Aay bit64 = 1 << iota
	bit64Bee
	bit64Cee

	_mask64ValMax = 1 << (iota - 1)
)

func (m mask64) MaxValue() mask64 {
	return _mask64ValMax
}

func (b bit64) String() string {
	switch b {
	case bit64Aay:
		return "1"
	case bit64Bee:
		return "2"
	case bit64Cee:
		return "4"
	}
	return ""
}

func (m mask64) StringValue() string {
	return bit64(m).String()
}

var _ = Context("Uint8", func() {
	DescribeTable("Has", getHasTable(mask8(0))...)
	DescribeTable("Set", getSetTable(mask8(0))...)
	DescribeTable("Unset", getUnsetTable(mask8(0))...)
	DescribeTable("String", getStringTable(mask8(0))...)
})

var _ = Describe("Uint32", func() {
	DescribeTable("Has", getHasTable(mask32(0))...)
	DescribeTable("Set", getSetTable(mask32(0))...)
	DescribeTable("Unset", getUnsetTable(mask32(0))...)
	DescribeTable(
		"String",
		func(a int, s string) {
			Expect(bitmask.String(mask32(a))).To(Equal(s))
		},
		Entry("0 = \"\"", 0, ""),
		Entry("a = \"1\"", 1, "1"),
		Entry("b = \"2\"", 2, "2"),
		Entry("c = \"4\"", 4, "4"),
		Entry("8 = \"\"", 8, ""),
		Entry("16 = \"\"", 16, ""),
		Entry("32 = \"\"", 32, ""),
		Entry("a|b = \"1|2\"", 1|2, "1|2"),
		Entry("a|b|c = \"1|2|4\"", 1|2|4, "1|2|4"),
		Entry("a|c = \"1|4\"", 1|4, "1|4"),
		Entry("b|c = \"2|4\"", 2|4, "2|4"),
		Entry("c|b = \"2|4\"", 4|2, "2|4"),
	)
})

var _ = Context("Uint64", func() {
	DescribeTable("Has", getHasTable(mask64(0))...)
	DescribeTable("Set", getSetTable(mask64(0))...)
	DescribeTable("Unset", getUnsetTable(mask64(0))...)
	DescribeTable("String", getStringTable(mask64(0))...)
})

func getHasTable[U bitmask.Bit, M bitmask.Bitmask[U]](_ M) []any {
	return []any{
		func(a, b int, expected bool) {
			Expect(bitmask.Has(M(a), M(b))).To(Equal(expected))
		},
		Entry("0,0 == false", 0, 0, false),
		Entry("a|b,a == true", 1|2, 1, true),
		Entry("a|b,b == true", 1|2, 2, true),
		Entry("a|b,c == false", 1|2, 4, false),
		Entry("a|b|c,a == true", 1|2|4, 1, true),
		Entry("a|b|c,b == true", 1|2|4, 2, true),
		Entry("a|b|c,c == true", 1|2|4, 4, true),
		Entry("a|b|c,a|b == true", 1|2|4, 1|2, true),
	}
}

func getSetTable[U bitmask.Bit, M bitmask.Bitmask[U]](_ M) []any {
	return []any{
		func(a, b, c int) {
			Expect(bitmask.Set(M(a), M(b))).To(Equal(M(c)))
		},
		Entry("0,0 = 0", 0, 0, 0),
		Entry("a,a = a", 1, 1, 1),
		Entry("a,b = a|b", 1, 2, 1|2),
		Entry("a|b,c = a|b|c", 1|2, 4, 1|2|4),
		Entry("a|b,b|c = a|b|c", 1|2, 2|4, 1|2|4),
	}
}

func getUnsetTable[U bitmask.Bit, M bitmask.Bitmask[U]](_ M) []any {
	return []any{
		func(a, b, c int) {
			Expect(bitmask.Unset(M(a), M(b))).To(Equal(M(c)))
		},
		Entry("0,0 = 0", 0, 0, 0),
		Entry("a,a = 0", 1, 1, 0),
		Entry("a,b = a", 1, 2, 1),
		Entry("a|b|c,b = a|b", 1|2|4, 2, 1|4),
		Entry("a|b|c,0 = a|b|c", 1|2|4, 0, 1|2|4),
		Entry("a|b|c,b|c = a", 1|2|4, 2|4, 1),
	}
}

func getStringTable[U bitmask.Bit, M bitmask.Bitmask[U]](_ M) []any {
	return []any{
		func(a int, s string) {
			Expect(bitmask.String(mask8(a))).To(Equal(s))
		},
		Entry("0 = \"\"", 0, ""),
		Entry("a = \"Aay\"", 1, "Aay"),
		Entry("b = \"Bee\"", 2, "Bee"),
		Entry("c = \"Cee\"", 4, "Cee"),
		Entry("8 = \"\"", 8, ""),
		Entry("16 = \"\"", 16, ""),
		Entry("32 = \"\"", 32, ""),
		Entry("a|b = \"AayAndBee\"", 1|2, "AayAndBee"),
		Entry("a|b|c = \"AayAndBeeAndCee\"", 1|2|4, "AayAndBeeAndCee"),
		Entry("a|c = \"AayAndCee\"", 1|4, "AayAndCee"),
		Entry("b|c = \"BeeAndCee\"", 2|4, "BeeAndCee"),
		Entry("c|b = \"BeeAndCee\"", 4|2, "BeeAndCee"),
	}
}
