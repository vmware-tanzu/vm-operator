// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/session"
)

var _ = Describe("Test Session Utils", func() {

	Context("GetMergedvAppConfigSpec", func() {
		trueVar := true
		falseVar := false

		DescribeTable("calling GetMergedvAppConfigSpec",
			func(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo, expected *vimTypes.VmConfigSpec) {
				vAppConfigSpec := session.GetMergedvAppConfigSpec(inProps, vmProps)
				if expected == nil {
					Expect(vAppConfigSpec).To(BeNil())
				} else {
					Expect(len(vAppConfigSpec.Property)).To(Equal(len(expected.Property)))
					for i := range vAppConfigSpec.Property {
						Expect(vAppConfigSpec.Property[i].Info.Key).To(Equal(expected.Property[i].Info.Key))
						Expect(vAppConfigSpec.Property[i].Info.Id).To(Equal(expected.Property[i].Info.Id))
						Expect(vAppConfigSpec.Property[i].Info.Value).To(Equal(expected.Property[i].Info.Value))
						Expect(vAppConfigSpec.Property[i].ArrayUpdateSpec.Operation).To(Equal(vimTypes.ArrayUpdateOperationEdit))
					}
					Expect(vAppConfigSpec.OvfEnvironmentTransport).To(HaveLen(1))
					Expect(vAppConfigSpec.OvfEnvironmentTransport[0]).To(Equal(session.OvfEnvironmentTransportGuestInfo))
				}
			},
			Entry("return nil for absent vm and input props",
				map[string]string{},
				[]vimTypes.VAppPropertyInfo{},
				nil,
			),
			Entry("return nil for non UserConfigurable vm props",
				map[string]string{
					"one-id": "one-override-value",
					"two-id": "two-override-value",
				},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value"},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &falseVar},
				},
				nil,
			),
			Entry("return nil for UserConfigurable vm props but no input props",
				map[string]string{},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value"},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &trueVar},
				},
				nil,
			),
			Entry("return valid vAppConfigSpec for setting mixed UserConfigurable props",
				map[string]string{
					"one-id":   "one-override-value",
					"two-id":   "two-override-value",
					"three-id": "three-override-value",
				},
				[]vimTypes.VAppPropertyInfo{
					{Key: 1, Id: "one-id", Value: "one-value", UserConfigurable: nil},
					{Key: 2, Id: "two-id", Value: "two-value", UserConfigurable: &trueVar},
					{Key: 3, Id: "three-id", Value: "three-value", UserConfigurable: &falseVar},
				},
				&vimTypes.VmConfigSpec{
					Property: []vimTypes.VAppPropertySpec{
						{Info: &vimTypes.VAppPropertyInfo{Key: 2, Id: "two-id", Value: "two-override-value"}},
					},
				},
			),
		)
	})

	Context("ExtraConfigToMap", func() {
		var (
			extraConfig    []vimTypes.BaseOptionValue
			extraConfigMap map[string]string
		)
		BeforeEach(func() {
			extraConfig = []vimTypes.BaseOptionValue{}
		})
		JustBeforeEach(func() {
			extraConfigMap = session.ExtraConfigToMap(extraConfig)
		})

		Context("Empty extraConfig", func() {
			It("Return empty map", func() {
				Expect(extraConfigMap).To(HaveLen(0))
			})
		})

		Context("With extraConfig", func() {
			BeforeEach(func() {
				extraConfig = append(extraConfig, &vimTypes.OptionValue{Key: "key1", Value: "value1"})
				extraConfig = append(extraConfig, &vimTypes.OptionValue{Key: "key2", Value: "value2"})
			})
			It("Return valid map", func() {
				Expect(extraConfigMap).To(HaveLen(2))
				Expect(extraConfigMap["key1"]).To(Equal("value1"))
				Expect(extraConfigMap["key2"]).To(Equal("value2"))
			})
		})
	})

	Context("MergeExtraConfig", func() {
		var (
			extraConfig []vimTypes.BaseOptionValue
			newMap      map[string]string
			merged      []vimTypes.BaseOptionValue
		)
		BeforeEach(func() {
			extraConfig = []vimTypes.BaseOptionValue{
				&vimTypes.OptionValue{Key: "existingkey1", Value: "existingvalue1"},
				&vimTypes.OptionValue{Key: "existingkey2", Value: "existingvalue2"},
			}
			newMap = map[string]string{}
		})
		JustBeforeEach(func() {
			merged = session.MergeExtraConfig(extraConfig, newMap)
		})

		Context("Empty newMap", func() {
			It("Return empty merged", func() {
				Expect(merged).To(BeEmpty())
			})
		})

		Context("NewMap with existing key", func() {
			BeforeEach(func() {
				newMap["existingkey1"] = "existingkey1"
			})
			It("Return empty merged", func() {
				Expect(merged).To(BeEmpty())
			})
		})

		Context("NewMap with new keys", func() {
			BeforeEach(func() {
				newMap["newkey1"] = "newvalue1"
				newMap["newkey2"] = "newvalue2"
			})
			It("Return merged map", func() {
				Expect(merged).To(HaveLen(2))
				mergedMap := session.ExtraConfigToMap(merged)
				Expect(mergedMap["newkey1"]).To(Equal("newvalue1"))
				Expect(mergedMap["newkey2"]).To(Equal("newvalue2"))
			})
		})
	})
})
