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
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha5/sysprep"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("SysPrep Bootstrap", func() {
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

	Context("BootStrapSysPrep", func() {
		const unattendXML = "dummy-unattend-xml"

		var (
			configSpec *vimtypes.VirtualMachineConfigSpec
			custSpec   *vimtypes.CustomizationSpec
			err        error

			vmCtx          pkgctx.VirtualMachineContext
			vm             *vmopv1.VirtualMachine
			sysPrepSpec    *vmopv1.VirtualMachineBootstrapSysprepSpec
			vAppConfigSpec *vmopv1.VirtualMachineBootstrapVAppConfigSpec
		)

		BeforeEach(func() {
			sysPrepSpec = &vmopv1.VirtualMachineBootstrapSysprepSpec{}
			vAppConfigSpec = nil

			bsArgs.Data["unattend"] = unattendXML
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
					Name:      "sys-prep-bootstrap-test",
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
			configSpec, custSpec, err = vmlifecycle.BootstrapSysPrep(
				vmCtx,
				configInfo,
				sysPrepSpec,
				vAppConfigSpec,
				&bsArgs,
			)
		})

		Context("No data", func() {
			It("returns error", func() {
				Expect(err).To(MatchError("no Sysprep data"))
			})
		})

		Context("Inlined Sysprep", func() {

			const (
				autoUsers      = int32(5)
				password       = "password_foo"
				domainPassword = "admin_password_foo"
				productID      = "product_id_foo"
				hostName       = "foo-win-vm"
			)

			BeforeEach(func() {
				bsArgs.DomainName = "foo.local"
				sysPrepSpec.Sysprep = &vmopv1sysprep.Sysprep{
					GUIUnattended: &vmopv1sysprep.GUIUnattended{
						AutoLogon:      true,
						AutoLogonCount: 2,
						Password: &vmopv1sysprep.PasswordSecretKeySelector{
							// omitting the name of the secret, since it does not get used
							// in this function
							Key: "pwd_key",
						},
						TimeZone: 4,
					},
					UserData: vmopv1sysprep.UserData{
						FullName:  "foo-bar",
						OrgName:   "foo-org",
						ProductID: &vmopv1sysprep.ProductIDSecretKeySelector{Key: "product_id_key"},
					},
					GUIRunOnce: &vmopv1sysprep.GUIRunOnce{
						Commands: []string{"blah", "boom"},
					},
					Identification: &vmopv1sysprep.Identification{
						DomainAdmin:         "[Foo/Administrator]",
						JoinWorkgroup:       "foo.local.wg",
						DomainAdminPassword: &vmopv1sysprep.DomainPasswordSecretKeySelector{Key: "admin_pwd_key"},
						DomainOU:            "OU=MyOu,DC=MyDom,DC=MyCompany,DC=com",
					},
					LicenseFilePrintData: &vmopv1sysprep.LicenseFilePrintData{
						AutoMode:  vmopv1sysprep.CustomizationLicenseDataModePerServer,
						AutoUsers: ptr.To(autoUsers),
					},
				}

				// secret data gets populated into the bootstrapArgs
				bsArgs.Sysprep = &sysprep.SecretData{
					ProductID:      productID,
					Password:       password,
					DomainPassword: domainPassword,
				}
				bsArgs.HostName = hostName
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(custSpec).ToNot(BeNil())

				sysPrep, ok := custSpec.Identity.(*vimtypes.CustomizationSysprep)
				Expect(ok).To(BeTrue())

				Expect(sysPrep.GuiUnattended.TimeZone).To(Equal(int32(4)))
				Expect(sysPrep.GuiUnattended.AutoLogonCount).To(Equal(int32(2)))
				Expect(sysPrep.GuiUnattended.AutoLogon).To(BeTrue())
				Expect(sysPrep.GuiUnattended.Password.Value).To(Equal(password))

				Expect(sysPrep.UserData.FullName).To(Equal("foo-bar"))
				Expect(sysPrep.UserData.OrgName).To(Equal("foo-org"))
				Expect(sysPrep.UserData.ProductId).To(Equal(productID))
				name, ok := sysPrep.UserData.ComputerName.(*vimtypes.CustomizationFixedName)
				Expect(ok).To(BeTrue())
				Expect(name.Name).To(Equal(hostName))

				Expect(sysPrep.GuiRunOnce.CommandList).To(HaveLen(2))

				Expect(sysPrep.Identification.DomainAdmin).To(Equal("[Foo/Administrator]"))
				Expect(sysPrep.Identification.JoinDomain).To(Equal("foo.local"))
				Expect(sysPrep.Identification.DomainAdminPassword.Value).To(Equal(domainPassword))
				Expect(sysPrep.Identification.DomainOU).To(Equal("OU=MyOu,DC=MyDom,DC=MyCompany,DC=com"))
				Expect(sysPrep.Identification.JoinWorkgroup).To(Equal("foo.local.wg"))

				Expect(sysPrep.LicenseFilePrintData.AutoMode).To(Equal(vimtypes.CustomizationLicenseDataModePerServer))
				Expect(sysPrep.LicenseFilePrintData.AutoUsers).To(Equal(autoUsers))

				Expect(sysPrep.ResetPassword).To(BeNil())
				Expect(sysPrep.ScriptText).To(BeEmpty())
				Expect(sysPrep.ExtraConfig).To(BeEmpty())
			})

			When("GuestCustomizationVCDParity is enabled", func() {
				BeforeEach(func() {
					pkgcfg.SetContext(vmCtx, func(config *pkgcfg.Config) {
						config.Features.GuestCustomizationVCDParity = true
					})
				})

				When("Reset password is specified", func() {
					BeforeEach(func() {
						sysPrepSpec.Sysprep.ExpirePasswordAfterNextLogin = true
					})

					It("should return expected customization spec", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(custSpec).ToNot(BeNil())

						sysPrep := custSpec.Identity.(*vimtypes.CustomizationSysprep)
						Expect(sysPrep.ResetPassword).To(HaveValue(BeTrue()))
					})
				})

				When("ScriptText is specified", func() {
					const text = "@echo off\necho hello"

					BeforeEach(func() {
						sysPrepSpec.Sysprep.ScriptText = &common.ValueOrSecretKeySelector{}
						bsArgs.Sysprep.ScriptText = text
					})

					It("should return expected customization spec", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(custSpec).ToNot(BeNil())

						sysPrep := custSpec.Identity.(*vimtypes.CustomizationSysprep)
						Expect(sysPrep.ScriptText).To(Equal(text))
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

						sysPrep := custSpec.Identity.(*vimtypes.CustomizationSysprep)
						Expect(sysPrep.ExtraConfig).To(HaveLen(1))
						optVal := sysPrep.ExtraConfig[0].GetOptionValue()
						Expect(optVal).ToNot(BeNil())
						Expect(optVal.Key).To(Equal(vmlifecycle.GOSCVCFAHashID))
						Expect(optVal.Value).To(Equal("foobar"))
					})
				})
			})

			When("no section is set", func() {

				BeforeEach(func() {
					sysPrepSpec.Sysprep = &vmopv1sysprep.Sysprep{}

					bsArgs.Sysprep = nil
				})

				It("still sets some fields", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					sysPrep, ok := custSpec.Identity.(*vimtypes.CustomizationSysprep)
					Expect(ok).To(BeTrue())

					name, ok := sysPrep.UserData.ComputerName.(*vimtypes.CustomizationFixedName)
					Expect(ok).To(BeTrue())
					Expect(name.Name).To(Equal(hostName))

					Expect(sysPrep.GuiUnattended.TimeZone).To(Equal(int32(85)))
				})
			})
		})

		Context("RawSysPrep", func() {
			BeforeEach(func() {
				sysPrepSpec.RawSysprep = &common.SecretKeySelector{}
				sysPrepSpec.RawSysprep.Name = "sysprep-secret"
				sysPrepSpec.RawSysprep.Key = "unattend"
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(configSpec).To(BeNil())

				Expect(custSpec).ToNot(BeNil())
				Expect(custSpec.GlobalIPSettings.DnsServerList).To(Equal(bsArgs.DNSServers))
				Expect(custSpec.GlobalIPSettings.DnsSuffixList).To(Equal(bsArgs.SearchSuffixes))

				sysPrepText := custSpec.Identity.(*vimtypes.CustomizationSysprepText)
				Expect(sysPrepText.Value).To(Equal(unattendXML))

				Expect(custSpec.NicSettingMap).To(HaveLen(len(bsArgs.NetworkResults.Results)))
				Expect(custSpec.NicSettingMap[0].MacAddress).To(Equal(macAddr))
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
})
