// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
)

var _ = Describe("Customization utils", func() {
	Context("IsPending", func() {
		var extraConfig []vimtypes.BaseOptionValue
		var pending bool

		BeforeEach(func() {
			extraConfig = nil
		})

		JustBeforeEach(func() {
			pending = vmlifecycle.IsCustomizationPendingExtraConfig(extraConfig)
		})

		Context("Empty ExtraConfig", func() {
			It("not pending", func() {
				Expect(pending).To(BeFalse())
			})
		})

		Context("ExtraConfig with pending key", func() {
			BeforeEach(func() {
				extraConfig = append(extraConfig, &vimtypes.OptionValue{
					Key:   constants.GOSCPendingExtraConfigKey,
					Value: "/foo/bar",
				})
			})

			It("is pending", func() {
				Expect(pending).To(BeTrue())
			})
		})
	})
})

var _ = Describe("SanitizeConfigSpec", func() {
	var (
		inConfigSpec, outConfigSpec vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		inConfigSpec = vimtypes.VirtualMachineConfigSpec{}
	})

	JustBeforeEach(func() {
		outConfigSpec = vmlifecycle.SanitizeConfigSpec(inConfigSpec)
	})

	When("EC CloudInitGuestInfoUserdata", func() {
		BeforeEach(func() {
			inConfigSpec.ExtraConfig = append(inConfigSpec.ExtraConfig, &vimtypes.OptionValue{
				Key:   constants.CloudInitGuestInfoUserdata,
				Value: "value",
			})
		})

		It("redacts value", func() {
			Expect(inConfigSpec.ExtraConfig).To(HaveLen(1))
			Expect(inConfigSpec.ExtraConfig[0].GetOptionValue().Key).To(Equal(constants.CloudInitGuestInfoUserdata))
			Expect(inConfigSpec.ExtraConfig[0].GetOptionValue().Value).To(Equal("value"))

			Expect(outConfigSpec.ExtraConfig).To(HaveLen(1))
			Expect(outConfigSpec.ExtraConfig[0].GetOptionValue().Key).To(Equal(constants.CloudInitGuestInfoUserdata))
			Expect(outConfigSpec.ExtraConfig[0].GetOptionValue().Value).To(Equal("***"))
		})
	})

	When("vAppConfig user property", func() {
		BeforeEach(func() {
			inConfigSpec.VAppConfig = &vimtypes.VmConfigSpec{
				Property: []vimtypes.VAppPropertySpec{
					{
						Info: &vimtypes.VAppPropertyInfo{
							UserConfigurable: vimtypes.NewBool(true),
							Value:            "value",
						},
					},
				},
			}
		})

		It("redacts value", func() {
			vmConfigSpec := inConfigSpec.VAppConfig.GetVmConfigSpec()
			Expect(vmConfigSpec).ToNot(BeNil())
			Expect(vmConfigSpec.Property).To(HaveLen(1))
			Expect(vmConfigSpec.Property[0].Info.Value).To(Equal("value"))

			vmConfigSpec = outConfigSpec.VAppConfig.GetVmConfigSpec()
			Expect(vmConfigSpec).ToNot(BeNil())
			Expect(vmConfigSpec.Property).To(HaveLen(1))
			Expect(vmConfigSpec.Property[0].Info.Value).To(Equal("***"))
		})
	})
})

var _ = Describe("SanitizeCustomizationSpec", func() {
	var (
		inCustSpec, outCustSpec vimtypes.CustomizationSpec
	)

	BeforeEach(func() {
		inCustSpec = vimtypes.CustomizationSpec{}
	})

	JustBeforeEach(func() {
		outCustSpec = vmlifecycle.SanitizeCustomizationSpec(inCustSpec)
	})

	When("CustomizationCloudinitPrep", func() {
		BeforeEach(func() {
			inCustSpec.Identity = &internal.CustomizationCloudinitPrep{
				Metadata: "metadata",
				Userdata: "userdata",
			}
		})

		It("redacts userdata", func() {
			Expect(inCustSpec.Identity).ToNot(BeNil())
			c := inCustSpec.Identity.(*internal.CustomizationCloudinitPrep)
			Expect(c.Metadata).To(Equal("metadata"))
			Expect(c.Userdata).To(Equal("userdata"))

			Expect(outCustSpec.Identity).ToNot(BeNil())
			c = outCustSpec.Identity.(*internal.CustomizationCloudinitPrep)
			Expect(c.Metadata).To(Equal("metadata"))
			Expect(c.Userdata).To(Equal("***"))
		})
	})

	When("CustomizationLinuxPrep", func() {
		BeforeEach(func() {
			inCustSpec.Identity = &vimtypes.CustomizationLinuxPrep{
				Password: &vimtypes.CustomizationPassword{
					Value: "value",
				},
				ScriptText: "value",
			}
		})

		It("redacts fields", func() {
			Expect(inCustSpec.Identity).ToNot(BeNil())
			s := inCustSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
			Expect(s.Password).ToNot(BeNil())
			Expect(s.Password.Value).To(Equal("value"))
			Expect(s.ScriptText).To(Equal("value"))

			Expect(outCustSpec.Identity).ToNot(BeNil())
			s = outCustSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
			Expect(s.Password).ToNot(BeNil())
			Expect(s.Password.Value).To(Equal("***"))
			Expect(s.ScriptText).To(Equal("***"))
		})
	})

	When("CustomizationSysprepText", func() {
		BeforeEach(func() {
			inCustSpec.Identity = &vimtypes.CustomizationSysprepText{
				Value: "value",
			}
		})

		It("redacts value", func() {
			Expect(inCustSpec.Identity).ToNot(BeNil())
			s := inCustSpec.Identity.(*vimtypes.CustomizationSysprepText)
			Expect(s.Value).To(Equal("value"))

			Expect(outCustSpec.Identity).ToNot(BeNil())
			s = outCustSpec.Identity.(*vimtypes.CustomizationSysprepText)
			Expect(s.Value).To(Equal("***"))
		})
	})

	When("CustomizationSysprep", func() {
		BeforeEach(func() {
			inCustSpec.Identity = &vimtypes.CustomizationSysprep{
				GuiUnattended: vimtypes.CustomizationGuiUnattended{
					Password: &vimtypes.CustomizationPassword{
						Value: "value",
					},
					TimeZone: 42,
				},
				UserData: vimtypes.CustomizationUserData{},
				Identification: vimtypes.CustomizationIdentification{
					DomainAdmin: "admin",
					DomainAdminPassword: &vimtypes.CustomizationPassword{
						Value: "value",
					},
				},
				ScriptText: "value",
			}
		})

		It("redacts fields", func() {
			Expect(inCustSpec.Identity).ToNot(BeNil())
			s := inCustSpec.Identity.(*vimtypes.CustomizationSysprep)
			Expect(s.GuiUnattended.TimeZone).To(BeEquivalentTo(42))
			Expect(s.GuiUnattended.Password).ToNot(BeNil())
			Expect(s.GuiUnattended.Password.Value).To(Equal("value"))
			Expect(s.Identification.DomainAdmin).To(Equal("admin"))
			Expect(s.Identification.DomainAdminPassword).ToNot(BeNil())
			Expect(s.Identification.DomainAdminPassword.Value).To(Equal("value"))
			Expect(s.ScriptText).To(Equal("value"))

			Expect(outCustSpec.Identity).ToNot(BeNil())
			s = outCustSpec.Identity.(*vimtypes.CustomizationSysprep)
			Expect(s.GuiUnattended.TimeZone).To(BeEquivalentTo(42))
			Expect(s.GuiUnattended.Password).ToNot(BeNil())
			Expect(s.GuiUnattended.Password.Value).To(Equal("***"))
			Expect(s.Identification.DomainAdmin).To(Equal("admin"))
			Expect(s.Identification.DomainAdminPassword).ToNot(BeNil())
			Expect(s.Identification.DomainAdminPassword.Value).To(Equal("***"))
			Expect(s.ScriptText).To(Equal("***"))
		})
	})
})

// TODO: We should at least a few basic DoBootstrap() tests so we test the overall
// Reconfigure/Customize flow but the old code didn't.
