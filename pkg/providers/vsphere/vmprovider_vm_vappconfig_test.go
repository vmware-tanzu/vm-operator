// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func vAppConfigExpressionTests() {
	Describe("NormalizeVAppConfigExpressionProperties", func() {
		It("is a no-op when config spec is nil", func() {
			vsphere.NormalizeVAppConfigExpressionProperties(nil)
		})

		It("is a no-op when VAppConfig is nil", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			Expect(spec.VAppConfig).To(BeNil())
		})

		It("is a no-op when VmConfigSpec has no properties", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			Expect(spec.VAppConfig.GetVmConfigSpec().Property).To(BeEmpty())
		})

		It("converts expression type to string and sets UserConfigurable", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{
						{
							Info: &vimtypes.VAppPropertyInfo{
								Id:           "net-expression",
								Type:         "expression",
								DefaultValue: "com.vmware.customization.network",
								UserConfigurable: ptr.To(false),
							},
						},
					},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			vac := spec.VAppConfig.GetVmConfigSpec()
			Expect(vac).ToNot(BeNil())
			Expect(vac.Property).To(HaveLen(1))
			Expect(vac.Property[0].Info.Type).To(Equal("string"))
			Expect(vac.Property[0].Info.DefaultValue).To(Equal(""))
			Expect(vac.Property[0].Info.UserConfigurable).ToNot(BeNil())
			Expect(*vac.Property[0].Info.UserConfigurable).To(BeTrue())
		})

		It("converts ip:network type to string and sets UserConfigurable", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{
						{
							Info: &vimtypes.VAppPropertyInfo{
								Id:           "ip-network",
								Type:         "ip:network",
								DefaultValue: "192.168.1.0/24",
								UserConfigurable: nil,
							},
						},
					},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			vac := spec.VAppConfig.GetVmConfigSpec()
			Expect(vac).ToNot(BeNil())
			Expect(vac.Property).To(HaveLen(1))
			Expect(vac.Property[0].Info.Type).To(Equal("string"))
			Expect(vac.Property[0].Info.DefaultValue).To(Equal(""))
			Expect(vac.Property[0].Info.UserConfigurable).ToNot(BeNil())
			Expect(*vac.Property[0].Info.UserConfigurable).To(BeTrue())
		})

		It("leaves other property types unchanged", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{
						{
							Info: &vimtypes.VAppPropertyInfo{
								Id: "string-prop", Type: "string",
								DefaultValue: "hello", UserConfigurable: ptr.To(true),
							},
						},
						{
							Info: &vimtypes.VAppPropertyInfo{
								Id: "int-prop", Type: "int",
								DefaultValue: "42", UserConfigurable: ptr.To(false),
							},
						},
					},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			vac := spec.VAppConfig.GetVmConfigSpec()
			Expect(vac.Property[0].Info.Type).To(Equal("string"))
			Expect(vac.Property[0].Info.DefaultValue).To(Equal("hello"))
			Expect(vac.Property[1].Info.Type).To(Equal("int"))
			Expect(vac.Property[1].Info.DefaultValue).To(Equal("42"))
		})

		It("converts only expression and ip:network in a mixed list", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{
						{Info: &vimtypes.VAppPropertyInfo{Id: "a", Type: "string", DefaultValue: "keep"}},
						{Info: &vimtypes.VAppPropertyInfo{Id: "b", Type: "expression", DefaultValue: "expr"}},
						{Info: &vimtypes.VAppPropertyInfo{Id: "c", Type: "ip:network", DefaultValue: "10.0.0.0/8"}},
						{Info: &vimtypes.VAppPropertyInfo{Id: "d", Type: "int", DefaultValue: "1"}},
					},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			vac := spec.VAppConfig.GetVmConfigSpec()
			Expect(vac.Property[0].Info.Type).To(Equal("string"))
			Expect(vac.Property[0].Info.DefaultValue).To(Equal("keep"))
			Expect(vac.Property[1].Info.Type).To(Equal("string"))
			Expect(vac.Property[1].Info.DefaultValue).To(Equal(""))
			Expect(*vac.Property[1].Info.UserConfigurable).To(BeTrue())
			Expect(vac.Property[2].Info.Type).To(Equal("string"))
			Expect(vac.Property[2].Info.DefaultValue).To(Equal(""))
			Expect(*vac.Property[2].Info.UserConfigurable).To(BeTrue())
			Expect(vac.Property[3].Info.Type).To(Equal("int"))
			Expect(vac.Property[3].Info.DefaultValue).To(Equal("1"))
		})

		It("skips properties with nil Info", func() {
			spec := &vimtypes.VirtualMachineConfigSpec{
				VAppConfig: &vimtypes.VmConfigSpec{
					Property: []vimtypes.VAppPropertySpec{
						{Info: nil},
						{Info: &vimtypes.VAppPropertyInfo{Id: "b", Type: "expression", DefaultValue: "x"}},
					},
				},
			}
			vsphere.NormalizeVAppConfigExpressionProperties(spec)
			vac := spec.VAppConfig.GetVmConfigSpec()
			Expect(vac.Property[0].Info).To(BeNil())
			Expect(vac.Property[1].Info.Type).To(Equal("string"))
			Expect(vac.Property[1].Info.DefaultValue).To(Equal(""))
		})
	})
}
