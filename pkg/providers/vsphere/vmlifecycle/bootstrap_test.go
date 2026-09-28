// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1sysprep "github.com/vmware-tanzu/vm-operator/api/v1alpha6/sysprep"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
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

	Context("CloudInit", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
			}
		})

		When("Disabled is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Disabled = true
			})

			It("Noop", func() {
				Expect(bsErr).ToNot(HaveOccurred())
			})
		})
	})

	Context("LinuxPrep", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
			}
		})

		When("Disabled is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Disabled = true
			})

			It("Noop", func() {
				Expect(bsErr).ToNot(HaveOccurred())
			})
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

		When("Disabled is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Disabled = true
			})

			It("Noop", func() {
				Expect(bsErr).ToNot(HaveOccurred())
			})
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

	Context("vAppConfig", func() {
		BeforeEach(func() {
			vmCtx.VM.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
				VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
			}
		})

		When("Disabled is true", func() {
			BeforeEach(func() {
				vmCtx.VM.Spec.Bootstrap.Disabled = true
			})

			It("Noop", func() {
				Expect(bsErr).ToNot(HaveOccurred())
			})
		})
	})
})

var _ = Describe("GetBootstrapArgs", func() {
	const (
		cmNameservers    = "10.0.0.53 fd00::53 10.0.0.54"
		cmSearchSuffixes = "cm.local"
		specNameserver   = "192.168.0.53"
		specSearchDomain = "spec.local"
		ifaceNameserver  = "172.16.0.53"
	)

	var (
		cmNS   = []string{"10.0.0.53", "fd00::53", "10.0.0.54"}
		cmNSv4 = []string{"10.0.0.53", "10.0.0.54"}
		cmSS   = []string{"cm.local"}
	)

	var (
		ctx        context.Context
		vm         *vmopv1.VirtualMachine
		configMap  *corev1.ConfigMap
		guestID    string
		bootstraps []network.Bootstrap

		bsa vmlifecycle.BootstrapArgs
		err error
	)

	static := func() network.Bootstrap {
		return network.Bootstrap{
			IPConfigs: []network.NetworkInterfaceIPConfig{
				{
					IPCIDR:  "192.168.1.10/24",
					IsIPv4:  true,
					Gateway: "192.168.1.1",
				},
			},
		}
	}
	staticNoGateway := func() network.Bootstrap {
		b := static()
		b.IPConfigs[0].Gateway = ""
		return b
	}
	staticV6 := func() network.Bootstrap {
		return network.Bootstrap{
			IPConfigs: []network.NetworkInterfaceIPConfig{
				{
					IPCIDR:  "fd00::10/64",
					Gateway: "fd00::1",
				},
			},
		}
	}
	dualStack := func() network.Bootstrap {
		b := static()
		b.IPConfigs = append(b.IPConfigs, staticV6().IPConfigs...)
		return b
	}
	dhcp := func() network.Bootstrap {
		return network.Bootstrap{DHCP4: true}
	}
	noIPAM := func() network.Bootstrap {
		return network.Bootstrap{NoIPAM: true}
	}

	enableScoped := func() {
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.ScopedDNSDefaults = true
		})
	}

	BeforeEach(func() {
		ctx = pkgcfg.NewContextWithDefaultConfig()

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      config.NetworkConfigMapName,
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			},
			Data: map[string]string{
				config.NameserversKey:    cmNameservers,
				config.SearchSuffixesKey: cmSearchSuffixes,
			},
		}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "get-bootstrap-args-test",
				Namespace: "my-namespace",
			},
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
				Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				},
			},
		}

		guestID = string(vimtypes.VirtualMachineGuestOsIdentifierUbuntu64Guest)
		bootstraps = nil
	})

	JustBeforeEach(func() {
		var objs []ctrlclient.Object
		if configMap != nil {
			objs = append(objs, configMap)
		}

		vmCtx := pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
			MoVM: mo.VirtualMachine{
				Config: &vimtypes.VirtualMachineConfigInfo{
					GuestId: guestID,
				},
			},
		}

		bsa, err = vmlifecycle.GetBootstrapArgs(
			vmCtx,
			builder.NewFakeClient(objs...),
			bootstraps,
			false,
			vmlifecycle.BootstrapData{})
	})

	Context("Legacy", func() {
		When("the capability is not enabled", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{static(), dhcp(), static()}
			})

			It("applies the defaults to every static interface", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
				Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				Expect(bsa.NetBootstraps[2].Nameservers).To(Equal(cmNS))
				Expect(bsa.DNSServers).To(Equal(cmNS))
				Expect(bsa.SearchSuffixes).To(Equal(cmSS))
				Expect(bsa.TemplateDNSServers).To(Equal(cmNS))
				Expect(vm.Annotations).ToNot(HaveKey(pkgconst.DNSDefaultsAnnotationKey))
			})

			When("the VM is annotated with the scoped behavior", func() {
				BeforeEach(func() {
					vm.Annotations = map[string]string{
						pkgconst.DNSDefaultsAnnotationKey: pkgconst.DNSDefaultsScoped,
					}
				})

				It("uses the legacy behavior and keeps the annotation", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
					Expect(bsa.NetBootstraps[2].Nameservers).To(Equal(cmNS))
					Expect(vm.Annotations).To(HaveKeyWithValue(
						pkgconst.DNSDefaultsAnnotationKey, pkgconst.DNSDefaultsScoped))
				})
			})

			When("the bootstrap provider is LinuxPrep", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
						LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
					}
				})

				It("applies the default nameservers to GOSC", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal(cmNS))
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})

			When("there is no bootstrap provider for a Linux VM", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap = nil
				})

				It("applies the default nameservers to GOSC", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal(cmNS))
					Expect(bsa.SearchSuffixes).To(Equal(cmSS))
				})
			})
		})

		When("the VM specifies DNS", func() {
			BeforeEach(func() {
				vm.Spec.Network.Nameservers = []string{"fd00::99"}
				vm.Spec.Network.SearchDomains = []string{specSearchDomain}
				b := static()
				b.Nameservers = []string{ifaceNameserver}
				bootstraps = []network.Bootstrap{static(), dhcp(), noIPAM(), b}
			})

			It("applies the VM's DNS unfiltered to every interface without its own", func() {
				Expect(err).ToNot(HaveOccurred())
				for i := range 3 {
					Expect(bsa.NetBootstraps[i].Nameservers).To(Equal([]string{"fd00::99"}))
					Expect(bsa.NetBootstraps[i].SearchDomains).To(Equal([]string{specSearchDomain}))
				}
				Expect(bsa.NetBootstraps[3].Nameservers).To(Equal([]string{ifaceNameserver}))
				Expect(bsa.NetBootstraps[3].SearchDomains).To(Equal([]string{specSearchDomain}))
				Expect(bsa.DNSServers).To(Equal([]string{"fd00::99"}))
				Expect(bsa.SearchSuffixes).To(Equal([]string{specSearchDomain}))
			})

			When("UseGlobalNameserversAsDefault and UseGlobalSearchDomainsAsDefault are false", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap.CloudInit.UseGlobalNameserversAsDefault = ptr.To(false)
					vm.Spec.Bootstrap.CloudInit.UseGlobalSearchDomainsAsDefault = ptr.To(false)
				})

				It("does not apply the VM's DNS to the interfaces", func() {
					Expect(err).ToNot(HaveOccurred())
					for i := range 3 {
						Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
					}
					Expect(bsa.NetBootstraps[3].Nameservers).To(Equal([]string{ifaceNameserver}))
				})
			})
		})

		When("the capability is enabled", func() {
			BeforeEach(func() {
				enableScoped()
				bootstraps = []network.Bootstrap{static(), static()}
			})

			DescribeTable("the VM's guest may have already been bootstrapped",
				func(key string) {
					vm.Annotations = map[string]string{key: "true"}
					vmCtx := pkgctx.VirtualMachineContext{
						Context: ctx,
						Logger:  suite.GetLogger(),
						VM:      vm,
					}
					bsa, err = vmlifecycle.GetBootstrapArgs(
						vmCtx,
						builder.NewFakeClient(configMap),
						[]network.Bootstrap{static(), static()},
						false,
						vmlifecycle.BootstrapData{})
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Annotations).To(HaveKeyWithValue(
						pkgconst.DNSDefaultsAnnotationKey, pkgconst.DNSDefaultsLegacy))
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
					Expect(bsa.NetBootstraps[1].Nameservers).To(Equal(cmNS))
				},
				Entry("configSpec hash", pkgconst.BootstrapHashConfigSpecAnnotationKey),
				Entry("customSpec hash", pkgconst.BootstrapHashCustomSpecAnnotationKey),
				Entry("first boot done", vmopv1.FirstBootDoneAnnotation),
				Entry("restored", vmopv1.RestoredVMAnnotation),
				Entry("imported", vmopv1.ImportedVMAnnotation),
				Entry("failed over", vmopv1.FailedOverVMAnnotation),
			)

			DescribeTable("the annotation selects the legacy behavior",
				func(value string) {
					vm.Annotations = map[string]string{
						pkgconst.DNSDefaultsAnnotationKey: value,
					}
					// Re-run since the annotation is set after JustBeforeEach.
					vmCtx := pkgctx.VirtualMachineContext{
						Context: ctx,
						Logger:  suite.GetLogger(),
						VM:      vm,
					}
					bsa, err = vmlifecycle.GetBootstrapArgs(
						vmCtx,
						builder.NewFakeClient(configMap),
						[]network.Bootstrap{static(), static()},
						false,
						vmlifecycle.BootstrapData{})
					Expect(err).ToNot(HaveOccurred())
					Expect(vm.Annotations).To(HaveKeyWithValue(
						pkgconst.DNSDefaultsAnnotationKey, value))
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
					Expect(bsa.NetBootstraps[1].Nameservers).To(Equal(cmNS))
				},
				Entry("legacy", pkgconst.DNSDefaultsLegacy),
				Entry("unknown value", "bogus"),
			)
		})
	})

	Context("Scoped", func() {
		BeforeEach(func() {
			enableScoped()
		})

		withVMNameservers := func() {
			vm.Spec.Network.Nameservers = []string{specNameserver}
		}
		withVMSearchDomains := func() {
			vm.Spec.Network.SearchDomains = []string{specSearchDomain}
		}

		It("annotates a new VM with the scoped behavior", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(vm.Annotations).To(HaveKeyWithValue(
				pkgconst.DNSDefaultsAnnotationKey, pkgconst.DNSDefaultsScoped))
		})

		Context("CloudInit", func() {
			When("there are multiple static interfaces", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), static()}
				})

				It("applies the default nameservers to only the first interface and no default search domains", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[0].SearchDomains).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].SearchDomains).To(BeEmpty())
					Expect(bsa.TemplateDNSServers).To(Equal(cmNS))
				})

				It("reports only the applied global DNS in status", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(BeEmpty())
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})

			When("the first interface is followed by a DHCP interface", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), dhcp()}
				})

				It("applies the default nameservers to only the first interface", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[0].SearchDomains).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].SearchDomains).To(BeEmpty())
				})
			})

			DescribeTable("the first interface does not need the defaults",
				func(first network.Bootstrap) {
					bootstraps = []network.Bootstrap{first, static()}
					// Reading the ConfigMap would fail.
					configMap.Data[config.NameserversKey] = "<worker_dns>"
					vmCtx := pkgctx.VirtualMachineContext{
						Context: ctx,
						Logger:  suite.GetLogger(),
						VM:      vm,
						MoVM: mo.VirtualMachine{
							Config: &vimtypes.VirtualMachineConfigInfo{GuestId: guestID},
						},
					}
					bsa, err = vmlifecycle.GetBootstrapArgs(
						vmCtx,
						builder.NewFakeClient(configMap),
						bootstraps,
						false,
						vmlifecycle.BootstrapData{})
					Expect(err).ToNot(HaveOccurred())
					for i := range bsa.NetBootstraps {
						Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
					}
					Expect(bsa.DNSServers).To(BeEmpty())
				},
				Entry("DHCP", dhcp()),
				Entry("no IPAM", noIPAM()),
				Entry("static without a gateway", staticNoGateway()),
				Entry("no IPAM with addresses", func() network.Bootstrap {
					b := noIPAM()
					b.IPConfigs = static().IPConfigs
					return b
				}()),
			)

			When("the first interface is IPv6 only", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{staticV6()}
				})

				It("applies only the IPv6 default nameservers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{"fd00::53"}))
				})
			})

			When("the first interface is dual-stack", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{dualStack()}
				})

				It("applies all the default nameservers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
				})
			})

			When("the first interface is dual-stack with an IPv6 gateway set to None", func() {
				BeforeEach(func() {
					b := dualStack()
					b.IPConfigs[1].Gateway = ""
					bootstraps = []network.Bootstrap{b}
				})

				It("applies only the IPv4 default nameservers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
				})
			})

			When("the first interface is IPv4 with Router Advertisements", func() {
				BeforeEach(func() {
					b := static()
					b.AcceptRA = true
					bootstraps = []network.Bootstrap{b}
				})

				It("applies all the default nameservers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNS))
				})
			})

			When("there are only DHCP interfaces and the ConfigMap is invalid", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{dhcp(), dhcp()}
					configMap.Data[config.NameserversKey] = "<worker_dns>"
				})

				It("does not read the ConfigMap", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(BeEmpty())
					Expect(bsa.TemplateDNSServers).To(BeEmpty())
				})
			})

			When("another interface specifies nameservers", func() {
				BeforeEach(func() {
					b := static()
					b.Nameservers = []string{ifaceNameserver}
					bootstraps = []network.Bootstrap{static(), b}
				})

				It("still applies the default nameservers to the first interface", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[1].Nameservers).To(Equal([]string{ifaceNameserver}))
				})
			})

			When("the first interface specifies nameservers", func() {
				BeforeEach(func() {
					b := static()
					b.Nameservers = []string{ifaceNameserver}
					bootstraps = []network.Bootstrap{b, static()}
				})

				It("does not apply the defaults to the first interface", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{ifaceNameserver}))
					Expect(bsa.NetBootstraps[0].SearchDomains).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				})
			})

			When("the VM specifies nameservers", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), dhcp(), noIPAM(), static()}
					withVMNameservers()
					// So the ConfigMap is not needed for search domains.
					withVMSearchDomains()
					// Reading the ConfigMap would fail.
					configMap.Data[config.NameserversKey] = "<worker_dns>"
				})

				It("applies the VM's DNS to every static interface without its own", func() {
					Expect(err).ToNot(HaveOccurred())
					for _, i := range []int{0, 3} {
						Expect(bsa.NetBootstraps[i].Nameservers).To(Equal([]string{specNameserver}))
						Expect(bsa.NetBootstraps[i].SearchDomains).To(Equal([]string{specSearchDomain}))
					}
					for _, i := range []int{1, 2} {
						Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
					}
					Expect(bsa.DNSServers).To(BeEmpty())
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})

				When("the DHCP interface specifies nameservers", func() {
					BeforeEach(func() {
						bootstraps[1].Nameservers = []string{ifaceNameserver}
					})

					It("applies the interface's nameservers", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[1].Nameservers).To(Equal([]string{ifaceNameserver}))
					})
				})

				When("every interface is DHCP", func() {
					BeforeEach(func() {
						bootstraps = []network.Bootstrap{dhcp(), dhcp()}
					})

					It("does not apply the VM's DNS", func() {
						Expect(err).ToNot(HaveOccurred())
						for i := range bsa.NetBootstraps {
							Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
							Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
						}
					})
				})
			})

			When("the VM specifies nameservers of both IP families", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), dualStack(), staticV6()}
					vm.Spec.Network.Nameservers = []string{"fd00::99", specNameserver}
				})

				It("applies the VM's nameservers of each interface's IP families", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{specNameserver}))
					Expect(bsa.NetBootstraps[1].Nameservers).To(Equal([]string{"fd00::99", specNameserver}))
					Expect(bsa.NetBootstraps[2].Nameservers).To(Equal([]string{"fd00::99"}))
				})
			})

			When("the VM specifies nameservers of another IP family", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static()}
					vm.Spec.Network.Nameservers = []string{"fd00::99"}
				})

				It("applies neither the VM's nameservers nor the defaults", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(BeEmpty())
				})
			})

			When("UseGlobalNameserversAsDefault is false", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap.CloudInit.UseGlobalNameserversAsDefault = ptr.To(false)
					bootstraps = []network.Bootstrap{static(), static()}
				})

				It("still applies the default nameservers to the first interface", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[0].SearchDomains).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				})

				When("UseGlobalSearchDomainsAsDefault is also false", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap.CloudInit.UseGlobalSearchDomainsAsDefault = ptr.To(false)
					})

					It("still applies the default nameservers to the first interface", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
						Expect(bsa.NetBootstraps[0].SearchDomains).To(BeEmpty())
						Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[1].SearchDomains).To(BeEmpty())
					})
				})
			})

			When("the VM specifies search domains", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), static()}
					withVMSearchDomains()
				})

				It("applies the VM's search domains to every interface and the default nameservers to the first", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].SearchDomains).To(Equal([]string{specSearchDomain}))
					Expect(bsa.NetBootstraps[1].SearchDomains).To(Equal([]string{specSearchDomain}))
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				})
			})

			When("the VM is a TKG VM", func() {
				BeforeEach(func() {
					vm.Labels = map[string]string{
						kubeutil.CAPWClusterRoleLabelKey: "",
					}
					bootstraps = []network.Bootstrap{static(), dhcp(), static()}
				})

				It("applies the default nameservers and search domains to only the first interface", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[0].SearchDomains).To(Equal(cmSS))
					for _, i := range []int{1, 2} {
						Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
					}
				})

				// VKS config: the guest merges the nameservers of every interface
				// into resolv.conf, so applying the defaults to both the primary
				// and secondary networks duplicated them and exceeded the glibc
				// limit of three nameservers.
				When("the node has a static primary and secondary network", func() {
					BeforeEach(func() {
						bootstraps = []network.Bootstrap{static(), static()}
					})

					It("does not duplicate the default nameservers across interfaces", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
						Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[1].SearchDomains).To(BeEmpty())

						var merged []string
						for _, b := range bsa.NetBootstraps {
							merged = append(merged, b.Nameservers...)
						}
						Expect(merged).To(HaveLen(len(cmNSv4)))
						Expect(len(merged)).To(BeNumerically("<=", 3))
					})
				})

				When("the first interface is DHCP", func() {
					BeforeEach(func() {
						bootstraps = []network.Bootstrap{dhcp(), static()}
					})

					It("does not apply the defaults", func() {
						Expect(err).ToNot(HaveOccurred())
						for i := range bsa.NetBootstraps {
							Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
							Expect(bsa.NetBootstraps[i].SearchDomains).To(BeEmpty())
						}
					})
				})

				When("the VM specifies search domains", func() {
					BeforeEach(func() {
						withVMSearchDomains()
					})

					It("does not apply the default search domains", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[0].SearchDomains).To(Equal([]string{specSearchDomain}))
						Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					})
				})
			})
		})

		Context("LinuxPrep", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					LinuxPrep: &vmopv1.VirtualMachineBootstrapLinuxPrepSpec{},
				}
				bootstraps = []network.Bootstrap{dhcp(), static()}
			})

			It("does not apply the defaults since the first interface uses DHCP", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(bsa.DNSServers).To(BeEmpty())
				Expect(bsa.SearchSuffixes).To(BeEmpty())
				Expect(bsa.NetBootstraps[0].Nameservers).To(BeEmpty())
				Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				Expect(bsa.TemplateDNSServers).To(Equal(cmNS))
			})

			When("every interface is static", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), static()}
				})

				It("applies the default nameservers of the first interface's IP families to GOSC", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal(cmNSv4))
					Expect(bsa.SearchSuffixes).To(BeEmpty())
					Expect(bsa.NetBootstraps[0].Nameservers).To(BeEmpty())
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				})
			})

			When("the first interface is followed by a DHCP interface", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{static(), dhcp()}
				})

				It("applies the default nameservers since only the first interface is considered", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal(cmNSv4))
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})

			When("the first interface has no gateway", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{staticNoGateway(), static()}
				})

				It("does not apply the defaults", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(BeEmpty())
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})

			When("the VM specifies DNS", func() {
				BeforeEach(func() {
					withVMNameservers()
					withVMSearchDomains()
				})

				It("applies the VM's DNS to GOSC", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal([]string{specNameserver}))
					Expect(bsa.SearchSuffixes).To(Equal([]string{specSearchDomain}))
				})
			})

			When("there is no bootstrap provider for a Linux VM", func() {
				BeforeEach(func() {
					vm.Spec.Bootstrap = nil
					bootstraps = []network.Bootstrap{static()}
				})

				It("applies the default nameservers to GOSC", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(Equal(cmNSv4))
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})
		})

		Context("vAppConfig", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					VAppConfig: &vmopv1.VirtualMachineBootstrapVAppConfigSpec{},
				}
				bootstraps = []network.Bootstrap{static()}
			})

			It("reports the resolved DNS in status and does not apply the defaults", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(bsa.DNSServers).To(Equal(cmNS))
				Expect(bsa.SearchSuffixes).To(Equal(cmSS))
				Expect(bsa.TemplateDNSServers).To(Equal(cmNS))
				Expect(bsa.NetBootstraps[0].Nameservers).To(BeEmpty())
			})
		})

		Context("Sysprep", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
				}
				guestID = string(vimtypes.VirtualMachineGuestOsIdentifierWindows9_64Guest)
				bootstraps = []network.Bootstrap{static(), dhcp(), static()}
			})

			It("applies the default nameservers to only the first adapter and no default search suffixes", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(bsa.DNSServers).To(BeEmpty())
				Expect(bsa.SearchSuffixes).To(BeEmpty())
				Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
				Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
				Expect(bsa.NetBootstraps[2].Nameservers).To(BeEmpty())
			})

			When("the first adapter is DHCP", func() {
				BeforeEach(func() {
					bootstraps = []network.Bootstrap{dhcp(), static(), static()}
				})

				It("does not apply the defaults", func() {
					Expect(err).ToNot(HaveOccurred())
					for i := range bsa.NetBootstraps {
						Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
					}
					Expect(bsa.SearchSuffixes).To(BeEmpty())
				})
			})

			When("the VM specifies nameservers", func() {
				BeforeEach(func() {
					withVMNameservers()
				})

				It("applies the VM's nameservers to every static adapter without its own", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.DNSServers).To(BeEmpty())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{specNameserver}))
					Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
					Expect(bsa.NetBootstraps[2].Nameservers).To(Equal([]string{specNameserver}))
				})

				When("an adapter specifies nameservers", func() {
					BeforeEach(func() {
						bootstraps[2].Nameservers = []string{ifaceNameserver}
					})

					It("keeps the adapter's nameservers", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{specNameserver}))
						Expect(bsa.NetBootstraps[1].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[2].Nameservers).To(Equal([]string{ifaceNameserver}))
					})
				})

				When("the DHCP adapter specifies nameservers", func() {
					BeforeEach(func() {
						bootstraps[1].Nameservers = []string{ifaceNameserver}
					})

					It("applies the adapter's nameservers to override DHCP", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{specNameserver}))
						Expect(bsa.NetBootstraps[1].Nameservers).To(Equal([]string{ifaceNameserver}))
						Expect(bsa.NetBootstraps[2].Nameservers).To(Equal([]string{specNameserver}))
					})
				})

				When("an adapter is NoIPAM or of another IP family", func() {
					BeforeEach(func() {
						bootstraps = append(bootstraps, noIPAM(), staticV6())
					})

					It("does not apply the VM's nameservers to them", func() {
						Expect(err).ToNot(HaveOccurred())
						Expect(bsa.NetBootstraps[3].Nameservers).To(BeEmpty())
						Expect(bsa.NetBootstraps[4].Nameservers).To(BeEmpty())
					})
				})

				When("every adapter is DHCP", func() {
					BeforeEach(func() {
						bootstraps = []network.Bootstrap{dhcp(), dhcp()}
						// So the ConfigMap is not needed for templates.
						configMap.Data[config.NameserversKey] = "<worker_dns>"
					})

					It("does not apply the VM's nameservers", func() {
						Expect(err).ToNot(HaveOccurred())
						for i := range bsa.NetBootstraps {
							Expect(bsa.NetBootstraps[i].Nameservers).To(BeEmpty())
						}
						Expect(bsa.DNSServers).To(BeEmpty())
						Expect(bsa.TemplateDNSServers).To(Equal([]string{specNameserver}))
					})
				})
			})

			When("the VM specifies search domains", func() {
				BeforeEach(func() {
					withVMSearchDomains()
				})

				It("applies the VM's search domains globally", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.SearchSuffixes).To(Equal([]string{specSearchDomain}))
				})
			})

			When("another adapter specifies nameservers", func() {
				BeforeEach(func() {
					bootstraps[2].Nameservers = []string{ifaceNameserver}
				})

				It("still applies the default nameservers to the first adapter", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal(cmNSv4))
					Expect(bsa.NetBootstraps[2].Nameservers).To(Equal([]string{ifaceNameserver}))
				})
			})

			When("the first adapter specifies nameservers", func() {
				BeforeEach(func() {
					bootstraps[0].Nameservers = []string{ifaceNameserver}
				})

				It("does not apply the default nameservers", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(bsa.NetBootstraps[0].Nameservers).To(Equal([]string{ifaceNameserver}))
					Expect(bsa.NetBootstraps[2].Nameservers).To(BeEmpty())
				})
			})
		})
	})
})
