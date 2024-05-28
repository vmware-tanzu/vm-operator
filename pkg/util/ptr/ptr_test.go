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
			Ω(ptr.To(in.(string))).To(Equal(expected.(*string)))
		case int32:
			Ω(ptr.To(in.(int32))).To(Equal(expected.(*int32)))
		case *byte:
			Ω(ptr.To(in.(*byte))).To(Equal(expected.(**byte)))
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
			Ω(ptr.Deref(in.(*string))).To(Equal(expected.(string)))
		case *int32:
			Ω(ptr.Deref(in.(*int32))).To(Equal(expected.(int32)))
		case **byte:
			Ω(ptr.Deref(in.(**byte))).To(Equal(expected.(*byte)))
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
			Ω(ptr.DerefWithDefault(in.(*string), def.(string))).To(Equal(expected.(string)))
		case *int32:
			Ω(ptr.DerefWithDefault(in.(*int32), def.(int32))).To(Equal(expected.(int32)))
		case **byte:
			Ω(ptr.DerefWithDefault(in.(**byte), def.(*byte))).To(Equal(expected.(*byte)))
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
			Ω(ptr.Equal(a.(*string), b.(*string))).To(Equal(expected))
		case *int32:
			Ω(ptr.Equal(a.(*int32), b.(*int32))).To(Equal(expected))
		case **byte:
			Ω(ptr.Equal(a.(**byte), b.(**byte))).To(Equal(expected))
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

var _ = DescribeTable(
	"Overwrite",
	func(dst any, src any, expectedResult bool, expectedValue any) {
		switch dst.(type) {
		case **string:
			Ω(ptr.Overwrite(dst.(**string), src.(*string))).To(Equal(expectedResult))
			if expectedValue == nil {
				Ω(dst.(**string)).To(BeNil())
			} else {
				Ω(**(dst.(**string))).To(Equal(expectedValue.(string)))
			}
		case **int32:
			Ω(ptr.Overwrite(dst.(**int32), src.(*int32))).To(Equal(expectedResult))
			if expectedValue == nil {
				Ω(dst.(**int32)).To(BeNil())
			} else {
				Ω(**(dst.(**int32))).To(Equal(expectedValue.(int32)))
			}
		case ***byte:
			Ω(ptr.Overwrite(dst.(***byte), src.(**byte))).To(Equal(expectedResult))
			if expectedValue == nil {
				Ω(dst.(***byte)).To(BeNil())
			} else {
				Ω(***(dst.(***byte))).To(Equal(expectedValue.(**byte)))
			}
		default:
			panic(fmt.Sprintf("unexpected type: %T", dst))
		}
	},
	Entry("**string - dst is nil", []**string{nil}[0], &[]string{"hello"}[0], false, nil),
	Entry("**string - dst is not nil, src is not empty", &[]*string{&[]string{""}[0]}[0], &[]string{"hello"}[0], true, "hello"),
	Entry("**string - dst is not empty, src is nil", &[]*string{&[]string{"hello"}[0]}[0], []*string{nil}[0], false, "hello"),
	Entry("**int32 - dst is not empty, src is not empty", &[]*int32{&[]int32{int32(1)}[0]}[0], &[]int32{int32(2)}[0], true, int32(2)),
)
