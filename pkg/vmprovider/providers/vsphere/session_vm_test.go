// +build !integration

// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

var _ = Describe("Session VM", func() {

	Context("GetMergedvAppConfigSpec", func() {
		trueVar := true
		falseVar := false

		DescribeTable("calling GetMergedvAppConfigSpec",
			func(inProps map[string]string, vmProps []vimTypes.VAppPropertyInfo, expected *vimTypes.VmConfigSpec) {
				vAppConfigSpec := vsphere.GetMergedvAppConfigSpec(inProps, vmProps)
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
})
