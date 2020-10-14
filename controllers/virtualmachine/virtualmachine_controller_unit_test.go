// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	vmopContext "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	providerfake "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/fake"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func unitTests() {
	Describe("Invoking Reconcile", unitTestsReconcile)
}

const finalizer = "virtualmachine.vmoperator.vmware.com"

func unitTestsReconcile() {
	const (
		providerError = "provider error"
	)

	var (
		initObjects []runtime.Object
		ctx         *builder.UnitTestContextForController

		reconciler       *virtualmachine.VirtualMachineReconciler
		fakeVmProvider   *providerfake.FakeVmProvider
		vmCtx            *vmopContext.VirtualMachineContext
		vm               *vmopv1alpha1.VirtualMachine
		vmClass          *vmopv1alpha1.VirtualMachineClass
		vmClassBinding   *vmopv1alpha1.VirtualMachineClassBinding
		vmMetaData       *corev1.ConfigMap
		vmResourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy
		storageClass     *storagev1.StorageClass
		resourceQuota    *corev1.ResourceQuota
	)

	BeforeEach(func() {
		vmClass = &vmopv1alpha1.VirtualMachineClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-vmclass",
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: vmClass.Name,
			},
		}

		vmMetaData = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-metadata",
				Namespace: vm.Namespace,
			},
			Data: map[string]string{
				"foo": "bar",
			},
		}

		vmResourcePolicy = &vmopv1alpha1.VirtualMachineSetResourcePolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm-rp",
				Namespace: vm.Namespace,
			},
			Spec: vmopv1alpha1.VirtualMachineSetResourcePolicySpec{
				ResourcePool: vmopv1alpha1.ResourcePoolSpec{Name: "fooRP"},
				Folder:       vmopv1alpha1.FolderSpec{Name: "fooFolder"},
			},
		}

		storageClass = &storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-storage-class",
			},
			Provisioner: "foo",
			Parameters: map[string]string{
				"storagePolicyID": "id42",
			},
		}

		rlName := storageClass.Name + ".storageclass.storage.k8s.io/persistentvolumeclaims"

		resourceQuota = &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-resource-quota",
				Namespace: vm.Namespace,
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceName(rlName): resource.MustParse("1"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		ctx = suite.NewUnitTestContextForController(initObjects...)
		reconciler = virtualmachine.NewReconciler(
			ctx.Client,
			ctx.Logger,
			ctx.Recorder,
			ctx.VmProvider,
		)
		fakeVmProvider = ctx.VmProvider.(*providerfake.FakeVmProvider)

		vmCtx = &vmopContext.VirtualMachineContext{
			Context: ctx,
			Logger:  ctx.Logger.WithName(vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		initObjects = nil
		vmCtx = nil
		reconciler = nil
		fakeVmProvider = nil
	})

	Context("ReconcileNormal", func() {

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass)
		})

		When("the WCP_VMService FSS is enabled", func() {
			var oldVMServiceEnableFunc func() bool

			BeforeEach(func() {
				oldVMServiceEnableFunc = lib.IsVMServiceFSSEnabled
				lib.IsVMServiceFSSEnabled = func() bool {
					return true
				}
			})

			AfterEach(func() {
				lib.IsVMServiceFSSEnabled = oldVMServiceEnableFunc
			})

			Context("VirtualMachineClassBinding does not exist for the class", func() {
				It("should fail to reconcile the VM with the appropriate error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace dummy-ns", vm.Spec.ClassName)))
				})
			})

			Context("VirtualMachineClassBinding exists for the class", func() {
				BeforeEach(func() {
					vmClassBinding = &vmopv1alpha1.VirtualMachineClassBinding{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "dummy-classbinding",
							Namespace: vm.Namespace,
						},
						ClassRef: vmopv1alpha1.ClassReference{
							APIVersion: vmopv1alpha1.SchemeGroupVersion.Group,
							Name:       vm.Spec.ClassName,
							Kind:       reflect.TypeOf(vmClass).Elem().Name(),
						},
					}
					initObjects = append(initObjects, vmClassBinding)
				})

				It("should successfully reconcile the VM, add finalizer and verify the phase", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
					expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Created)
				})
			})
		})

		It("will have finalizer set after successful reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Created)
		})

		It("will return error when provider fails to create VM", func() {
			// Simulate an error during VM create
			fakeVmProvider.CreateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(providerError))
			expectEvent(ctx, "CreateFailure")
			expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Creating)
		})

		It("will return error when provider fails to update VM", func() {
			// Simulate an error after the VM is created.
			fakeVmProvider.UpdateVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
				return errors.New(providerError)
			}

			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(providerError))
			expectEvent(ctx, "UpdateFailure")
			//expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Created) VM won't be updated: bug until we Patch().
		})

		It("can be called multiple times", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))

			err = reconciler.ReconcileNormal(vmCtx)
			Expect(err).ToNot(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
		})

		When("VM Class does not exist", func() {
			BeforeEach(func() {
				initObjects = []runtime.Object{vm}
			})

			It("return an error", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("not found"))
			})
		})

		When("VM Metadata is specified", func() {
			BeforeEach(func() {
				vm.Spec.VmMetadata = &vmopv1alpha1.VirtualMachineMetadata{
					ConfigMapName: vmMetaData.Name,
					Transport:     "transport",
				}
			})

			When("VM Metadata does not exist", func() {
				It("return an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("VM Metadata exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmMetaData)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})

		When("VM ResourcePolicy is specified", func() {
			BeforeEach(func() {
				vm.Spec.ResourcePolicyName = vmResourcePolicy.Name
			})

			When("VM ResourcePolicy does not exist", func() {
				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("VM ResourcePolicy exists", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmResourcePolicy)
				})

				When("VM ResourcePolicy is not ready", func() {
					It("returns an error", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("VirtualMachineSetResourcePolicy is not yet ready"))
					})
				})

				When("VM ResourcePolicy exists check returns error", func() {
					errMsg := "exists error"
					JustBeforeEach(func() {
						fakeVmProvider.DoesVirtualMachineSetResourcePolicyExistFn = func(ctx context.Context, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
							return false, errors.New(errMsg)
						}
					})

					It("returns an error", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring(errMsg))
					})
				})

				When("VM ResourcePolicy is ready", func() {
					JustBeforeEach(func() {
						fakeVmProvider.DoesVirtualMachineSetResourcePolicyExistFn = func(ctx context.Context, rp *vmopv1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
							return true, nil
						}
					})

					It("returns success", func() {
						err := reconciler.ReconcileNormal(vmCtx)
						Expect(err).ToNot(HaveOccurred())
					})
				})
			})
		})

		When("VM StorageClass is specified", func() {
			BeforeEach(func() {
				vm.Spec.StorageClass = storageClass.Name
			})

			When("ResourceQuotas exists but no StorageClass", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, resourceQuota)
				})

				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
				})
			})

			When("StorageClass exists but no ResourceQuotas", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, storageClass)
				})

				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("no ResourceQuotas assigned to namespace"))
				})
			})

			When("StorageClass exists but not assigned to ResourceQuota", func() {
				BeforeEach(func() {
					resourceQuota.Spec.Hard = corev1.ResourceList{
						"blah.storageclass.storage.k8s.io/persistentvolumeclaims": resource.MustParse("42"),
					}
					initObjects = append(initObjects, storageClass, resourceQuota)
				})

				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("is not assigned to any ResourceQuotas"))
				})
			})

			When("StorageClass exists and is assigned to ResourceQuota", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, storageClass, resourceQuota)
				})

				It("returns success", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).ToNot(HaveOccurred())
				})
			})
		})
	})

	Context("ReconcileDelete", func() {

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass)
		})

		JustBeforeEach(func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Created)
		})

		It("will delete the created VM and emit corresponding event", func() {
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeFalse())

			expectEvent(ctx, "DeleteSuccess")
			expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Deleted)
		})

		It("will emit corresponding event during delete failure", func() {
			// Simulate delete failure
			fakeVmProvider.DeleteVirtualMachineFn = func(ctx context.Context, vm *vmopv1alpha1.VirtualMachine) error {
				return errors.New(providerError)
			}
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).To(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeTrue())

			expectEvent(ctx, "DeleteFailure")
			expectPhase(vmCtx, ctx.Client, vmopv1alpha1.Deleting)
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	var event string
	Eventually(ctx.Events).Should(Receive(&event))
	eventComponents := strings.Split(event, " ")
	ExpectWithOffset(1, eventComponents[1]).To(Equal(eventStr))
}

func expectPhase(ctx *vmopContext.VirtualMachineContext, k8sClient client.Client, expectedPhase vmopv1alpha1.VMStatusPhase) {
	vm := &vmopv1alpha1.VirtualMachine{}
	err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ctx.VM.Namespace, Name: ctx.VM.Name}, vm)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, vm.Status.Phase).To(Equal(expectedPhase))
}
