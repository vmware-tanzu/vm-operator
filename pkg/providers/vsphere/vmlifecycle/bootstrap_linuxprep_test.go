// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/linuxprep"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("LinuxPrep Bootstrap", func() {
	const (
		macAddr = "43:AB:B4:1B:7E:87"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *vimtypes.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		configInfo = &vimtypes.VirtualMachineConfigInfo{}
		bsArgs.Data = map[string]string{}
	})

	AfterEach(func() {
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("BootStrapLinuxPrep", func() {

		var (
			configSpec         *vimtypes.VirtualMachineConfigSpec
			custSpec           *vimtypes.CustomizationSpec
			customizationLatch *bool
			err                error

			vmCtx          pkgctx.VirtualMachineContext
			vm             *vmopv1.VirtualMachine
			linuxPrepSpec  *vmopv1.VirtualMachineBootstrapLinuxPrepSpec
			vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		)

		BeforeEach(func() {
			linuxPrepSpec = &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{}
			vAppConfigSpec = nil

			bsArgs.HostName = "my-hostname"
			bsArgs.SearchSuffixes = []string{"suffix1", "suffix2"}
			bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
				{
					MacAddress: macAddr,
					IPConfigs: []network.NetworkInterfaceIPConfig{
						{
							Gateway: "192.168.1.1",
							IPCIDR:  "192.168.1.10/24",
							IsIPv4:  true,
						},
					},
				},
			}

			vm = &vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "linux-prep-bootstrap-test",
					Namespace: "test-ns",
				},
				Spec: vmopv1.VirtualMachineSpec{
					PowerState: vmopv1.VirtualMachinePowerStateOn,
				},
			}

			vmCtx = pkgctx.VirtualMachineContext{
				Context: pkgcfg.NewContext(),
				Logger:  suite.GetLogger(),
				VM:      vm,
				MoVM: mo.VirtualMachine{
					Runtime: vimtypes.VirtualMachineRuntimeInfo{
						PowerState: vimtypes.VirtualMachinePowerStatePoweredOff,
					},
				},
			}
		})

		JustBeforeEach(func() {
			configSpec, custSpec, customizationLatch, err = vmlifecycle.BootStrapLinuxPrep(
				vmCtx,
				configInfo,
				linuxPrepSpec,
				vAppConfigSpec,
				&bsArgs,
			)
		})

		It("should return expected customization spec", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(configSpec).To(BeNil())
			Expect(customizationLatch).To(BeNil())

			Expect(custSpec).ToNot(BeNil())
			Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(bsArgs.DNSServers))
			Expect(custSpec.GlobalIPSettings.DnsSuffixList).To(Equal(bsArgs.SearchSuffixes))

			linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
			hostName := linuxSpec.HostName.(*vimtypes.CustomizationFixedName).Name
			Expect(hostName).To(Equal(bsArgs.HostName))
			Expect(linuxSpec.TimeZone).To(Equal(linuxPrepSpec.TimeZone))
			Expect(linuxSpec.HwClockUTC).To(Equal(linuxPrepSpec.HardwareClockIsUTC))
			Expect(linuxSpec.Password).To(BeNil())
			Expect(linuxSpec.ResetPassword).To(BeNil())
			Expect(linuxSpec.ScriptText).To(BeEmpty())

			Expect(custSpec.NicSettingMap).To(HaveLen(len(bsArgs.NetworkResults.Results)))
			Expect(custSpec.NicSettingMap[0].MacAddress).To(Equal(macAddr))
		})

		When("GuestCustomizationVCDParity is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
					config.Features.GuestCustomizationVCDParity = true
				})
			})

			When("Reset password is specified", func() {
				BeforeEach(func() {
					linuxPrepSpec.ExpirePasswordAfterNextLogin = true
				})

				It("should return expected customization spec", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
					Expect(linuxSpec.ResetPassword).To(HaveValue(BeTrue()))
				})
			})

			When("Password is specified", func() {
				BeforeEach(func() {
					linuxPrepSpec.Password = &common.PasswordSecretKeySelector{}
					bsArgs.LinuxPrep = &linuxprep.SecretData{
						Password: "my-new-password",
					}
				})

				It("should return expected customization spec", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
					Expect(linuxSpec.Password).ToNot(BeNil())
					Expect(linuxSpec.Password.Value).To(Equal("my-new-password"))
					Expect(linuxSpec.Password.PlainText).To(BeTrue())
				})
			})

			When("ScriptText is specified", func() {
				const text = "#/bin/sh\necho hello"

				BeforeEach(func() {
					linuxPrepSpec.ScriptText = &common.ValueOrSecretKeySelector{}
					bsArgs.LinuxPrep = &linuxprep.SecretData{
						ScriptText: text,
					}
				})

				It("should return expected customization spec", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
					Expect(linuxSpec.ScriptText).To(Equal(text))
				})
			})

			When("VCFA ID annotation is specified", func() {
				BeforeEach(func() {
					if vm.Annotations == nil {
						vm.Annotations = make(map[string]string)
					}
					vm.Annotations[constants.VCFAIDAnnotationKey] = "foobar"
				})

				It("should return expected customization spec", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					linuxSpec := custSpec.Identity.(*vimtypes.CustomizationLinuxPrep)
					Expect(linuxSpec.ExtraConfig).To(HaveLen(1))
					optVal := linuxSpec.ExtraConfig[0].GetOptionValue()
					Expect(optVal).ToNot(BeNil())
					Expect(optVal.Key).To(Equal(vmlifecycle.GOSCVCFAHashID))
					Expect(optVal.Value).To(Equal("foobar"))
				})
			})
		})

		Context("when has power on customization latch", func() {
			Context("latch is false", func() {
				BeforeEach(func() {
					linuxPrepSpec.CustomizeAtNextPowerOn = vimtypes.NewBool(false)
				})

				It("should return no specs", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(configSpec).To(BeNil())
					Expect(custSpec).To(BeNil())
					Expect(customizationLatch).To(Equal(linuxPrepSpec.CustomizeAtNextPowerOn))
				})
			})

			Context("latch is true", func() {
				BeforeEach(func() {
					linuxPrepSpec.CustomizeAtNextPowerOn = vimtypes.NewBool(true)
				})

				It("should return customization spec", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(configSpec).To(BeNil())
					Expect(custSpec).ToNot(BeNil())
					Expect(customizationLatch).To(Equal(linuxPrepSpec.CustomizeAtNextPowerOn))
				})
			})
		})

		Context("when has vAppConfig", func() {
			const key, value = "fooKey", "fooValue"

			BeforeEach(func() {
				configInfo.VAppConfig = &vimtypes.VmConfigInfo{
					Property: []vimtypes.VAppPropertyInfo{
						{
							Id:               key,
							Value:            "should-change",
							UserConfigurable: ptr.To(true),
						},
					},
				}

				vAppConfigSpec = &vmopv1.VirtualMachineBootstrapVAppConfigSpec{
					Properties: []common.KeyValueOrSecretKeySelectorPair{
						{
							Key:   key,
							Value: common.ValueOrSecretKeySelector{Value: ptr.To(value)},
						},
					},
				}
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(custSpec).ToNot(BeNil())

				Expect(configSpec).ToNot(BeNil())
				Expect(configSpec.VAppConfig).ToNot(BeNil())
				vmCs := configSpec.VAppConfig.GetVmConfigSpec()
				Expect(vmCs).ToNot(BeNil())
				Expect(vmCs.Property).To(HaveLen(1))
				Expect(vmCs.Property[0].Info).ToNot(BeNil())
				Expect(vmCs.Property[0].Info.Id).To(Equal(key))
				Expect(vmCs.Property[0].Info.Value).To(Equal(value))
			})
		})
	})
})
