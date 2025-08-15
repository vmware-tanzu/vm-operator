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

		When("moVM config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should return an error", func() {
				Expect(err).To(HaveOccurred())
			})
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

		Context("enterBootSetup", func() {
			When("enterBootSetup is added to vm spec", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EnterBootSetup: vmopv1.VirtualMachineBootOptionsForceBootEntryEnabled,
					}
				})
				It("should be set in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.EnterBIOSSetup).To(BeTrue())
				})
			})

			When("enterBootSetup is updated from vm spec", func() {
				BeforeEach(func() {
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								EnterBIOSSetup: ptr.To(true),
							},
						},
					}
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EnterBootSetup: vmopv1.VirtualMachineBootOptionsForceBootEntryDisabled,
					}
				})
				It("should be updated in configSpec", func() {
					Expect(err).To(BeNil())
					Expect(configSpec.BootOptions).NotTo(BeNil())
					Expect(*configSpec.BootOptions.EnterBIOSSetup).To(BeFalse())
				})
			})

			When("enterBootSetup remains unchanged", func() {
				BeforeEach(func() {
					vm.Spec.BootOptions = &vmopv1.VirtualMachineBootOptions{
						EnterBootSetup: vmopv1.VirtualMachineBootOptionsForceBootEntryEnabled,
					}
					moVM = mo.VirtualMachine{
						Config: &vimtypes.VirtualMachineConfigInfo{
							BootOptions: &vimtypes.VirtualMachineBootOptions{
								EnterBIOSSetup: ptr.To(true),
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
	})
})
