// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	upgradevm "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/upgrade/virtualmachine"
)

var _ = Describe("ReconcileSchemaUpgrade", func() {

	var (
		ctx       context.Context
		k8sClient ctrlclient.Client
		vm        *vmopv1.VirtualMachine
		moVM      mo.VirtualMachine
	)

	BeforeEach(func() {
		pkg.BuildVersion = "v1.2.3"
		ctx = pkgcfg.NewContextWithDefaultConfig()
		ctx = logr.NewContext(ctx, klog.Background())
		k8sClient = fake.NewFakeClient()
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				UID: types.UID("abc-123"),
			},
		}
		moVM = mo.VirtualMachine{
			Config: &vimtypes.VirtualMachineConfigInfo{},
		}
	})

	When("it should panic", func() {
		When("ctx is nil", func() {
			BeforeEach(func() {
				ctx = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
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
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("k8sClient is nil"))
			})
		})

		When("vm is nil", func() {
			JustBeforeEach(func() {
				vm = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("vm is nil"))
			})
		})

		When("moVM.config is nil", func() {
			BeforeEach(func() {
				moVM.Config = nil
			})
			It("should panic", func() {
				fn := func() {
					_ = upgradevm.ReconcileSchemaUpgrade(ctx, k8sClient, vm, moVM)
				}
				Expect(fn).To(PanicWith("moVM.config is nil"))
			})
		})
	})

	When("it should not panic", func() {
		var expectedErr error

		BeforeEach(func() {
			expectedErr = upgradevm.ErrUpgradeSchema
		})
		JustBeforeEach(func() {
			err := upgradevm.ReconcileSchemaUpgrade(
				ctx,
				k8sClient,
				vm,
				moVM)
			if expectedErr == nil {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(MatchError(expectedErr))
			}
		})

		When("the object is already upgraded", func() {
			BeforeEach(func() {
				expectedErr = nil

				vm.Annotations = map[string]string{
					pkgconst.UpgradedToBuildVersionAnnotationKey:  pkgcfg.FromContext(ctx).BuildVersion,
					pkgconst.UpgradedToSchemaVersionAnnotationKey: vmopv1.GroupVersion.Version,
				}
				vm.Spec.InstanceUUID = ""
				vm.Spec.BiosUUID = ""
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						InstanceID: "",
					},
				}
				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					InstanceUuid: "123",
					Uuid:         "123",
				}
			})
			It("should not modify any of the fields it would otherwise upgrade", func() {
				Expect(vm.Spec.InstanceUUID).To(BeEmpty())
				Expect(vm.Spec.BiosUUID).To(BeEmpty())
				Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).To(BeEmpty())
			})
		})

		When("the object needs to be upgraded", func() {
			BeforeEach(func() {
				moVM.Config = &vimtypes.VirtualMachineConfigInfo{
					Uuid:         "test-bios-uuid",
					InstanceUuid: "test-instance-uuid",
				}
			})

			assertUpgraded := func() {
				ExpectWithOffset(1, vm.Annotations).To(HaveKeyWithValue(
					pkgconst.UpgradedToBuildVersionAnnotationKey,
					pkgcfg.FromContext(ctx).BuildVersion))
				ExpectWithOffset(1, vm.Annotations).To(HaveKeyWithValue(
					pkgconst.UpgradedToSchemaVersionAnnotationKey,
					vmopv1.GroupVersion.Version))
			}

			Context("BIOS UUID reconciliation", func() {
				When("BIOS UUID is empty in VM spec", func() {
					It("should set BIOS UUID from moVM", func() {
						assertUpgraded()
						Expect(vm.Spec.BiosUUID).To(Equal("test-bios-uuid"))
					})
				})

				When("BIOS UUID is already set in VM spec", func() {
					BeforeEach(func() {
						vm.Spec.BiosUUID = "existing-bios-uuid"
					})
					It("should not modify existing BIOS UUID", func() {
						Expect(vm.Spec.BiosUUID).To(Equal("existing-bios-uuid"))
					})
				})

				When("moVM config UUID is empty", func() {
					BeforeEach(func() {
						moVM.Config.Uuid = ""
					})
					It("should not set BIOS UUID", func() {
						Expect(vm.Spec.BiosUUID).To(BeEmpty())
					})
				})
			})

			Context("Instance UUID reconciliation", func() {
				When("Instance UUID is empty in VM spec", func() {
					It("should set Instance UUID from moVM", func() {
						assertUpgraded()
						Expect(vm.Spec.InstanceUUID).To(Equal("test-instance-uuid"))
					})
				})

				When("Instance UUID is already set in VM spec", func() {
					BeforeEach(func() {
						vm.Spec.InstanceUUID = "existing-instance-uuid"
					})
					It("should not modify existing Instance UUID", func() {
						Expect(vm.Spec.InstanceUUID).To(Equal("existing-instance-uuid"))
					})
				})

				When("moVM config InstanceUuid is empty", func() {
					BeforeEach(func() {
						moVM.Config.InstanceUuid = ""
					})
					It("should not set Instance UUID", func() {
						Expect(vm.Spec.InstanceUUID).To(BeEmpty())
					})
				})
			})

			Context("CloudInit instance UUID reconciliation", func() {
				When("VM has CloudInit bootstrap configuration", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
							CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
						}
					})

					When("CloudInit InstanceID is empty", func() {
						It("should generate CloudInit InstanceID", func() {
							assertUpgraded()
							Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).ToNot(BeEmpty())
						})
					})

					When("CloudInit InstanceID is already set", func() {
						BeforeEach(func() {
							vm.Spec.Bootstrap.CloudInit.InstanceID = "existing-cloud-init-id"
						})
						It("should not modify existing CloudInit InstanceID", func() {
							Expect(vm.Spec.Bootstrap.CloudInit.InstanceID).To(Equal("existing-cloud-init-id"))
						})
					})
				})

				When("VM has no bootstrap configuration", func() {
					It("should not set CloudInit InstanceID", func() {
						Expect(vm.Spec.Bootstrap).To(BeNil())
					})
				})

				When("VM has bootstrap but no CloudInit configuration", func() {
					BeforeEach(func() {
						vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
					})
					It("should not set CloudInit InstanceID", func() {
						Expect(vm.Spec.Bootstrap.CloudInit).To(BeNil())
					})
				})
			})
		})
	})
})
