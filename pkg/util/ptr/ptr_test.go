// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ptr_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = DescribeTable(
	"To",
	func(in any, expected any) {
		switch in.(type) {
		case string:
			Expect(ptr.To(in.(string))).To(Equal(expected.(*string)))
		case int32:
			Expect(ptr.To(in.(int32))).To(Equal(expected.(*int32)))
		case *byte:
			Expect(ptr.To(in.(*byte))).To(Equal(expected.(**byte)))
		default:
			panic(fmt.Sprintf("unexpected type: %T", in))
		}
	},
	Entry("*string", "hello", &[]string{"hello"}[0]),
	Entry("*int32", int32(1), &[]int32{int32(1)}[0]),
	Entry("**byte", &[]byte{byte(2)}[0], &[]*byte{&[]byte{byte(2)}[0]}[0]),
)

var _ = DescribeTable(
	"Deref",
	func(in any, expected any) {
		switch in.(type) {
		case *string:
			Expect(ptr.Deref(in.(*string))).To(Equal(expected.(string)))
		case *int32:
			Expect(ptr.Deref(in.(*int32))).To(Equal(expected.(int32)))
		case **byte:
			Expect(ptr.Deref(in.(**byte))).To(Equal(expected.(*byte)))
		default:
			panic(fmt.Sprintf("unexpected type: %T", in))
		}
	},
	Entry("*string", &[]string{"hello"}[0], "hello"),
	Entry("*int32", &[]int32{int32(1)}[0], int32(1)),
	Entry("**byte", &[]*byte{&[]byte{byte(2)}[0]}[0], &[]byte{byte(2)}[0]),
)

var _ = DescribeTable(
	"DerefWithDefault",
	func(in any, def any, expected any) {
		switch in.(type) {
		case *string:
			Expect(ptr.DerefWithDefault(in.(*string), def.(string))).To(Equal(expected.(string)))
		case *int32:
			Expect(ptr.DerefWithDefault(in.(*int32), def.(int32))).To(Equal(expected.(int32)))
		case **byte:
			Expect(ptr.DerefWithDefault(in.(**byte), def.(*byte))).To(Equal(expected.(*byte)))
		default:
			panic(fmt.Sprintf("unexpected type: %T", in))
		}
	},
	Entry("nil *string", []*string{nil}[0], "default", "default"),
	Entry("non-nil *int32", &[]int32{int32(1)}[0], int32(0), int32(1)),
	Entry("nil **byte", &[]*byte{nil}[0], []*byte{nil}[0], []*byte{nil}[0]),
)

var (
	ptrToPtrByte1 = &[]*byte{&[]byte{byte(1)}[0]}[0]
)

var _ = DescribeTable(
	"Equal",
	func(a any, b any, expected bool) {
		switch a.(type) {
		case *string:
			Expect(ptr.Equal(a.(*string), b.(*string))).To(Equal(expected))
		case *int32:
			Expect(ptr.Equal(a.(*int32), b.(*int32))).To(Equal(expected))
		case **byte:
			Expect(ptr.Equal(a.(**byte), b.(**byte))).To(Equal(expected))
		default:
			panic(fmt.Sprintf("unexpected type: %T", a))
		}
	},
	Entry("*string - equ - same val", &[]string{"hello"}[0], &[]string{"hello"}[0], true),
	Entry("*string - equ - both nil", []*string{nil}[0], []*string{nil}[0], true),
	Entry("*string - neq - diff val", &[]string{"hello"}[0], &[]string{"world"}[0], false),
	Entry("*string - neq - one nil", &[]string{"hello"}[0], []*string{nil}[0], false),
	Entry("*int32 - equ - same val", &[]int32{1}[0], &[]int32{1}[0], true),
	Entry("*int32 - neq - diff val", &[]int32{1}[0], &[]int32{2}[0], false),
	Entry("**byte - equ - same ptr", ptrToPtrByte1, ptrToPtrByte1, true),
	Entry("**byte - equ - diff ptr", &[]*byte{&[]byte{byte(1)}[0]}[0], &[]*byte{&[]byte{byte(1)}[0]}[0], false),
	Entry("**byte - neq - diff val", &[]*byte{&[]byte{byte(1)}[0]}[0], &[]*byte{&[]byte{byte(2)}[0]}[0], false),
)

const dstIsNil = "dst is nil"

var _ = DescribeTable(
	"Overwrite",
	func(dst any, src any, expectedValue any, expectPanic bool) {
		switch src.(type) {
		case string:
			fn := func() {
				ptr.Overwrite(dst.(*string), src.(string))
			}
			if expectPanic {
				Expect(fn).Should(PanicWith(dstIsNil))
			} else {
				fn()
				if expectedValue == nil {
					Expect(dst.(*string)).To(BeNil())
				} else {
					Expect(*(dst.(*string))).To(Equal(expectedValue.(string)))
				}
			}
		case *string:
			fn := func() {
				ptr.Overwrite(dst.(**string), src.(*string))
			}
			if expectPanic {
				Expect(fn).Should(PanicWith(dstIsNil))
			} else {
				fn()
				if expectedValue == nil {
					Expect(dst.(**string)).To(BeNil())
				} else {
					Expect(**(dst.(**string))).To(Equal(expectedValue.(string)))
				}
			}
		case int32:
			fn := func() {
				ptr.Overwrite(dst.(*int32), src.(int32))
			}
			if expectPanic {
				Expect(fn).Should(PanicWith(dstIsNil))
			} else {
				fn()
				if expectedValue == nil {
					Expect(dst.(*int32)).To(BeNil())
				} else {
					Expect(*(dst.(*int32))).To(Equal(expectedValue.(int32)))
				}
			}
		case *int32:
			fn := func() {
				ptr.Overwrite(dst.(**int32), src.(*int32))
			}
			if expectPanic {
				Expect(fn).Should(PanicWith(dstIsNil))
			} else {
				fn()
				if expectedValue == nil {
					Expect(dst.(**int32)).To(BeNil())
				} else {
					Expect(**(dst.(**int32))).To(Equal(expectedValue.(int32)))
				}
			}
		case **byte:
			fn := func() {
				ptr.Overwrite(dst.(***byte), src.(**byte))
			}
			if expectPanic {
				Expect(fn).Should(PanicWith(dstIsNil))
			} else {
				fn()
				if expectedValue == nil {
					Expect(dst.(***byte)).To(BeNil())
				} else {
					Expect(***(dst.(***byte))).To(Equal(expectedValue.(byte)))
				}
			}
		default:
			panic(fmt.Sprintf("unexpected type: %T", dst))
		}
	},
	Entry("string - dst is nil", []*string{nil}[0], "hello", nil, true),
	Entry("*string - dst is nil", []**string{nil}[0], &[]string{"hello"}[0], nil, true),
	Entry("string - dst is not nil, src is empty", &[]string{"hello"}[0], "", "", false),
	Entry("*string - dst is not nil, src is not empty", &[]*string{&[]string{""}[0]}[0], &[]string{"hello"}[0], "hello", false),
	Entry("int32 - dst is nil", []*int32{nil}[0], int32(1), nil, true),
	Entry("*int32 - dst is nil", []**int32{nil}[0], &[]int32{int32(1)}[0], nil, true),
	Entry("int32 - dst is not empty, src is empty", &[]int32{int32(1)}[0], int32(0), int32(0), false),
	Entry("*int32 - dst is not empty, src is not empty", &[]*int32{&[]int32{int32(1)}[0]}[0], &[]int32{int32(2)}[0], int32(2), false),
	Entry("**byte - dst is nil", []***byte{nil}[0], &[]*byte{&[]byte{byte(1)}[0]}[0], nil, true),
	Entry("**byte - dst is not empty, src is not empty", &[]**byte{&[]*byte{&[]byte{byte(1)}[0]}[0]}[0], &[]*byte{&[]byte{byte(2)}[0]}[0], byte(2), false),
	Entry("**byte - dst is not empty, src is empty", &[]**byte{&[]*byte{&[]byte{byte(1)}[0]}[0]}[0], &[]*byte{&[]byte{byte(0)}[0]}[0], byte(0), false),
)
