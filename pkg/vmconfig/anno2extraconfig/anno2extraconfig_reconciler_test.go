// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package anno2extraconfig_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig/anno2extraconfig"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("New", func() {
	It("should return a reconciler", func() {
		Expect(anno2extraconfig.New()).ToNot(BeNil())
	})
})

var _ = Describe("Name", func() {
	It("should return 'anno2extraconfig'", func() {
		Expect(anno2extraconfig.New().Name()).To(Equal("anno2extraconfig"))
	})
})

var _ = Describe("OnResult", func() {
	It("should return nil", func() {
		var ctx context.Context
		Expect(anno2extraconfig.New().OnResult(ctx, nil, mo.VirtualMachine{}, nil)).To(Succeed())
	})
})

var _ = Describe("Reconcile", func() {

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
		r = anno2extraconfig.New()

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
				Namespace:   "my-namespace",
				Name:        "my-vm",
				Annotations: map[string]string{},
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

		const (
			annotationKey1  = anno2extraconfig.ManagementProxyAllowListAnnotation
			extraConfigKey1 = anno2extraconfig.ManagementProxyAllowListExtraConfig
			annotationKey2  = anno2extraconfig.ManagementProxyWatermarkAnnotation
			extraConfigKey2 = anno2extraconfig.ManagementProxyWatermarkExtraConfig

			value1 = "1234"
			value2 = "5678"
		)

		var (
			err error
		)

		JustBeforeEach(func() {
			err = anno2extraconfig.Reconcile(
				ctx,
				k8sClient,
				vimClient,
				vm,
				moVM,
				configSpec)
		})

		assertEmptyExtraConfig := func() {
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			ExpectWithOffset(1, configSpec.ExtraConfig).To(HaveLen(0))
		}

		assertExtraConfig := func(expectedKey, expectedValue string) {
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			outEC := object.OptionValueList(configSpec.ExtraConfig)
			v, ok := outEC.GetString(expectedKey)
			ExpectWithOffset(1, ok).To(BeTrue())
			ExpectWithOffset(1, v).To(Equal(expectedValue))
		}

		assertNotExtraConfig := func(notExpectedKey string) {
			ExpectWithOffset(1, err).ToNot(HaveOccurred())
			outEC := object.OptionValueList(configSpec.ExtraConfig)
			_, ok := outEC.GetString(notExpectedKey)
			ExpectWithOffset(1, ok).To(BeFalse())
		}

		When("moVM config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("Annotation is present on VM", func() {

			BeforeEach(func() {
				vm.Annotations[annotationKey1] = value1
			})
			When("ExtraConfig key is missing from ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = nil
				})
				It("should update the ConfigSpec", func() {
					assertExtraConfig(extraConfigKey1, value1)
				})

				When("ExtraConfig key is different in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey1,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(1))
						assertExtraConfig(extraConfigKey1, value1)
					})
				})
			})
			When("ExtraConfig key is different in ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   extraConfigKey1,
							Value: value2,
						},
					}
				})
				It("should update the ConfigSpec", func() {
					Expect(configSpec.ExtraConfig).To(HaveLen(1))
					assertExtraConfig(extraConfigKey1, value1)
				})

				When("ExtraConfig key is different in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey1,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(1))
						assertExtraConfig(extraConfigKey1, value1)
					})
				})
			})
		})

		When("Multiple annotations are present on VM", func() {

			BeforeEach(func() {
				vm.Annotations[annotationKey1] = value1
				vm.Annotations[annotationKey2] = value1
			})
			When("ExtraConfig key 2 is missing from ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   extraConfigKey1,
							Value: value1,
						},
					}
				})
				It("should update the ConfigSpec", func() {
					Expect(configSpec.ExtraConfig).To(HaveLen(1))
					assertExtraConfig(extraConfigKey2, value1)
				})

				When("ExtraConfig key is different in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey2,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(1))
						assertNotExtraConfig(extraConfigKey1)
						assertExtraConfig(extraConfigKey2, value1)
					})
				})
			})
		})

		When("Annotation is missing from VM", func() {

			BeforeEach(func() {
				vm.Annotations = nil
			})
			When("ExtraConfig key is missing from ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = nil
				})
				It("should not update the ConfigSpec", func() {
					assertEmptyExtraConfig()
				})

				When("ExtraConfig key is present in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey1,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(1))
						assertExtraConfig(extraConfigKey1, "")
					})
				})
			})
			When("ExtraConfig key is present in ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   extraConfigKey1,
							Value: value1,
						},
					}
				})
				It("should update the ConfigSpec", func() {
					Expect(configSpec.ExtraConfig).To(HaveLen(1))
					assertExtraConfig(extraConfigKey1, "")
				})

				When("ExtraConfig key is different in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey1,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(1))
						assertExtraConfig(extraConfigKey1, "")
					})
				})
			})

			When("ExtraConfig keys are present in ConfigInfo", func() {
				BeforeEach(func() {
					moVM.Config.ExtraConfig = []vimtypes.BaseOptionValue{
						&vimtypes.OptionValue{
							Key:   extraConfigKey1,
							Value: value1,
						},
						&vimtypes.OptionValue{
							Key:   extraConfigKey2,
							Value: value1,
						},
					}
				})
				It("should update the ConfigSpec", func() {
					Expect(configSpec.ExtraConfig).To(HaveLen(2))
					assertExtraConfig(extraConfigKey1, "")
					assertExtraConfig(extraConfigKey2, "")
				})

				When("ExtraConfig keys are different in ConfigSpec", func() {
					BeforeEach(func() {
						configSpec.ExtraConfig = []vimtypes.BaseOptionValue{
							&vimtypes.OptionValue{
								Key:   extraConfigKey1,
								Value: value2,
							},
							&vimtypes.OptionValue{
								Key:   extraConfigKey2,
								Value: value2,
							},
						}
					})
					It("should update the ConfigSpec", func() {
						Expect(configSpec.ExtraConfig).To(HaveLen(2))
						assertExtraConfig(extraConfigKey1, "")
						assertExtraConfig(extraConfigKey2, "")
					})

					When("ConfigSpec.ExtraConfig already has other keys", func() {
						BeforeEach(func() {
							configSpec.ExtraConfig = append(
								configSpec.ExtraConfig,
								&vimtypes.OptionValue{
									Key:   "fu",
									Value: value2,
								},
								&vimtypes.OptionValue{
									Key:   "bar",
									Value: value2,
								})
						})
						It("should update the ConfigSpec", func() {
							Expect(configSpec.ExtraConfig).To(HaveLen(4))
							assertExtraConfig(extraConfigKey1, "")
							assertExtraConfig(extraConfigKey2, "")
							assertExtraConfig("fu", value2)
							assertExtraConfig("bar", value2)
						})
					})
				})
			})
		})
	})
})
