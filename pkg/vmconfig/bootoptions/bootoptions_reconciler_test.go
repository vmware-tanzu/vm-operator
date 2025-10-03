// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package bootoptions_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/bootoptions"
	"github.com/vmware-tanzu/vm-operator/test/builder"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

var _ = Describe("New", Label(testlabels.V1Alpha4), func() {
	It("should return a reconciler", func() {
		Expect(bootoptions.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", Label(testlabels.V1Alpha4), func() {
	It("should return 'bootoptions'", func() {
		Expect(bootoptions.New().Name()).To(Equal("bootoptions"))
	})
})

var _ = Describe("Reconcile", Label(testlabels.V1Alpha4), func() {

	var (
		r          vmconfig.Reconciler
		ctx        context.Context
		k8sClient  ctrlclient.Client
		vimClient  *vim25.Client
		moVM       mo.VirtualMachine
		vm         *vmopv1.VirtualMachine
		withObjs   []ctrlclient.Object
		withFuncs  interceptor.Funcs
		configSpec *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		r = bootoptions.New()

		vcsimCtx := builder.NewTestContextForVCSim(
			ctxop.WithContext(pkgcfg.NewContextWithDefaultConfig()), builder.VCSimTestConfig{})
		ctx = vcsimCtx
		ctx = vmconfig.WithContext(ctx)

		vimClient = vcsimCtx.VCClient.Client

		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}

		configSpec = &vimtypes.VirtualMachineConfigSpec{}

		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "my-namespace",
				Name:      "my-vm",
			},
			Spec: vmopv1.VirtualMachineSpec{},
		}

		withFuncs = interceptor.Funcs{}
		withObjs = []ctrlclient.Object{}
	})

	JustBeforeEach(func() {
		k8sClient = builder.NewFakeClientWithInterceptors(withFuncs, withObjs...)
	})

	Context("a panic is expected", func() {
		When("ctx is nil", func() {
			JustBeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("context is nil"))
			})
		})
		When("k8sClient is nil", func() {
			JustBeforeEach(func() {
				k8sClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})
		When("vimClient is nil", func() {
			JustBeforeEach(func() {
				vimClient = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vimClient is nil"))
			})
		})
		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})
		When("configSpec is nil", func() {
			JustBeforeEach(func() {
				configSpec = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
				}
				Expect(fn).To(PanicWith("configSpec is nil"))
			})
		})
	})

	When("no panic is expected", func() {
		var (
			err error
		)

		JustBeforeEach(func() {
			err = r.Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
		})

		Context("bootDelay", func() {
			When("bootDelay is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootDelay: &metav1.Duration{Duration: 10 * time.Second},
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.BootDelay).To(Equal(metav1.Duration{Duration: 10 * time.Second}.Milliseconds()))
				})
			})

			When("bootDelay is removed from vm spec", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootDelay: metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootDelay: &metav1.Duration{Duration: 0 * time.Second},
					}
				})
				It("should not be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("bootDelay remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootDelay: &metav1.Duration{Duration: 10 * time.Second},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootDelay: metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
				})
				It("should leave an empty configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("bootDelay is updated", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootDelay: &metav1.Duration{Duration: 5 * time.Second},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootDelay: metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.BootDelay).To(Equal(metav1.Duration{Duration: 5 * time.Second}.Milliseconds()))
				})
			})
		})

		Context("bootRetry", func() {
			When("bootRetry is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetry: vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.BootRetryEnabled).To(BeTrue())
				})
			})

			When("bootRetry is updated in vm spec", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootRetryEnabled: ptr.To(true),
							},
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetry: vmopv1.VirtualMachineBootOptionsBootRetryDisabled,
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.BootRetryEnabled).To(BeFalse())
				})
			})

			When("bootRetry remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetry: vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootRetryEnabled: ptr.To(true),
							},
						},
					}
				})
				It("should leave an empty configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})
		})

		Context("bootRetryDelay", func() {
			When("bootRetryDelay is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.BootRetryDelay).To(Equal(metav1.Duration{Duration: 10 * time.Second}.Milliseconds()))
				})
			})

			When("bootRetryDelay is removed from vm spec", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootRetryDelay: metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetryDelay: &metav1.Duration{Duration: 0 * time.Second},
					}
				})
				It("should not be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("bootRetryDelay remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
						BootRetryDelay: &metav1.Duration{Duration: 10 * time.Second},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootRetryEnabled: ptr.To(true),
								BootRetryDelay:   metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
				})
				It("should leave an empty configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("bootRetryDelay is updated", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootRetry:      vmopv1.VirtualMachineBootOptionsBootRetryEnabled,
						BootRetryDelay: &metav1.Duration{Duration: 5 * time.Second},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								BootRetryEnabled: ptr.To(true),
								BootRetryDelay:   metav1.Duration{Duration: 10 * time.Second}.Milliseconds(),
							},
						},
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.BootRetryDelay).To(Equal(metav1.Duration{Duration: 5 * time.Second}.Milliseconds()))
				})
			})
		})

		Context("efiSecureBootEnabled", func() {
			When("efiSecureBootEnabled is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.EfiSecureBootEnabled).To(BeTrue())
				})
			})

			When("efiSecureBootEnabled is updated in vm spec", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								EfiSecureBootEnabled: ptr.To(true),
							},
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootDisabled,
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.EfiSecureBootEnabled).To(BeFalse())
				})
			})

			When("efiSecureBootEnabled remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EFISecureBoot: vmopv1.VirtualMachineBootOptionsEFISecureBootEnabled,
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								EfiSecureBootEnabled: ptr.To(true),
							},
						},
					}
				})
				It("should leave an empty configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})
		})

		Context("networkBootProtocol", func() {
			When("networkBootProtocol is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						NetworkBootProtocol: vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.NetworkBootProtocol).To(Equal(string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv4)))
				})
			})

			When("networkBootProtocol remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						NetworkBootProtocol: vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP4,
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								NetworkBootProtocol: string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv4),
							},
						},
					}
				})
				It("should not be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("networkBootProtocol is updated", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						NetworkBootProtocol: vmopv1.VirtualMachineBootOptionsNetworkBootProtocolIP6,
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								NetworkBootProtocol: string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv4),
							},
						},
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(configSpec.BootOptions.NetworkBootProtocol).To(Equal(string(vimtypes.VirtualMachineBootOptionsNetworkBootProtocolTypeIpv6)))
				})
			})
		})

		Context("bootOrder", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
						{
							Name: "eth0",
						},
					},
				}
				vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
					{
						Name:     "disk-0",
						DiskUUID: "6000C298-df15-fe89-ddcb-8ea33329595d",
					},
				}

				ethDevice := &vimtypes.VirtualVmxnet3{}
				ethDevice.Key = 4000

				moVM = mo.VirtualMachine{
					Config: &vimtypes.VirtualMachineConfigInfo{
						Hardware: vimtypes.VirtualHardware{
							Device: []vimtypes.BaseVirtualDevice{
								&vimtypes.VirtualCdrom{},
								&vimtypes.VirtualDisk{
									VirtualDevice: vimtypes.VirtualDevice{
										Key: 2000,
										Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
											Uuid: "6000C298-df15-fe89-ddcb-8ea33329595d",
										},
									},
								},
								ethDevice,
							},
						},
					},
				}
			})

			When("bootOrder is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
				})

				It("should be set in configSpec", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(len(configSpec.BootOptions.BootOrder)).To(Equal(len(vm.Spec.BootOptions.BootOrder)))
					Expect(configSpec.BootOptions.BootOrder[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableCdromDevice{}))
					Expect(configSpec.BootOptions.BootOrder[1]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{}))
					diskDevice := configSpec.BootOptions.BootOrder[1].(*vimtypes.VirtualMachineBootOptionsBootableDiskDevice)
					Expect(diskDevice.DeviceKey).To(Equal(int32(2000)))
					Expect(configSpec.BootOptions.BootOrder[2]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableEthernetDevice{}))
					ethDevice := configSpec.BootOptions.BootOrder[2].(*vimtypes.VirtualMachineBootOptionsBootableEthernetDevice)
					Expect(ethDevice.DeviceKey).To(Equal(int32(4000)))
				})
			})

			When("bootOrder is remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
					moVM.Config.BootOptions = &vimtypes.VirtualMachineBootOptions{
						BootOrder: []vimtypes.BaseVirtualMachineBootOptionsBootableDevice{
							&vimtypes.VirtualMachineBootOptionsBootableCdromDevice{},
							&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{
								DeviceKey: 2000,
							},
							&vimtypes.VirtualMachineBootOptionsBootableEthernetDevice{
								DeviceKey: 4000,
							},
						},
					}
				})

				It("should not be set in configSpec", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("bootOrder is updated", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
						},
					}
					moVM.Config.BootOptions = &vimtypes.VirtualMachineBootOptions{
						BootOrder: []vimtypes.BaseVirtualMachineBootOptionsBootableDevice{
							&vimtypes.VirtualMachineBootOptionsBootableCdromDevice{},
							&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{
								DeviceKey: 2000,
							},
							&vimtypes.VirtualMachineBootOptionsBootableEthernetDevice{
								DeviceKey: 4000,
							},
						},
					}
				})

				It("should be updated in configSpec", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(len(configSpec.BootOptions.BootOrder)).To(Equal(len(vm.Spec.BootOptions.BootOrder)))
					Expect(configSpec.BootOptions.BootOrder[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableCdromDevice{}))
					Expect(configSpec.BootOptions.BootOrder[1]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{}))
					diskDevice := configSpec.BootOptions.BootOrder[1].(*vimtypes.VirtualMachineBootOptionsBootableDiskDevice)
					Expect(diskDevice.DeviceKey).To(Equal(int32(2000)))
				})
			})

			When("bootOrder is removed", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = nil
					moVM.Config.BootOptions = &vimtypes.VirtualMachineBootOptions{
						BootOrder: []vimtypes.BaseVirtualMachineBootOptionsBootableDevice{
							&vimtypes.VirtualMachineBootOptionsBootableCdromDevice{},
							&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{
								DeviceKey: 2000,
							},
							&vimtypes.VirtualMachineBootOptionsBootableEthernetDevice{
								DeviceKey: 4000,
							},
						},
					}
				})

				It("should be removed from configSpec", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(configSpec.BootOptions).To(BeNil())
				})
			})

			When("boot disk is not found in status.volumes", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-1",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to locate disk matching name"))
				})
			})

			When("boot disk UUID is not found in configInfo", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualCdrom{},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2000,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "6000C298-d873-cdd9-a1e2-bd595af0e6f5",
											},
										},
									},
								},
							},
						},
					}
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to locate a disk matching UUID"))
					Expect(err.Error()).To(ContainSubstring(vm.Status.Volumes[0].DiskUUID))
				})
			})

			When("ethernet device is not found", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth1",
							},
						},
					}
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unable to locate a network interface matching name"))
				})
			})

			When("CD-ROM device is not found", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2000,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "6000C298-df15-fe89-ddcb-8ea33329595d",
											},
										},
									},
								},
							},
						},
					}
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("no cdrom device found"))
				})
			})

			When("unsupported device type is specified", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: "usb",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableCDRomDevice,
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-1",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth0",
							},
						},
					}
				})

				It("should return an error", func() {
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("unsupported bootable device type"))
				})
			})

			When("there are multiple devices", func() {
				BeforeEach(func() {
					ethDevice1 := &vimtypes.VirtualVmxnet3{}
					ethDevice1.Key = 4000
					ethDevice2 := &vimtypes.VirtualVmxnet3{}
					ethDevice2.Key = 4001
					ethDevice3 := &vimtypes.VirtualVmxnet3{}
					ethDevice3.Key = 4002

					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							Hardware: vimtypes.VirtualHardware{
								Device: []vimtypes.BaseVirtualDevice{
									&vimtypes.VirtualCdrom{},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2000,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "6000C298-df15-fe89-ddcb-8ea33329595d",
											},
										},
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2001,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "6000C298-df15-fe89-ddcb-8ea33329595e",
											},
										},
									},
									&vimtypes.VirtualDisk{
										VirtualDevice: vimtypes.VirtualDevice{
											Key: 2002,
											Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
												Uuid: "6000C298-df15-fe89-ddcb-8ea33329595f",
											},
										},
									},
									ethDevice1,
									ethDevice2,
									ethDevice3,
								},
							},
						},
					}
					vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
						Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
							{
								Name: "eth0",
							},
							{
								Name: "eth1",
							},
							{
								Name: "eth2",
							},
						},
					}
					vm.Status.Volumes = []vmopv1.VirtualMachineVolumeStatus{
						{
							Name:     "disk-2",
							DiskUUID: "6000C298-df15-fe89-ddcb-8ea33329595f",
						},
						{
							Name:     "disk-0",
							DiskUUID: "6000C298-df15-fe89-ddcb-8ea33329595d",
						},
						{
							Name:     "disk-1",
							DiskUUID: "6000C298-df15-fe89-ddcb-8ea33329595e",
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						BootOrder: []vmopv1.VirtualMachineBootOptionsBootableDevice{
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-1",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableDiskDevice,
								Name: "disk-0",
							},
							{
								Type: vmopv1.VirtualMachineBootOptionsBootableNetworkDevice,
								Name: "eth2",
							},
						},
					}
				})

				It("should set the correct configSpec bootOrder", func() {
					Expect(err).NotTo(HaveOccurred())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(len(configSpec.BootOptions.BootOrder)).To(Equal(len(vm.Spec.BootOptions.BootOrder)))
					Expect(configSpec.BootOptions.BootOrder[0]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{}))
					disk1 := configSpec.BootOptions.BootOrder[0].(*vimtypes.VirtualMachineBootOptionsBootableDiskDevice)
					Expect(disk1.DeviceKey).To(Equal(int32(2001)))
					Expect(configSpec.BootOptions.BootOrder[1]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableDiskDevice{}))
					disk0 := configSpec.BootOptions.BootOrder[1].(*vimtypes.VirtualMachineBootOptionsBootableDiskDevice)
					Expect(disk0.DeviceKey).To(Equal(int32(2000)))
					Expect(configSpec.BootOptions.BootOrder[2]).To(BeAssignableToTypeOf(&vimtypes.VirtualMachineBootOptionsBootableEthernetDevice{}))
					eth2 := configSpec.BootOptions.BootOrder[2].(*vimtypes.VirtualMachineBootOptionsBootableEthernetDevice)
					Expect(eth2.DeviceKey).To(Equal(int32(4002)))
				})
			})
		})
	})
})
