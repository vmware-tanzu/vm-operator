// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("EthernetExtraConfigPrefix", func() {
	DescribeTable("returns the correct ethernetX. prefix for a device key",
		func(deviceKey int32, expected string) {
			Expect(vmopv1util.EthernetExtraConfigPrefix(deviceKey)).To(Equal(expected))
		},
		Entry("key 4000 → ethernet0.", int32(4000), "ethernet0."),
		Entry("key 4001 → ethernet1.", int32(4001), "ethernet1."),
		Entry("key 4009 → ethernet9.", int32(4009), "ethernet9."),
	)
})

var _ = Describe("DecodeVMXFieldValue/*bool", func() {
	// boolField returns a fresh reflect.Value for a *bool pointer.
	boolField := func() reflect.Value {
		var s struct{ F *bool }
		return reflect.ValueOf(&s).Elem().Field(0)
	}

	DescribeTable("bool pointer field",
		func(raw string, wantNil bool, wantTrue bool) {
			rv := boolField()
			Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, raw)).To(Succeed())
			if wantNil {
				Expect(rv.IsNil()).To(BeTrue(),
					"raw %q: expected nil (auto sentinel)", raw)
			} else {
				Expect(rv.IsNil()).To(BeFalse(),
					"raw %q: expected non-nil", raw)
				Expect(rv.Elem().Bool()).To(Equal(wantTrue),
					"raw %q: unexpected bool value", raw)
			}
		},

		// True-ish values
		Entry("TRUE", "TRUE", false, true),
		Entry("true", "true", false, true),
		Entry("True", "True", false, true),
		Entry("1", "1", false, true),
		Entry("YES", "YES", false, true),
		Entry("yes", "yes", false, true),
		Entry("ON", "ON", false, true),
		Entry("on", "on", false, true),
		Entry("T", "T", false, true),
		Entry("t", "t", false, true),
		Entry("Y", "Y", false, true),
		Entry("y", "y", false, true),

		// False-ish values
		Entry("FALSE", "FALSE", false, false),
		Entry("false", "false", false, false),
		Entry("False", "False", false, false),
		Entry("0", "0", false, false),
		Entry("NO", "NO", false, false),
		Entry("no", "no", false, false),
		Entry("OFF", "OFF", false, false),
		Entry("off", "off", false, false),
		Entry("F", "F", false, false),
		Entry("f", "f", false, false),
		Entry("N", "N", false, false),
		Entry("n", "n", false, false),

		// Auto/dontcare sentinels — must leave pointer nil (nil = auto convention).
		Entry("auto", "auto", true, false),
		Entry("AUTO", "AUTO", true, false),
		Entry("Auto", "Auto", true, false),
		Entry("default", "default", true, false),
		Entry("DEFAULT", "DEFAULT", true, false),
		Entry("Default", "Default", true, false),
		Entry("dontcare", "dontcare", true, false),
		Entry("DONTCARE", "DONTCARE", true, false),
		Entry("DontCare", "DontCare", true, false),

		// Unrecognised value — default to false, no error.
		Entry("unknown string → false", "junk-value", false, false),
	)

	It("decodes into the given field", func() {
		var s struct{ F *bool }
		t := true
		s.F = &t
		rv := reflect.ValueOf(&s).Elem().Field(0)

		Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, "FALSE")).To(Succeed())
		Expect(rv.IsNil()).To(BeFalse())
		Expect(rv.Elem().Bool()).To(BeFalse())
	})
})

var _ = Describe("DecodeVMXFieldValue/*TxContextThreadingMode", func() {
	ctxField := func() reflect.Value {
		var s struct {
			F *vmopv1.TxContextThreadingMode
		}
		return reflect.ValueOf(&s).Elem().Field(0)
	}

	DescribeTable("decodes vSphere integer strings to spec constants or stores raw for unknowns",
		func(raw string, wantNil bool, wantVal vmopv1.TxContextThreadingMode) {
			rv := ctxField()
			Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, raw)).To(Succeed())
			if wantNil {
				Expect(rv.IsNil()).To(BeTrue(), "raw %q: expected nil (skip)", raw)
			} else {
				Expect(rv.IsNil()).To(BeFalse(), "raw %q: expected non-nil", raw)
				got := vmopv1.TxContextThreadingMode(rv.Elem().String())
				Expect(got).To(Equal(wantVal), "raw %q", raw)
			}
		},

		// Known vSphere integers → spec constants.
		Entry("1 → PerDevice", "1", false, vmopv1.TxContextThreadingModePerDevice),
		Entry("2 → PerVM", "2", false, vmopv1.TxContextThreadingModePerVM),
		Entry("3 → PerQueue", "3", false, vmopv1.TxContextThreadingModePerQueue),

		// Unknown single digits accepted as weak-enum raw values.
		Entry("5 → raw 5", "5", false, vmopv1.TxContextThreadingMode("5")),
		Entry("9 → raw 9", "9", false, vmopv1.TxContextThreadingMode("9")),

		// Invalid values (multi-digit, zero, non-numeric) → skip (nil).
		Entry("0 → nil", "0", true, vmopv1.TxContextThreadingMode("")),
		Entry("10 → nil", "10", true, vmopv1.TxContextThreadingMode("")),
		Entry("auto → nil", "auto", true, vmopv1.TxContextThreadingMode("")),
		Entry("unknown → nil", "perqueue", true, vmopv1.TxContextThreadingMode("")),
	)
})

var _ = Describe("DecodeVMXFieldValue/*CoalescingScheme", func() {
	schemeField := func() reflect.Value {
		var s struct{ F *vmopv1.CoalescingScheme }
		return reflect.ValueOf(&s).Elem().Field(0)
	}

	DescribeTable("decodes vSphere lowercase strings to spec constants or stores raw for unknowns",
		func(raw string, wantNil bool, wantVal vmopv1.CoalescingScheme) {
			rv := schemeField()
			Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, raw)).To(Succeed())
			if wantNil {
				Expect(rv.IsNil()).To(BeTrue(), "raw %q: expected nil (skip)", raw)
			} else {
				Expect(rv.IsNil()).To(BeFalse(), "raw %q: expected non-nil", raw)
				got := vmopv1.CoalescingScheme(rv.Elem().String())
				Expect(got).To(Equal(wantVal), "raw %q", raw)
			}
		},

		// Known vSphere values → spec constants.
		Entry("disabled → Disabled", "disabled", false, vmopv1.CoalescingSchemeDisabled),
		Entry("adapt → Adapt", "adapt", false, vmopv1.CoalescingSchemeAdapt),
		Entry("static → Static", "static", false, vmopv1.CoalescingSchemeStatic),
		Entry("rbc → RateBasedCoalescing", "rbc", false, vmopv1.CoalescingSchemeRateBasedCoalescing),

		// Unknown values stored raw if length < 128.
		Entry("future-mode stored raw", "future-mode", false, vmopv1.CoalescingScheme("future-mode")),

		// Values ≥ 128 chars are skipped.
		Entry("128-char string → nil", string(make([]byte, 128)), true, vmopv1.CoalescingScheme("")),
	)
})

var _ = Describe("DecodeVMXFieldValue/[]PNICQueueFeature", func() {
	featureField := func() reflect.Value {
		var s struct{ F []vmopv1.PNICQueueFeature }
		return reflect.ValueOf(&s).Elem().Field(0)
	}

	DescribeTable("decodes vSphere bitmask integer strings to []PNICQueueFeature",
		func(raw string, wantEmpty bool, wantFeatures []vmopv1.PNICQueueFeature) {
			rv := featureField()
			Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, raw)).To(Succeed())
			if wantEmpty {
				Expect(rv.Len()).To(Equal(0), "raw %q: expected empty/nil slice", raw)
			} else {
				got := make([]vmopv1.PNICQueueFeature, rv.Len())
				for i := range got {
					got[i] = vmopv1.PNICQueueFeature(rv.Index(i).String())
				}
				Expect(got).To(ConsistOf(wantFeatures), "raw %q", raw)
			}
		},

		// Zero / unparseable → empty slice (field not set).
		Entry("0 → empty", "0", true, nil),
		Entry("non-integer → empty", "auto", true, nil),
		Entry("empty string → empty", "", true, nil),

		// Known bits.
		Entry("1 → LargeReceiveOffload", "1", false,
			[]vmopv1.PNICQueueFeature{vmopv1.PNICQueueFeatureLargeReceiveOffload}),
		Entry("4 → ReceiveSideScaling", "4", false,
			[]vmopv1.PNICQueueFeature{vmopv1.PNICQueueFeatureReceiveSideScaling}),

		// Combined known bits.
		Entry("5 → LRO+RSS", "5", false,
			[]vmopv1.PNICQueueFeature{
				vmopv1.PNICQueueFeatureLargeReceiveOffload,
				vmopv1.PNICQueueFeatureReceiveSideScaling,
			}),

		// Unknown bit → decimal power-of-2 string.
		Entry("2 → raw \"2\"", "2", false,
			[]vmopv1.PNICQueueFeature{vmopv1.PNICQueueFeature("2")}),

		// Combined known + unknown bits.
		Entry("7 → LRO+2+RSS", "7", false,
			[]vmopv1.PNICQueueFeature{
				vmopv1.PNICQueueFeatureLargeReceiveOffload,
				vmopv1.PNICQueueFeature("2"),
				vmopv1.PNICQueueFeatureReceiveSideScaling,
			}),

		// Bit 15 (32768) — highest supported bit, stored as raw decimal.
		Entry("32768 → raw \"32768\"", "32768", false,
			[]vmopv1.PNICQueueFeature{vmopv1.PNICQueueFeature("32768")}),

		// Bits beyond 15 are silently dropped (MaxItems=16 constraint).
		Entry("65536 (bit 16) → empty", "65536", true, nil),
	)
})

var _ = Describe("DecodeVMXFieldValue/*UDPRSSMode", func() {
	udpRSSField := func() reflect.Value {
		var s struct{ F *vmopv1.UDPRSSMode }
		return reflect.ValueOf(&s).Elem().Field(0)
	}

	DescribeTable("decodes vSphere 1/2 integer encoding to UDPRSSMode",
		func(raw string, wantNil bool, wantVal vmopv1.UDPRSSMode) {
			rv := udpRSSField()
			Expect(vmopv1util.DecodeVMXFieldValue(context.Background(), rv, raw)).To(Succeed())
			if wantNil {
				Expect(rv.IsNil()).To(BeTrue(), "raw %q: expected nil", raw)
			} else {
				Expect(rv.IsNil()).To(BeFalse(), "raw %q: expected non-nil", raw)
				Expect(vmopv1.UDPRSSMode(rv.Elem().Bool())).To(Equal(wantVal), "raw %q", raw)
			}
		},

		// Valid vSphere values.
		Entry("1 → Enabled", "1", false, vmopv1.UDPRSSModeEnabled),
		Entry("2 → Disabled", "2", false, vmopv1.UDPRSSModeDisabled),

		// Auto/host-default sentinels → nil (hypervisor decides).
		Entry("auto → nil", "auto", true, vmopv1.UDPRSSMode(false)),
		Entry("DEFAULT → nil", "DEFAULT", true, vmopv1.UDPRSSMode(false)),

		// Any other value → nil (only 1/2 are valid for this field).
		Entry("TRUE → nil", "TRUE", true, vmopv1.UDPRSSMode(false)),
		Entry("FALSE → nil", "FALSE", true, vmopv1.UDPRSSMode(false)),
		Entry("junk → nil", "junk", true, vmopv1.UDPRSSMode(false)),
	)
})
