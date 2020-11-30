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
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

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
		vmImage          *vmopv1alpha1.VirtualMachineImage
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

		vmImage = &vmopv1alpha1.VirtualMachineImage{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-image",
			},
		}

		vm = &vmopv1alpha1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "dummy-vm",
				Namespace:  "dummy-ns",
				Finalizers: []string{finalizer},
			},
			Spec: vmopv1alpha1.VirtualMachineSpec{
				ClassName: vmClass.Name,
				ImageName: vmImage.Name,
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

	Context("getCLUUID", func() {

		cl := &vmopv1alpha1.ContentLibraryProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: "dummy-cl",
			},
			Spec: vmopv1alpha1.ContentLibraryProviderSpec{
				UUID: "dummy-cl-uuid",
			},
		}

		When("the VirtualMachine Spec's VirtualMachineImage does not exist", func() {
			It("returns an error", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(BeEmpty())
				Expect(err).To(HaveOccurred())
				Expect(apiErrors.IsNotFound(err)).To(BeTrue())
			})
		})

		When("VirtualMachineImage does not have an OwnerReference", func() {
			BeforeEach(func() {
				initObjects = append(initObjects, vmImage, cl)
			})

			It("returns an empty content library UUID", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(BeEmpty())
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("VirtualMachineImage has an OwnerReference", func() {
			BeforeEach(func() {
				vmImage.OwnerReferences = []metav1.OwnerReference{
					{
						Kind: "ContentLibraryProvider",
						Name: "dummy-cl",
					},
				}

				initObjects = append(initObjects, vmImage, cl)
			})

			It("returns the content library UUID from the OwnerReference", func() {
				clUUID, err := reconciler.GetCLUUID(vmCtx)
				Expect(clUUID).To(Equal(cl.Spec.UUID))
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Context("ReconcileNormal", func() {

		BeforeEach(func() {
			initObjects = append(initObjects, vm, vmClass, vmImage)
		})

		When("the WCP_VMService FSS is enabled", func() {
			var oldVMServiceEnableFunc func() bool
			var vmClassBinding *vmopv1alpha1.VirtualMachineClassBinding

			BeforeEach(func() {
				oldVMServiceEnableFunc = lib.IsVMServiceFSSEnabled
				lib.IsVMServiceFSSEnabled = func() bool {
					return true
				}

				vmClassBinding = &vmopv1alpha1.VirtualMachineClassBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dummy-class-binding",
						Namespace: vm.Namespace,
					},
					ClassRef: vmopv1alpha1.ClassReference{
						APIVersion: vmopv1alpha1.SchemeGroupVersion.Group,
						Name:       vm.Spec.ClassName,
						Kind:       reflect.TypeOf(vmClass).Elem().Name(),
					},
				}
			})

			AfterEach(func() {
				lib.IsVMServiceFSSEnabled = oldVMServiceEnableFunc
			})

			Context("No VirtualMachineClassBindings exist in namespace", func() {
				It("return an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("no VirtualMachineClassBindings exist in namespace %s", vm.Namespace)))
				})
			})

			Context("VirtualMachineBinding is not for VM Class", func() {
				BeforeEach(func() {
					vmClassBinding.ClassRef.Name = "blah-blah-binding"
					initObjects = append(initObjects, vmClassBinding)
				})

				It("returns an error", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(fmt.Errorf("VirtualMachineClassBinding does not exist for VM Class %s in namespace %s", vm.Spec.ClassName, vm.Namespace)))
				})
			})

			Context("VirtualMachineClassBinding exists for the class", func() {
				BeforeEach(func() {
					initObjects = append(initObjects, vmClassBinding)
				})

				It("should successfully reconcile the VM, add finalizer and verify the phase", func() {
					err := reconciler.ReconcileNormal(vmCtx)
					Expect(err).NotTo(HaveOccurred())
					Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
					Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
				})
			})
		})

		When("object does not have finalizer set", func() {
			BeforeEach(func() {
				vm.Finalizers = nil
			})

			It("will set finalizer", func() {
				err := reconciler.ReconcileNormal(vmCtx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			})
		})

		It("will have finalizer set upon successful reconciliation", func() {
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.GetFinalizers()).To(ContainElement(finalizer))
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
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
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Creating))
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
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
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
			initObjects = append(initObjects, vm, vmClass, vmImage)
		})

		JustBeforeEach(func() {
			// Create the VM to be deleted
			err := reconciler.ReconcileNormal(vmCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Created))
		})

		It("will delete the created VM and emit corresponding event", func() {
			err := reconciler.ReconcileDelete(vmCtx)
			Expect(err).NotTo(HaveOccurred())

			vmExists, err := fakeVmProvider.DoesVirtualMachineExist(vmCtx, vmCtx.VM)
			Expect(err).NotTo(HaveOccurred())
			Expect(vmExists).To(BeFalse())

			expectEvent(ctx, "DeleteSuccess")
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleted))
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
			Expect(vmCtx.VM.Status.Phase).To(Equal(vmopv1alpha1.Deleting))
		})
	})
}

func expectEvent(ctx *builder.UnitTestContextForController, eventStr string) {
	var event string
	Eventually(ctx.Events).Should(Receive(&event))
	eventComponents := strings.Split(event, " ")
	ExpectWithOffset(1, eventComponents[1]).To(Equal(eventStr))
}
