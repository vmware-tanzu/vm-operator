// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha3/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/sysprep"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
)

var _ = Describe("SysPrep Bootstrap", func() {
	const (
		macAddr = "43:AB:B4:1B:7E:87"
	)

	var (
		bsArgs     vmlifecycle.BootstrapArgs
		configInfo *types.VirtualMachineConfigInfo
	)

	BeforeEach(func() {
		configInfo = &types.VirtualMachineConfigInfo{}
		bsArgs.Data = map[string]string{}
	})

	AfterEach(func() {
		bsArgs = vmlifecycle.BootstrapArgs{}
	})

	Context("BootStrapSysPrep", func() {
		const unattendXML = "dummy-unattend-xml"

		var (
			configSpec *types.VirtualMachineConfigSpec
			custSpec   *types.CustomizationSpec
			err        error

			vmCtx          context.VirtualMachineContext
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
			}

			vmCtx = context.VirtualMachineContext{
				Context: goctx.Background(),
				Logger:  suite.GetLogger(),
				VM:      vm,
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
			autoUsers := int32(5)
			password, domainPassword, productID := "password_foo", "admin_password_foo", "product_id_foo"
			hostName := "foo-win-vm"

			BeforeEach(func() {
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
					UserData: &vmopv1sysprep.UserData{
						FullName:  "foo-bar",
						OrgName:   "foo-org",
						ProductID: &vmopv1sysprep.ProductIDSecretKeySelector{Key: "product_id_key"},
					},
					GUIRunOnce: vmopv1sysprep.GUIRunOnce{
						Commands: []string{"blah", "boom"},
					},
					Identification: &vmopv1sysprep.Identification{
						DomainAdmin:         "[Foo/Administrator]",
						JoinDomain:          "foo.local",
						JoinWorkgroup:       "foo.local.wg",
						DomainAdminPassword: &vmopv1sysprep.DomainPasswordSecretKeySelector{Key: "admin_pwd_key"},
					},
					LicenseFilePrintData: &vmopv1sysprep.LicenseFilePrintData{
						AutoMode:  vmopv1sysprep.CustomizationLicenseDataModePerServer,
						AutoUsers: &autoUsers,
					},
				}

				// secret data gets populated into the bootstrapArgs
				bsArgs.Sysprep = &sysprep.SecretData{
					ProductID:      productID,
					Password:       password,
					DomainPassword: domainPassword,
				}
				bsArgs.Hostname = hostName
			})

			It("should return expected customization spec", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(custSpec).ToNot(BeNil())

				sysPrep, ok := custSpec.Identity.(*types.CustomizationSysprep)
				Expect(ok).To(BeTrue())

				Expect(sysPrep.GuiUnattended.TimeZone).To(Equal(int32(4)))
				Expect(sysPrep.GuiUnattended.AutoLogonCount).To(Equal(int32(2)))
				Expect(sysPrep.GuiUnattended.AutoLogon).To(BeTrue())
				Expect(sysPrep.GuiUnattended.Password.Value).To(Equal(password))

				Expect(sysPrep.UserData.FullName).To(Equal("foo-bar"))
				Expect(sysPrep.UserData.OrgName).To(Equal("foo-org"))
				Expect(sysPrep.UserData.ProductId).To(Equal(productID))
				name, ok := sysPrep.UserData.ComputerName.(*types.CustomizationFixedName)
				Expect(ok).To(BeTrue())
				Expect(name.Name).To(Equal(hostName))

				Expect(sysPrep.GuiRunOnce.CommandList).To(HaveLen(2))

				Expect(sysPrep.Identification.DomainAdmin).To(Equal("[Foo/Administrator]"))
				Expect(sysPrep.Identification.JoinDomain).To(Equal("foo.local"))
				Expect(sysPrep.Identification.DomainAdminPassword.Value).To(Equal(domainPassword))
				Expect(sysPrep.Identification.JoinWorkgroup).To(Equal("foo.local.wg"))

				Expect(sysPrep.LicenseFilePrintData.AutoMode).To(Equal(types.CustomizationLicenseDataModePerServer))
				Expect(sysPrep.LicenseFilePrintData.AutoUsers).To(Equal(autoUsers))
			})

			When("no section is set", func() {

				BeforeEach(func() {
					sysPrepSpec.Sysprep = &vmopv1sysprep.Sysprep{}

					bsArgs.Sysprep = nil
				})

				It("does not set", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(custSpec).ToNot(BeNil())

					sysPrep, ok := custSpec.Identity.(*types.CustomizationSysprep)
					Expect(ok).To(BeTrue())

					name, ok := sysPrep.UserData.ComputerName.(*types.CustomizationFixedName)
					Expect(ok).To(BeTrue())
					Expect(name.Name).To(Equal(hostName))
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

				sysPrepText := custSpec.Identity.(*types.CustomizationSysprepText)
				Expect(sysPrepText.Value).To(Equal(unattendXML))

				Expect(custSpec.NicSettingMap).To(HaveLen(len(bsArgs.NetworkResults.Results)))
				Expect(custSpec.NicSettingMap[0].MacAddress).To(Equal(macAddr))
			})

			Context("when has vAppConfig", func() {
				const key, value = "fooKey", "fooValue"

				BeforeEach(func() {
					configInfo.VAppConfig = &types.VmConfigInfo{
						Property: []types.VAppPropertyInfo{
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
