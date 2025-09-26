// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
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

var _ = Describe("DoBootstrap", func() {
	// Use a VM that vcsim creates for us.
	const vcVMName = "DC0_C0_RP0_VM0"

	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		testConfig builder.VCSimTestConfig

		bsArgs     vmlifecycle.BootstrapArgs
		bsErr      error
		vcVM       *object.VirtualMachine
		vmCtx      pkgctx.VirtualMachineContext
		configInfo *vimtypes.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		var err error

		testConfig = builder.VCSimTestConfig{}
		ctx = suite.NewTestContextForVCSim(testConfig)
		nsInfo = ctx.CreateWorkloadNamespace()

		vm := builder.DummyVirtualMachine()
		vm.Name = "bootstrap-test"
		vm.Namespace = nsInfo.Namespace

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		vcVM, err = ctx.Finder.VirtualMachine(ctx, vcVMName)
		Expect(err).ToNot(HaveOccurred())
		vmCtx.VM.Status.UniqueID = vcVM.Reference().Value
		task, err := vcVM.PowerOff(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(task.Wait(ctx)).To(Succeed())

		{
			// Just remove all EthernetCards to make GOSC happy, instead of having
			// to fake more data.
			devices, err := vcVM.Device(ctx)
			Expect(err).ToNot(HaveOccurred())

			var cs vimtypes.VirtualMachineConfigSpec
			cs.DeviceChange, err = devices.SelectByType(&vimtypes.VirtualEthernetCard{}).
				ConfigSpec(vimtypes.VirtualDeviceConfigSpecOperationRemove)
			Expect(err).ToNot(HaveOccurred())
			task, err := vcVM.Reconfigure(vmCtx, cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		}

		moVM := &mo.VirtualMachine{}
		Expect(vcVM.Properties(ctx, vcVM.Reference(), nil, moVM)).To(Succeed())
		configInfo = moVM.Config
	})

	JustBeforeEach(func() {
		bsErr = vmlifecycle.DoBootstrap(vmCtx, vcVM, configInfo, bsArgs)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vcVM = nil
		configInfo = nil
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("LinuxPrep", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
			}
		})

		It("Customizes", func() {
			Expect(bsErr).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
		})

		When("CustomizedAtNextPowerOn is false", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn = ptr.To(false)
			})

			It("Does not customize", func() {
				Expect(bsErr).ToNot(HaveOccurred())
				Expect(vmCtx.VM.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn).To(HaveValue(BeFalse()))
			})
		})

		When("CustomizedAtNextPowerOn is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn = ptr.To(true)
			})

			It("Customizes and toggles", func() {
				Expect(bsErr).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
				Expect(vmCtx.VM.Spec.Bootstrap.LinuxPrep.CustomizeAtNextPowerOn).To(HaveValue(BeFalse()))
			})
		})
	})

	Context("Sysprep", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{
					Sysprep: &vmopv1sysprep.Sysprep{},
				},
			}
		})

		It("Customizes", func() {
			Expect(bsErr).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
		})

		When("CustomizedAtNextPowerOn is false", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Sysprep.CustomizeAtNextPowerOn = ptr.To(false)
			})

			It("Does not customize", func() {
				Expect(bsErr).ToNot(HaveOccurred())
				Expect(vmCtx.VM.Spec.Bootstrap.Sysprep.CustomizeAtNextPowerOn).To(HaveValue(BeFalse()))
			})
		})

		When("CustomizedAtNextPowerOn is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Sysprep.CustomizeAtNextPowerOn = ptr.To(true)
			})

			It("Customizes and toggles", func() {
				Expect(bsErr).To(MatchError(vmlifecycle.ErrBootstrapCustomize))
				Expect(vmCtx.VM.Spec.Bootstrap.Sysprep.CustomizeAtNextPowerOn).To(HaveValue(BeFalse()))
			})
		})
	})
})
